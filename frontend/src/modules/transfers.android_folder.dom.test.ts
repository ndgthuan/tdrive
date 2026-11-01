import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { idleTransferActivity, state } from '../state';

const mocks = vi.hoisted(() => ({
    markTransferDone: vi.fn(),
    notify: vi.fn(),
    openImportOptionsModal: vi.fn(),
    pushTransferStart: vi.fn(),
    updateTransferName: vi.fn(),
    updateTransferProgress: vi.fn(),
    refreshFiles: vi.fn(),
}));

const eventListeners = vi.hoisted(() => new Map<string, (...args: unknown[]) => void>());
const eventsOn = vi.hoisted(() => vi.fn((eventName: string, callback: (event: { name: string; data: unknown[] }) => void) => {
    eventListeners.set(eventName, (...args: unknown[]) => callback({ name: eventName, data: args }));
    return () => eventListeners.delete(eventName);
}));
const app = vi.hoisted(() => ({
    CreateFolder: vi.fn(),
    DownloadFile: vi.fn(),
    GetFolderContents: vi.fn(),
    ImportFindExisting: vi.fn(),
    ImportLedgerForget: vi.fn(),
    ImportLedgerRecord: vi.fn(),
    ImportLedgerVeto: vi.fn(),
    ImportPaths: vi.fn(),
    ImportResolveFolders: vi.fn(),
    PlanImport: vi.fn(),
    SelectFiles: vi.fn(),
    SelectFolder: vi.fn(),
    UploadToDriveFS: vi.fn(),
}));

vi.mock('../../bindings/TDrive/internal/app/app', () => app);
vi.mock('@wailsio/runtime', () => ({ Events: { On: eventsOn } }));
vi.mock('./notifications', () => ({ notify: mocks.notify }));
vi.mock('./app_actions', () => ({ appActions: () => ({ refreshFiles: mocks.refreshFiles }) }));
vi.mock('./notif_bell', () => ({
    forgetResumableUpload: vi.fn(),
    markTransferDone: mocks.markTransferDone,
    pushTransferStart: mocks.pushTransferStart,
    updateTransferName: mocks.updateTransferName,
    updateTransferProgress: mocks.updateTransferProgress,
    wasUploadCanceled: () => false,
}));
vi.mock('./encryption', () => ({ loadEncryptionStatus: vi.fn() }));
vi.mock('./modals/import_options', () => ({ openImportOptionsModal: mocks.openImportOptionsModal }));
vi.mock('./modals/upload_options', () => ({ openUploadOptionsModal: vi.fn() }));
vi.mock('./modals/encryption_setup', () => ({ openEncryptionSetupModal: vi.fn() }));
vi.mock('./modals/encryption_password', () => ({ openEncryptionPasswordModal: vi.fn() }));
vi.mock('../ui/chrome/upload_menu.svelte', () => ({ default: {} }));
vi.mock('../ui/mount', () => ({ mountSvelte: vi.fn() }));

import { get } from 'svelte/store';
import { failedUploadReports, failedUploadsModal } from '../ui/modals/failed_uploads_store';
import { activateTransferSurfaces, importFolderWithParentID, retryFailedUploads } from './transfers';

interface BridgeWindow {
    wails?: {
        pickFolder?: (id: string) => void;
        materializeFiles?: (id: string, idsJson: string) => void;
        releaseFiles?: (id: string, idsJson: string) => void;
    };
    _wailsAndroidCallback?: ((id: string, result: string | null, error: string | null) => void) | undefined;
    _tdriveBridgeCallbacks?: Record<string, unknown>;
}

const host = window as unknown as BridgeWindow;

let manifestJson = '';
let unreadable = new Set<string>();
let changedWhileCopying = new Set<string>();
let notOnDevice = new Set<string>();
/** Runs when the bridge is asked to copy, to model a cancel or a drive switch landing mid-window. */
let onMaterialize: (() => void) | null = null;
let materialized: string[][] = [];
let released: string[][] = [];
let createdFolders: { id: string; name: string; parentId: string }[] = [];

/**
 * Stands in for the Android bridge. It answers on the spot, which the real one
 * never does, but the callback is registered before the bridge is called so
 * the orchestration cannot tell the difference.
 */
function installBridge(): void {
    const answer = (id: string, result: string) => host._wailsAndroidCallback?.(id, result, null);
    host.wails = {
        pickFolder: (id) => answer(id, manifestJson),
        materializeFiles: (id, idsJson) => {
            const ids = JSON.parse(idsJson) as string[];
            materialized.push(ids);
            onMaterialize?.();
            const paths: Record<string, string> = {};
            for (const fileId of ids) {
                if (!unreadable.has(fileId) && !notOnDevice.has(fileId)) paths[fileId] = `/cache/${fileId}`;
            }
            answer(id, JSON.stringify({
                paths,
                changed: ids.filter((fileId) => changedWhileCopying.has(fileId)),
                notOnDevice: ids.filter((fileId) => notOnDevice.has(fileId)),
            }));
        },
        releaseFiles: (id, idsJson) => {
            released.push(JSON.parse(idsJson) as string[]);
            answer(id, '');
        },
    };
}

function manifestOf(root: string, rels: string[], size = 100): string {
    return JSON.stringify({
        root,
        files: rels.map((rel, index) => ({ id: `doc:${index}`, rel, size })),
    });
}

/** The entries an import sent to the live check, by file name. */
function liveNames(call: unknown[]): string[] {
    return (call[2] as { name: string }[]).map((entry) => entry.name);
}

let deactivateTransfers = () => {};

beforeEach(() => {
    vi.clearAllMocks();
    eventListeners.clear();
    state.activeChannel = { id: 1, title: 'Test drive', kind: 'shared' };
    state.transferActivity = idleTransferActivity;
    state.cancelingUpload = false;
    state.importBatch = null;
    state.uploadBatch = null;
    state.uploadTransfers = new Map();

    manifestJson = manifestOf('Holiday', ['a.jpg']);
    unreadable = new Set();
    changedWhileCopying = new Set();
    notOnDevice = new Set();
    onMaterialize = null;
    materialized = [];
    released = [];
    createdFolders = [];

    mocks.openImportOptionsModal.mockResolvedValue({ encrypt: false, extract: false });
    // Stands in for the backend: reuse is decided there, so the mock creates
    // whatever it is asked for and records it, in the order it is asked.
    app.ImportResolveFolders.mockImplementation(async (_driveId: number, parentId: string, paths: string[]) => {
        const ids: Record<string, string> = {};
        for (const path of paths) {
            const cut = path.lastIndexOf('/');
            const name = path.slice(cut + 1);
            const parent = cut < 0 ? parentId : ids[path.slice(0, cut)];
            const existing = createdFolders.find((folder) => folder.name === name && folder.parentId === parent);
            if (existing) {
                ids[path] = existing.id;
                continue;
            }
            const folder = { id: `f${createdFolders.length + 1}`, name, parentId: parent };
            createdFolders.push(folder);
            ids[path] = folder.id;
        }
        return ids;
    });
    app.ImportFindExisting.mockResolvedValue([]);
    app.ImportLedgerVeto.mockImplementation(async (_driveId: number, _parentId: string, entries: unknown[]) => entries.map(() => false));
    app.ImportLedgerRecord.mockResolvedValue(undefined);
    app.ImportLedgerForget.mockResolvedValue(undefined);
    app.UploadToDriveFS.mockImplementation(async (paths: string[]) => ({
        result: { ok: true },
        files: paths.map((path) => ({ name: path })),
    }));
    failedUploadReports.set([]);
    failedUploadsModal.close();

    installBridge();
    deactivateTransfers = activateTransferSurfaces();
});

afterEach(() => {
    state.activeChannel = null;
    state.transferActivity = idleTransferActivity;
    state.cancelingUpload = false;
    state.importBatch = null;
    delete host.wails;
    host._wailsAndroidCallback = undefined;
    delete host._tdriveBridgeCallbacks;
    deactivateTransfers();
});

describe('uploading an Android folder a window at a time', () => {
    it('copies, uploads and releases four files at a time', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i']);

        await importFolderWithParentID('root');

        expect(materialized).toEqual([
            ['doc:0', 'doc:1', 'doc:2', 'doc:3'],
            ['doc:4', 'doc:5', 'doc:6', 'doc:7'],
            ['doc:8'],
        ]);
        // Nothing stays in the cache behind the window that is uploading.
        expect(released).toEqual(materialized);
        expect(app.UploadToDriveFS).toHaveBeenCalledTimes(3);
    });

    it('creates the folder tree parents first and uploads each file into its own', async () => {
        manifestJson = manifestOf('Holiday', ['b/deep/x.jpg', 'a/y.jpg', 'top.jpg']);

        await importFolderWithParentID('root');

        expect(app.ImportResolveFolders).toHaveBeenCalledWith(1, 'root', ['Holiday', 'Holiday/b', 'Holiday/a', 'Holiday/b/deep']);
        expect(createdFolders).toEqual([
            { id: 'f1', name: 'Holiday', parentId: 'root' },
            { id: 'f2', name: 'b', parentId: 'f1' },
            { id: 'f3', name: 'a', parentId: 'f1' },
            { id: 'f4', name: 'deep', parentId: 'f2' },
        ]);
        const [paths, parentIDs] = app.UploadToDriveFS.mock.calls[0];
        expect(paths).toEqual(['/cache/doc:0', '/cache/doc:1', '/cache/doc:2']);
        expect(parentIDs).toEqual(['f4', 'f3', 'f1']);
    });

    it('releases the window even when the upload throws, and reports the import failed', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b']);
        app.UploadToDriveFS.mockRejectedValue(new Error('backend went away'));

        await importFolderWithParentID('root');

        expect(released).toEqual([['doc:0', 'doc:1']]);
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'failed' });
    });

    it('releases the window it was cancelled in and starts no more', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b', 'c', 'd', 'e']);
        app.UploadToDriveFS.mockResolvedValue({
            result: { ok: false, error: { code: 'canceled', message: 'canceled' } },
            files: [],
        });

        await importFolderWithParentID('root');

        expect(app.UploadToDriveFS).toHaveBeenCalledTimes(1);
        expect(released).toEqual([['doc:0', 'doc:1', 'doc:2', 'doc:3']]);
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'canceled' });
        // Backing out is not a failure to report.
        expect(mocks.notify).not.toHaveBeenCalled();
    });

    it('uploads the rest of a window when one file will not copy', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b', 'c']);
        unreadable = new Set(['doc:1']);

        await importFolderWithParentID('root');

        const [paths] = app.UploadToDriveFS.mock.calls[0];
        expect(paths).toEqual(['/cache/doc:0', '/cache/doc:2']);
        // The id that never became a file is still released: a half-done copy
        // would otherwise sit in the cache forever.
        expect(released).toEqual([['doc:0', 'doc:1', 'doc:2']]);
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'failed' });
        expect(mocks.updateTransferName).toHaveBeenLastCalledWith({
            id: 'import',
            direction: 'up',
            name: 'Imported 2 files · 1 failed',
        });
    });

    it('releases a window whose files all failed to copy', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b']);
        unreadable = new Set(['doc:0', 'doc:1']);

        await importFolderWithParentID('root');

        expect(app.UploadToDriveFS).not.toHaveBeenCalled();
        expect(released).toEqual([['doc:0', 'doc:1']]);
    });

    it('reports progress against the manifest, not the window', async () => {
        manifestJson = JSON.stringify({
            root: 'Holiday',
            files: [
                { id: 'doc:0', rel: 'a.jpg', size: 400 },
                { id: 'doc:1', rel: 'b.jpg', size: 600 },
            ],
        });

        await importFolderWithParentID('root');

        expect(mocks.updateTransferProgress).toHaveBeenLastCalledWith({
            id: 'import',
            direction: 'up',
            progress: 100,
            bytes: 1000,
            total: 1000,
            itemsDone: 2,
            itemsTotal: 2,
        });
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'done' });
    });

    it('shows the walk is running before the manifest arrives', async () => {
        await importFolderWithParentID('root');

        expect(mocks.pushTransferStart).toHaveBeenCalledWith({
            id: 'import',
            direction: 'up',
            name: 'Reading folder…',
            total: 0,
        });
    });

    it('does nothing at all when the picker is dismissed', async () => {
        manifestJson = '';

        await importFolderWithParentID('root');

        expect(app.ImportResolveFolders).not.toHaveBeenCalled();
        expect(app.UploadToDriveFS).not.toHaveBeenCalled();
        expect(mocks.openImportOptionsModal).not.toHaveBeenCalled();
        expect(mocks.notify).not.toHaveBeenCalled();
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'canceled' });
    });

    it('uploads nothing when the import dialog is dismissed', async () => {
        mocks.openImportOptionsModal.mockResolvedValue(null);

        await importFolderWithParentID('root');

        expect(app.ImportResolveFolders).not.toHaveBeenCalled();
        expect(materialized).toEqual([]);
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'canceled' });
    });

    it('stops when the tree cannot be created, rather than uploading into the wrong place', async () => {
        manifestJson = manifestOf('Holiday', ['sub/a.jpg']);
        app.ImportResolveFolders.mockRejectedValue(new Error('folder projection failed'));

        await importFolderWithParentID('root');

        expect(materialized).toEqual([]);
        expect(app.UploadToDriveFS).not.toHaveBeenCalled();
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'failed' });
    });

    it('leaves the desktop picker alone', async () => {
        delete host.wails;
        app.SelectFolder.mockResolvedValue('');

        await importFolderWithParentID('root');

        expect(app.SelectFolder).toHaveBeenCalled();
        expect(materialized).toEqual([]);
    });
});

describe('the live drive decides what is already there', () => {
    const threeFiles = () => JSON.stringify({
        root: 'Holiday',
        files: [
            { id: 'doc:0', rel: 'a.jpg', size: 100, modified: 1 },
            { id: 'doc:1', rel: 'b.jpg', size: 300, modified: 2 },
            { id: 'doc:2', rel: 'c.jpg', size: 600, modified: 3 },
        ],
    });

    it('skips only the entries the live check confirms, and counts them as settled', async () => {
        manifestJson = threeFiles();
        app.ImportFindExisting.mockResolvedValue([{ index: 0, folder_id: 'live-1' }]);

        await importFolderWithParentID('root');

        expect(app.ImportFindExisting).toHaveBeenCalledWith(1, 'root', [
            { folder_path: 'Holiday', name: 'a.jpg', size: 100, encrypted: false },
            { folder_path: 'Holiday', name: 'b.jpg', size: 300, encrypted: false },
            { folder_path: 'Holiday', name: 'c.jpg', size: 600, encrypted: false },
        ]);
        // The options dialog counts the whole manifest, the job decides after.
        expect(materialized).toEqual([['doc:1', 'doc:2']]);
        expect(mocks.updateTransferProgress).toHaveBeenLastCalledWith({
            id: 'import',
            direction: 'up',
            progress: 100,
            bytes: 900,
            total: 900,
            itemsDone: 2,
            itemsTotal: 2,
        });
        expect(mocks.updateTransferName).toHaveBeenLastCalledWith({
            id: 'import',
            direction: 'up',
            name: 'Imported 2 files · 1 file already on the drive',
        });
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'done' });
    });

    it('uploads a file the ledger says was edited since it was imported', async () => {
        manifestJson = threeFiles();
        app.ImportFindExisting.mockResolvedValue([{ index: 0, folder_id: 'live-1' }]);
        app.ImportLedgerVeto.mockResolvedValue([true]);

        await importFolderWithParentID('root');

        expect(app.ImportLedgerVeto).toHaveBeenCalledWith(1, 'root', [
            { folder_path: 'Holiday', name: 'a.jpg', size: 100, modified: 1, encrypted: false },
        ]);
        expect(materialized).toEqual([['doc:0', 'doc:1', 'doc:2']]);
    });

    it('uploads everything when the live check fails, rather than skipping on a guess', async () => {
        manifestJson = threeFiles();
        app.ImportFindExisting.mockRejectedValue(new Error('no answer'));

        await importFolderWithParentID('root');

        expect(materialized).toEqual([['doc:0', 'doc:1', 'doc:2']]);
        expect(app.ImportLedgerVeto).not.toHaveBeenCalled();
    });

    it('asks the live check about the destination it was given, not any other', async () => {
        manifestJson = manifestOf('Holiday', ['a.jpg']);

        await importFolderWithParentID('trips');

        expect(app.ImportFindExisting).toHaveBeenCalledWith(1, 'trips', expect.any(Array));
        expect(app.UploadToDriveFS).toHaveBeenCalledTimes(1);
    });

    it('shows the summary plurally when several files were already there', async () => {
        manifestJson = threeFiles();
        app.ImportFindExisting.mockResolvedValue([{ index: 0, folder_id: 'x' }, { index: 1, folder_id: 'x' }]);

        await importFolderWithParentID('root');

        expect(mocks.updateTransferName).toHaveBeenLastCalledWith({
            id: 'import',
            direction: 'up',
            name: 'Imported 1 file · 2 files already on the drive',
        });
    });

    it('creates no folder and starts no window when everything is already on the drive', async () => {
        manifestJson = JSON.stringify({
            root: 'Holiday',
            files: [{ id: 'doc:0', rel: 'a.jpg', size: 5, modified: 9 }],
        });
        app.ImportFindExisting.mockResolvedValue([{ index: 0, folder_id: 'x' }]);

        await importFolderWithParentID('root');

        expect(app.ImportResolveFolders).not.toHaveBeenCalled();
        expect(app.CreateFolder).not.toHaveBeenCalled();
        expect(app.UploadToDriveFS).not.toHaveBeenCalled();
        expect(materialized).toEqual([]);
        expect(mocks.updateTransferName).toHaveBeenLastCalledWith({
            id: 'import',
            direction: 'up',
            name: 'Everything is already on the drive',
        });
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'done' });
    });

    it('does not count bytes that were skipped or failed as transferred', async () => {
        manifestJson = JSON.stringify({
            root: 'Holiday',
            files: [
                { id: 'doc:0', rel: 'a.jpg', size: 100 },
                { id: 'doc:1', rel: 'b.jpg', size: 200 },
                { id: 'doc:2', rel: 'c.jpg', size: 300 },
            ],
        });
        app.ImportFindExisting.mockResolvedValue([{ index: 0, folder_id: 'x' }]);
        app.UploadToDriveFS.mockResolvedValue({
            result: { ok: true },
            files: [{ name: '/cache/doc:2' }],
            failures: [{ index: 0, name: '/cache/doc:1', message: 'quota exceeded', code: 'upload_failed', retryable: true }],
        });

        await importFolderWithParentID('root');

        expect(mocks.updateTransferProgress).toHaveBeenLastCalledWith(expect.objectContaining({
            bytes: 300,
            total: 500,
        }));
    });
});

describe('the ledger records only what is proven uploaded', () => {
    it('records exactly the files that uploaded, window by window', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b', 'c', 'd', 'e']);
        app.UploadToDriveFS
            .mockResolvedValueOnce({
                result: { ok: true },
                files: [{ name: '/cache/doc:0' }, { name: '/cache/doc:2' }, { name: '/cache/doc:3' }],
                failures: [{ index: 1, name: '/cache/doc:1', message: 'quota exceeded', code: 'upload_failed', retryable: true }],
            })
            .mockResolvedValue({ result: { ok: true }, files: [{ name: '/cache/doc:4' }] });

        await importFolderWithParentID('root');

        const recorded = app.ImportLedgerRecord.mock.calls
            .map(([, , entries]) => (entries as { name: string }[]).map((entry) => entry.name))
            .filter((names) => names.length);
        expect(recorded).toEqual([['a', 'c', 'd'], ['e']]);
    });

    it('records nothing when the upload answer does not account for every file', async () => {
        // One file is named as uploaded and the other is named nowhere. Neither
        // is remembered: a file only goes on the ledger on proof it went up.
        manifestJson = manifestOf('Holiday', ['a', 'b']);
        app.UploadToDriveFS.mockResolvedValueOnce({ result: { ok: true }, files: [{ name: '/cache/doc:0' }] });

        await importFolderWithParentID('root');

        const recorded = app.ImportLedgerRecord.mock.calls.flatMap(([, , entries]) => entries as unknown[]);
        expect(recorded).toEqual([]);
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'failed' });
    });

    it('does not record a file that changed while it was being copied', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b']);
        changedWhileCopying = new Set(['doc:0']);

        await importFolderWithParentID('root');

        const [paths] = app.UploadToDriveFS.mock.calls[0];
        expect(paths).toEqual(['/cache/doc:0', '/cache/doc:1']);
        const recorded = app.ImportLedgerRecord.mock.calls
            .flatMap(([, , entries]) => (entries as { name: string }[]).map((entry) => entry.name));
        expect(recorded).toEqual(['b']);
    });

    it('records nothing and does not touch the upload when the drive is switched while copying', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b']);
        onMaterialize = () => {
            state.activeChannel = { id: 2, title: 'Other drive', kind: 'shared' };
        };

        await importFolderWithParentID('root');

        expect(app.UploadToDriveFS).not.toHaveBeenCalled();
        expect(app.ImportLedgerRecord.mock.calls.flatMap(([, , entries]) => entries as unknown[])).toEqual([]);
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'failed' });
    });

    it('makes zero upload calls when cancelled during materialization', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b']);
        onMaterialize = () => {
            state.cancelingUpload = true;
        };

        await importFolderWithParentID('root');

        expect(app.UploadToDriveFS).not.toHaveBeenCalled();
        expect(released).toEqual([['doc:0', 'doc:1']]);
        expect(mocks.markTransferDone).toHaveBeenCalledWith({ id: 'import', direction: 'up', status: 'canceled' });
    });
});

describe('failure semantics from the backend', () => {
    it('never offers an outcome_unknown failure for retry, even when the backend says retryable', async () => {
        manifestJson = manifestOf('Holiday', ['a']);
        app.UploadToDriveFS.mockResolvedValueOnce({
            result: { ok: true },
            files: [],
            failures: [{ index: 0, name: '/cache/doc:0', message: 'may have landed', code: 'outcome_unknown', retryable: true }],
        });

        await importFolderWithParentID('root');

        mocks.notify.mock.calls[0][0].action.run();
        const view = get(failedUploadsModal.state);
        expect(view.payload?.failures[0]).toMatchObject({ code: 'outcome_unknown', retryable: false });
        expect(view.payload?.retry).toBeUndefined();
    });

    it('keeps a sibling that uploaded when another file in the same answer failed', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b', 'c']);
        app.UploadToDriveFS.mockResolvedValueOnce({
            result: { ok: true },
            files: [{ name: '/cache/doc:0' }, { name: '/cache/doc:2' }],
            failures: [{ index: 1, name: '/cache/doc:1', message: 'quota exceeded', code: 'quota', retryable: false }],
        });

        await importFolderWithParentID('root');

        expect(mocks.updateTransferName).toHaveBeenLastCalledWith({
            id: 'import',
            direction: 'up',
            name: 'Imported 2 files · 1 failed',
        });
        const recorded = app.ImportLedgerRecord.mock.calls.flatMap(([, , entries]) => (entries as { name: string }[]).map((entry) => entry.name));
        expect(recorded).toEqual(['a', 'c']);
    });

    it('keeps the backend retryable flag and code on a failed file', async () => {
        manifestJson = manifestOf('Holiday', ['a']);
        app.UploadToDriveFS.mockResolvedValueOnce({
            result: { ok: true },
            files: [],
            failures: [{ index: 0, name: '/cache/doc:0', message: 'quota exceeded', code: 'quota', retryable: false }],
        });

        await importFolderWithParentID('root');

        mocks.notify.mock.calls[0][0].action.run();
        expect(get(failedUploadsModal.state).payload?.failures[0]).toMatchObject({ code: 'quota', retryable: false });
    });

    it('keeps every failed file, not three, and offers them for review', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b', 'c', 'd', 'e', 'f']);
        app.UploadToDriveFS.mockImplementation(async (paths: string[]) => ({
            result: { ok: true },
            files: [],
            failures: paths.map((path, index) => ({ index, name: path, message: `error ${index}`, code: 'upload_failed', retryable: true })),
        }));

        await importFolderWithParentID('root');

        const [notice] = mocks.notify.mock.calls.map(([options]) => options);
        expect(notice.level).toBe('error');
        expect(notice.action.label).toBe('Review failed files');
        notice.action.run();
        const view = get(failedUploadsModal.state);
        expect(view.open).toBe(true);
        expect(view.payload?.failures).toHaveLength(6);
        expect(view.payload?.failures[5]).toMatchObject({ name: 'f', retryable: true });
        expect(view.payload?.paths).toEqual(['a', 'b', 'c', 'd', 'e', 'f']);
    });

    it('marks a file the drive does not store on this device as not retryable', async () => {
        manifestJson = manifestOf('Holiday', ['a']);
        notOnDevice = new Set(['doc:0']);

        await importFolderWithParentID('root');

        expect(app.UploadToDriveFS).not.toHaveBeenCalled();
        mocks.notify.mock.calls[0][0].action.run();
        expect(get(failedUploadsModal.state).payload?.failures[0]).toMatchObject({ code: 'not_on_device', retryable: false });
    });
});

describe('retrying the failed list', () => {
    it('retries only the retryable failed files, through a fresh live check', async () => {
        manifestJson = manifestOf('Holiday', ['a', 'b', 'c']);
        app.UploadToDriveFS.mockResolvedValueOnce({
            result: { ok: true },
            files: [{ name: '/cache/doc:0' }, { name: '/cache/doc:2' }],
            failures: [{ index: 1, name: '/cache/doc:1', message: 'quota exceeded', code: 'upload_failed', retryable: true }],
        });
        await importFolderWithParentID('root');
        const createdBefore = createdFolders.length;
        materialized = [];
        app.UploadToDriveFS.mockClear();
        app.ImportLedgerRecord.mockClear();
        app.ImportFindExisting.mockClear();

        mocks.notify.mock.calls[0][0].action.run();
        await retryFailedUploads();

        expect(app.ImportFindExisting).toHaveBeenCalledTimes(1);
        expect(liveNames(app.ImportFindExisting.mock.calls[0])).toEqual(['b']);
        expect(materialized).toEqual([['doc:1']]);
        expect(app.UploadToDriveFS).toHaveBeenCalledTimes(1);
        const [paths, parentIDs] = app.UploadToDriveFS.mock.calls[0];
        expect(paths).toEqual(['/cache/doc:1']);
        expect(parentIDs).toEqual(['f1']);
        // The folders were made by the import; reuse means the retry adds none.
        expect(createdFolders).toHaveLength(createdBefore);
        const recorded = app.ImportLedgerRecord.mock.calls.flatMap(([, , entries]) => (entries as { name: string }[]).map((entry) => entry.name));
        expect(recorded).toEqual(['b']);
        expect(get(failedUploadsModal.state).open).toBe(false);
    });

    it('lets go of the previous failed list when the next import starts', async () => {
        manifestJson = manifestOf('Holiday', ['a']);
        app.UploadToDriveFS.mockResolvedValueOnce({
            result: { ok: true },
            files: [],
            failures: [{ index: 0, name: '/cache/doc:0', message: 'quota exceeded', code: 'upload_failed', retryable: true }],
        });
        await importFolderWithParentID('root');
        expect(get(failedUploadReports)).toHaveLength(1);

        manifestJson = manifestOf('Holiday', ['z']);
        await importFolderWithParentID('root');

        expect(get(failedUploadReports)).toEqual([]);
    });

    it('reuses the folder the drive already has rather than creating it again', async () => {
        manifestJson = manifestOf('Holiday', ['a']);

        await importFolderWithParentID('root');

        expect(app.ImportResolveFolders).toHaveBeenCalledWith(1, 'root', ['Holiday']);
        expect(app.CreateFolder).not.toHaveBeenCalled();
    });

    it('reports how many temporary camera files were skipped inside the one result entry', async () => {
        manifestJson = JSON.stringify({
            root: 'Holiday',
            files: [{ id: 'doc:0', rel: 'a.jpg', size: 100 }],
            skipped: 2,
        });

        await importFolderWithParentID('root');

        expect(mocks.notify).toHaveBeenCalledTimes(1);
        expect(mocks.notify.mock.calls[0][0].body).toContain('2 temporary camera files skipped');
    });
});
