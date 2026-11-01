// The import ledger: which files a folder import last uploaded to a destination.
// It only ever vetoes a skip. Live drive state decides whether a file is already
// there, so the ledger cannot make a missing file look present.
import {
    ImportLedgerForget as rawImportLedgerForget,
    ImportLedgerRecord as rawImportLedgerRecord,
    ImportLedgerVeto as rawImportLedgerVeto,
} from "../../bindings/TDrive/internal/app/app";
import { invokeBackend } from "./gateway";

/** One uploaded file. folder_path starts at the picked folder; modified is 0 when unknown. */
export interface ImportLedgerEntry {
    folder_path: string;
    name: string;
    size: number;
    modified: number;
    encrypted: boolean;
}

/**
 * For each entry, whether the ledger forbids skipping it. A true value means the
 * file changed since the import that recorded it, so it must upload again.
 */
export async function importLedgerVeto(driveId: number, parentId: string, entries: ImportLedgerEntry[]): Promise<boolean[]> {
    const vetoed = await invokeBackend(rawImportLedgerVeto, driveId, parentId, entries);
    return Array.isArray(vetoed) ? vetoed.map((value) => value === true) : entries.map(() => true);
}

/**
 * Remembers files whose staged bytes uploaded. Pass an empty list to retry a save
 * that failed earlier, which the job does once more at its end.
 */
export async function importLedgerRecord(driveId: number, parentId: string, entries: ImportLedgerEntry[]): Promise<void> {
    await invokeBackend(rawImportLedgerRecord, driveId, parentId, entries);
}

/** Drops everything remembered for a drive, for when it is left or removed. */
export async function importLedgerForget(driveId: number): Promise<void> {
    await invokeBackend(rawImportLedgerForget, driveId);
}
