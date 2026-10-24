// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Where a link download is saved: a drive and a folder inside it. The choice is
// remembered per device, so the next download starts where the last one went.
import type { SidebarState } from '../sidebar/sidebar_store';

/** One folder on the way from the drive root to the chosen folder. */
export interface DestinationCrumb {
    id: string;
    name: string;
}

/** The chosen drive and folder. An empty folder_id is that drive's root, and path is then empty. */
export interface DownloadDestination {
    drive_id: number;
    folder_id: string;
    path: DestinationCrumb[];
}

export const DESTINATION_STORAGE_KEY = 'tdrive.downloader.destination';

/** The drive root, the destination every download starts from. */
export function destination_at_root(drive_id: number): DownloadDestination {
    return { drive_id, folder_id: '', path: [] };
}

function parse_crumb(value: unknown): DestinationCrumb | null {
    if (!value || typeof value !== 'object') return null;
    const record = value as Record<string, unknown>;
    if (typeof record.id !== 'string' || record.id === '') return null;
    if (typeof record.name !== 'string' || record.name === '') return null;
    return { id: record.id, name: record.name };
}

/** Checks a stored value field by field. Anything malformed is rejected, never guessed at. */
export function parse_destination(value: unknown): DownloadDestination | null {
    if (!value || typeof value !== 'object') return null;
    const record = value as Record<string, unknown>;
    const drive_id = record.drive_id;
    if (typeof drive_id !== 'number' || !Number.isInteger(drive_id) || drive_id <= 0) return null;
    if (typeof record.folder_id !== 'string') return null;
    if (!Array.isArray(record.path)) return null;
    const path: DestinationCrumb[] = [];
    for (const entry of record.path) {
        const crumb = parse_crumb(entry);
        if (!crumb) return null;
        path.push(crumb);
    }
    if (record.folder_id === '') return destination_at_root(drive_id);
    if (path.length === 0 || path[path.length - 1].id !== record.folder_id) return null;
    return { drive_id, folder_id: record.folder_id, path };
}

/** The remembered destination, or null when none is stored or storage cannot be read. */
export function read_saved_destination(): DownloadDestination | null {
    try {
        const raw = window.localStorage.getItem(DESTINATION_STORAGE_KEY);
        if (!raw) return null;
        return parse_destination(JSON.parse(raw));
    } catch {
        return null;
    }
}

/** Remembers the destination. A storage failure only means it is not remembered. */
export function save_destination(destination: DownloadDestination): void {
    try {
        window.localStorage.setItem(DESTINATION_STORAGE_KEY, JSON.stringify(destination));
    } catch {
        // Private windows and blocked storage: the choice simply is not kept.
    }
}

/**
 * The destination a dialog opens with: the remembered one while its drive is still
 * joined, otherwise My Drive's root. Before the drive list loads, the active drive
 * stands in for My Drive.
 */
export function starting_destination(saved: DownloadDestination | null, sidebar: SidebarState): DownloadDestination {
    const drive_ids = [...sidebar.personal, ...sidebar.shared].map((drive) => drive.id);
    if (saved && drive_ids.includes(saved.drive_id)) return saved;
    const personal_id = sidebar.personal[0]?.id ?? sidebar.activeChannelId ?? 0;
    return destination_at_root(personal_id);
}

/** Path label for the picker button, such as "My Drive / Videos / Clips". */
export function destination_place(drive_name: string, destination: DownloadDestination): string {
    return [drive_name, ...destination.path.map((crumb) => crumb.name)].join(' / ');
}
