// The live side of a folder import: which drive folders to reuse or create, and
// which entries are already on the drive right now. These answers come from the
// drive itself, never from the ledger.
import {
    ImportFindExisting as rawImportFindExisting,
    ImportResolveFolders as rawImportResolveFolders,
} from "../../bindings/TDrive/internal/app/app";
import { invokeBackend } from "./gateway";

/** One file of an import, placed by its folder path from the picked folder down. size is -1 when unknown. */
export interface ImportLiveEntry {
    folder_path: string;
    name: string;
    size: number;
    encrypted: boolean;
}

/** An entry the drive already holds live, by its index in the request. */
export interface ImportLiveMatch {
    index: number;
    folder_id: string;
}

/**
 * Returns the drive folder id for each path under parentId, reusing live folders
 * with the same name and creating the rest. Rejects when a file holds a folder's
 * name or two source names collide.
 */
export async function importResolveFolders(driveId: number, parentId: string, folderPaths: string[]): Promise<Map<string, string>> {
    const ids = await invokeBackend(rawImportResolveFolders, driveId, parentId, folderPaths);
    const resolved = new Map<string, string>();
    for (const [path, id] of Object.entries(ids ?? {})) {
        if (typeof id === "string" && id) resolved.set(path, id);
    }
    return resolved;
}

/** Which entries already exist live under parentId with the same size and encryption policy. */
export async function importFindExisting(driveId: number, parentId: string, entries: ImportLiveEntry[]): Promise<ImportLiveMatch[]> {
    const matches = await invokeBackend(rawImportFindExisting, driveId, parentId, entries);
    return Array.isArray(matches) ? matches.map((match) => ({ index: Number(match.index), folder_id: String(match.folder_id) })) : [];
}
