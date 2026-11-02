import { afterEach, describe, expect, it, vi } from "vitest";
import {
    canPickFolder,
    folderPathFor,
    folderPathsFor,
    isTemporaryMediaName,
    materializeAndroidFiles,
    pickAndroidFolder,
    releaseAndroidFiles,
} from "./android_folder";

interface TestWindow {
    wails?: {
        pickFolder?: (id: string) => void;
        materializeFiles?: (id: string, idsJson: string) => void;
        releaseFiles?: (id: string, idsJson: string) => void;
    };
    _wailsAndroidCallback?: ((id: string, result: string | null, error: string | null) => void) | undefined;
    _tdriveBridgeCallbacks?: Record<string, unknown>;
}

const host = window as unknown as TestWindow;

interface BridgeCall {
    method: string;
    id: string;
    args: string[];
}

let calls: BridgeCall[] = [];

/** Stands in for the Android bridge, recording every call it was handed. */
function installBridge(): void {
    calls = [];
    host.wails = {
        pickFolder: (id) => { calls.push({ method: "pickFolder", id, args: [] }); },
        materializeFiles: (id, idsJson) => { calls.push({ method: "materializeFiles", id, args: [idsJson] }); },
        releaseFiles: (id, idsJson) => { calls.push({ method: "releaseFiles", id, args: [idsJson] }); },
    };
}

function lastCall(): BridgeCall {
    return calls[calls.length - 1];
}

function answer(id: string, result: string | null, error: string | null = null): void {
    host._wailsAndroidCallback?.(id, result, error);
}

afterEach(() => {
    delete host.wails;
    host._wailsAndroidCallback = undefined;
    delete host._tdriveBridgeCallbacks;
    calls = [];
});

describe("asking Android for a folder", () => {
    it("reports whether this build can ask at all", () => {
        expect(canPickFolder()).toBe(false);
        installBridge();
        expect(canPickFolder()).toBe(true);
    });

    it("resolves with what the bridge found in the tree", async () => {
        installBridge();
        const picked = pickAndroidFolder();

        answer(lastCall().id, JSON.stringify({
            root: "Holiday",
            files: [{ id: "doc:1", rel: "sub/a.jpg", size: 12345 }],
        }));
        await expect(picked).resolves.toEqual({
            root: "Holiday",
            files: [{ id: "doc:1", rel: "sub/a.jpg", size: 12345, modified: 0 }],
            dirs: [],
            skipped: 0,
        });
    });

    it("keeps each file's modified time and the bridge's count of skipped camera files", async () => {
        installBridge();
        const picked = pickAndroidFolder();

        answer(lastCall().id, JSON.stringify({
            root: "Camera",
            files: [{ id: "doc:1", rel: "VID_1.mp4", size: 9, modified: 1700000000000 }],
            skipped: 2,
        }));
        await expect(picked).resolves.toEqual({
            root: "Camera",
            files: [{ id: "doc:1", rel: "VID_1.mp4", size: 9, modified: 1700000000000 }],
            dirs: [],
            skipped: 2,
        });
    });

    it("drops temporary camera copies even when the bridge sends them, and counts them", async () => {
        // The bridge filters these too. This is the second line of defense, so a
        // manifest from anywhere else still cannot bring them in.
        installBridge();
        const picked = pickAndroidFolder();

        answer(lastCall().id, JSON.stringify({
            root: "Camera",
            files: [
                { id: "doc:1", rel: ".pending-1793613831-VID_20261026_170351.mp4", size: 5, modified: 1 },
                { id: "doc:2", rel: "Sub/.TRASHED-7-old.jpg", size: 5, modified: 1 },
                { id: "doc:3", rel: "VID_20261026_170351.mp4", size: 5, modified: 1 },
            ],
            skipped: 1,
        }));
        await expect(picked).resolves.toEqual({
            root: "Camera",
            files: [{ id: "doc:3", rel: "VID_20261026_170351.mp4", size: 5, modified: 1 }],
            dirs: [],
            skipped: 3,
        });
    });

    it("treats a dismissed picker as no folder rather than a failure", async () => {
        // Changing your mind is not an error, and an error here would put a
        // red notice on screen for it.
        installBridge();
        const picked = pickAndroidFolder();

        answer(lastCall().id, "");
        await expect(picked).resolves.toBeNull();
    });

    it("drops manifest entries with nothing to upload them by", async () => {
        installBridge();
        const picked = pickAndroidFolder();

        answer(lastCall().id, JSON.stringify({
            root: "Holiday",
            files: [{ id: "", rel: "a.jpg", size: 1 }, { id: "doc:2", rel: "", size: 1 }, { id: "doc:3", rel: "b.jpg" }],
        }));
        await expect(picked).resolves.toEqual({
            root: "Holiday",
            files: [{ id: "doc:3", rel: "b.jpg", size: -1, modified: 0 }],
            dirs: [],
            skipped: 0,
        });
    });

    it("rejects when the bridge could not read the folder", async () => {
        installBridge();
        const picked = pickAndroidFolder();

        answer(lastCall().id, null, "could not read that folder");
        await expect(picked).rejects.toThrow("could not read that folder");
    });

    it("leaves other host callbacks alone", async () => {
        // The runtime owns this global and every async call into the host comes
        // back through it. Replacing it rather than chaining would break all of
        // them, quietly, for as long as a folder picker had ever been opened.
        const runtime = vi.fn();
        host._wailsAndroidCallback = runtime;
        installBridge();
        const picked = pickAndroidFolder();

        answer("some-other-call", "result", null);
        expect(runtime).toHaveBeenCalledWith("some-other-call", "result", null);

        answer(lastCall().id, JSON.stringify({ root: "Holiday", files: [] }));
        await expect(picked).resolves.toEqual({ root: "Holiday", files: [], dirs: [], skipped: 0 });
        // Ours was handled by us, not passed down.
        expect(runtime).toHaveBeenCalledTimes(1);
    });

    it("chains onto the runtime only once, however many folders are picked", async () => {
        host._wailsAndroidCallback = vi.fn();
        installBridge();

        const first = pickAndroidFolder();
        const chained = host._wailsAndroidCallback;
        answer(lastCall().id, JSON.stringify({ root: "A", files: [] }));
        await first;

        const second = pickAndroidFolder();
        expect(host._wailsAndroidCallback).toBe(chained);
        answer(lastCall().id, JSON.stringify({ root: "B", files: [] }));
        await expect(second).resolves.toEqual({ root: "B", files: [], dirs: [], skipped: 0 });
    });

    it("forgets a picker once it has answered", async () => {
        installBridge();
        const picked = pickAndroidFolder();
        answer(lastCall().id, JSON.stringify({ root: "Holiday", files: [] }));
        await picked;

        expect(Object.keys(host._tdriveBridgeCallbacks ?? {})).toHaveLength(0);
    });

    it("refuses on a build with no bridge", async () => {
        await expect(pickAndroidFolder()).rejects.toThrow(/cannot open a folder picker/);
    });
});

describe("materializing and releasing files", () => {
    it("maps the ids it asked for to the paths the bridge copied them to", async () => {
        installBridge();
        const materialized = materializeAndroidFiles(["doc:1", "doc:2"]);

        expect(lastCall().method).toBe("materializeFiles");
        expect(JSON.parse(lastCall().args[0])).toEqual(["doc:1", "doc:2"]);
        answer(lastCall().id, JSON.stringify({ paths: { "doc:1": "/cache/a.jpg", "doc:2": "/cache/b.jpg" } }));
        await expect(materialized).resolves.toMatchObject({
            paths: new Map([
                ["doc:1", "/cache/a.jpg"],
                ["doc:2", "/cache/b.jpg"],
            ]),
        });
    });

    it("leaves out an id the bridge could not read instead of failing the batch", async () => {
        installBridge();
        const materialized = materializeAndroidFiles(["doc:1", "doc:2"]);

        answer(lastCall().id, JSON.stringify({ paths: { "doc:1": "/cache/a.jpg" } }));
        const { paths, changed, notOnDevice } = await materialized;
        expect(paths.has("doc:2")).toBe(false);
        expect(paths.get("doc:1")).toBe("/cache/a.jpg");
        expect(changed.size).toBe(0);
        expect(notOnDevice.size).toBe(0);
    });

    it("reports files that changed while copying and files that only exist in the cloud", async () => {
        installBridge();
        const materialized = materializeAndroidFiles(["doc:1", "doc:2"]);

        answer(lastCall().id, JSON.stringify({ paths: { "doc:1": "/cache/a.jpg" }, changed: ["doc:1"], notOnDevice: ["doc:2"] }));
        const { changed, notOnDevice } = await materialized;
        expect([...changed]).toEqual(["doc:1"]);
        expect([...notOnDevice]).toEqual(["doc:2"]);
    });

    it("releases by id", async () => {
        installBridge();
        const released = releaseAndroidFiles(["doc:1"]);

        expect(lastCall().method).toBe("releaseFiles");
        expect(JSON.parse(lastCall().args[0])).toEqual(["doc:1"]);
        answer(lastCall().id, "");
        await expect(released).resolves.toBeUndefined();
    });
});

describe("the folder tree a manifest implies", () => {
    it("lists every directory parents first", () => {
        const paths = folderPathsFor({
            root: "Holiday",
            files: [
                { id: "1", rel: "b/deep/nested/x.jpg", size: 1, modified: 0 },
                { id: "2", rel: "a/y.jpg", size: 1, modified: 0 },
                { id: "3", rel: "a/z.jpg", size: 1, modified: 0 },
                { id: "4", rel: "top.jpg", size: 1, modified: 0 },
            ],
            dirs: [],
            skipped: 0,
        });

        expect(paths).toEqual([
            "Holiday",
            "Holiday/b",
            "Holiday/a",
            "Holiday/b/deep",
            "Holiday/b/deep/nested",
        ]);
        // Every entry's parent is listed before it.
        for (const path of paths) {
            const cut = path.lastIndexOf("/");
            if (cut < 0) continue;
            expect(paths.indexOf(path.slice(0, cut))).toBeLessThan(paths.indexOf(path));
        }
    });

    it("keeps the picked folder itself even when it holds nothing", () => {
        expect(folderPathsFor({ root: "Empty", files: [], dirs: [], skipped: 0 })).toEqual(["Empty"]);
    });

    it("creates the folders the bridge lists even when no file lies inside them", () => {
        const paths = folderPathsFor({
            root: "Holiday",
            files: [{ id: "1", rel: "a/zero.jpg", size: 0, modified: 0 }],
            dirs: ["a", "empty", "trip/empty"],
            skipped: 0,
        });

        expect(paths).toEqual(["Holiday", "Holiday/a", "Holiday/empty", "Holiday/trip", "Holiday/trip/empty"]);
    });

    it("keeps the folders the bridge lists, empty ones included", async () => {
        installBridge();
        const picked = pickAndroidFolder();

        answer(lastCall().id, JSON.stringify({ root: "Holiday", files: [], dirs: ["empty", "trip/empty"] }));
        await expect(picked).resolves.toEqual({ root: "Holiday", files: [], dirs: ["empty", "trip/empty"], skipped: 0 });
    });

    it("places a file against the directory it came from", () => {
        expect(folderPathFor("Holiday", "sub/a.jpg")).toBe("Holiday/sub");
        expect(folderPathFor("Holiday", "a.jpg")).toBe("Holiday");
        expect(folderPathFor("", "a.jpg")).toBe("");
    });
});

describe("the temporary-file rule and the ledger path", () => {
    it("matches camera capture and trash copies, case-insensitively", () => {
        expect(isTemporaryMediaName(".pending-1793613831-VID_20261026_170351.mp4")).toBe(true);
        expect(isTemporaryMediaName(".Trashed-42-IMG_1.jpg")).toBe(true);
    });

    it("leaves ordinary and near-miss names alone", () => {
        expect(isTemporaryMediaName("VID_20261026_170351.mp4")).toBe(false);
        expect(isTemporaryMediaName(".pending-VID.mp4")).toBe(false);
        expect(isTemporaryMediaName(".pending-12x-clip.mp4")).toBe(false);
        expect(isTemporaryMediaName("pending-12-clip.mp4")).toBe(false);
        expect(isTemporaryMediaName(".pending-12-")).toBe(false);
    });

});
