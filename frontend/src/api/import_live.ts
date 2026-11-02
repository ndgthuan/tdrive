// The live side of a folder import: which drive folders to reuse or create, and
// which entries are already on the drive right now. These answers come from the
// drive itself, never from the ledger.
import {
    ImportFindExisting as rawImportFindExisting,
    ImportReplaceFile as rawImportReplaceFile,
    ImportResolveFolders as rawImportResolveFolders,
} from "../../bindings/TDrive/internal/app/app";
import { invokeBackend } from "./gateway";

/**
 * One file of an import, placed by its folder path from the picked folder down. size is -1 when unknown.
 * folder marks a folder the import will create: it never matches, but its name is taken, so no copy lands on it.
 */
export interface ImportLiveEntry {
    folder_path: string;
    name: string;
    size: number;
    encrypted: boolean;
    folder: boolean;
}

/**
 * An entry the drive already holds live, by its index in the request. name_taken
 * means the drive holds a different file under that name, so the entry must not be
 * sent as is. existing_msg_id and existing_size describe that live file, and
 * suggested_name is the free copy name for it ("" when none is free).
 */
export interface ImportLiveMatch {
    index: number;
    folder_id: string;
    name_taken: boolean;
    existing_msg_id: number;
    existing_size: number;
    suggested_name: string;
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

/**
 * Which entries already exist live under parentId with the same size and encryption
 * policy, and which are name clashes: a live file with that name but different content.
 */
export async function importFindExisting(driveId: number, parentId: string, entries: ImportLiveEntry[]): Promise<ImportLiveMatch[]> {
    const matches = await invokeBackend(rawImportFindExisting, driveId, parentId, entries);
    return Array.isArray(matches)
        ? matches.map((match) => ({
            index: Number(match.index),
            folder_id: String(match.folder_id),
            name_taken: Boolean(match.name_taken),
            existing_msg_id: Number(match.existing_msg_id ?? 0),
            existing_size: Number(match.existing_size ?? 0),
            suggested_name: String(match.suggested_name ?? ""),
        }))
        : [];
}

/**
 * Puts newMsgId in place of oldMsgId: the old file goes to the trash and the new
 * one takes the name. Call it only after the new file is uploaded under its copy name.
 */
export async function importReplaceFile(driveId: number, oldMsgId: number, newMsgId: number, name: string): Promise<void> {
    await invokeBackend(rawImportReplaceFile, driveId, oldMsgId, newMsgId, name);
}
