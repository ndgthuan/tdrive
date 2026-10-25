// Pure rules behind the automatic backup pane. Nothing here touches the DOM,
// the bindings or the native bridge, so each rule can be tested directly.
// The rules mirror backend/autobackup: a name the backend would refuse is
// refused here first, with the same words.

import type { AutoBackupPermission, AutoBackupPhase } from '../../api/auto_backup';

/** The live progress of the current upload run, when the caller can supply it. */
export interface AutoBackupProgress {
    done: number;
    total: number;
    currentName: string;
    currentBytes: number;
    currentBytesDone?: number;
    paused: boolean;
    /** Whether automatic backup is on; absent from older callers, which count as on. */
    enabled?: boolean;
    folderName?: string;
}

const MAX_FOLDER_NAME_BYTES = 200;
const FORBIDDEN_FOLDER_CHARACTERS = /[<>:"/\\|?*]/;

/**
 * Checks a folder rename. An empty value is valid: it restores the device
 * default. Returns the i18n key of the problem, or null when the name is fine.
 */
export function folderNameProblemKey(value: string): 'folder_name_invalid' | null {
    const trimmed = value.trim();
    if (trimmed === '') return null;
    if (FORBIDDEN_FOLDER_CHARACTERS.test(trimmed) || hasControlCharacter(trimmed)) return 'folder_name_invalid';
    const name = trimmed.replace(/[. ]+$/, '');
    if (name === '' || name === '.' || name === '..') return 'folder_name_invalid';
    if (new TextEncoder().encode(name).length > MAX_FOLDER_NAME_BYTES) return 'folder_name_invalid';
    return null;
}

function hasControlCharacter(value: string): boolean {
    for (const character of value) {
        const code = character.codePointAt(0) ?? 0;
        if (code < 0x20 || code === 0x7f) return true;
    }
    return false;
}

/** The name the user typed, trimmed, or '' to mean "use the device default". */
export function normalizedFolderName(value: string): string {
    return value.trim();
}

/** Translation key for the one line that says what the backup is doing. */
export function phaseMessageKey(phase: AutoBackupPhase): string {
    switch (phase) {
        case 'off': return 'auto_backup.phase_off';
        case 'needs_permission': return 'auto_backup.phase_needs_permission';
        case 'paused': return 'auto_backup.phase_paused';
        case 'waiting_for_wifi': return 'auto_backup.phase_waiting_for_wifi';
        case 'waiting_for_charging': return 'auto_backup.phase_waiting_for_charging';
        case 'running': return 'auto_backup.phase_running';
        case 'failed': return 'auto_backup.phase_failed';
        case 'up_to_date': return 'auto_backup.phase_up_to_date';
    }
}

/** Whether the only way forward is the system settings page. */
export function permissionNeedsSettings(permission: AutoBackupPermission): boolean {
    return permission === 'denied';
}

/** Whether the grant is partial, so the pane should show the limited notice. */
export function permissionIsLimited(permission: AutoBackupPermission): boolean {
    return permission === 'limited';
}

/** Progress as a fraction in [0, 1]. Unknown or impossible totals read as 0. */
export function progressFraction(done: number, total: number): number {
    if (!Number.isFinite(done) || !Number.isFinite(total) || total <= 0) return 0;
    return Math.min(1, Math.max(0, done / total));
}

/** Whole percentage for a progress bar, rounded and bounded to 0-100. */
export function progressPercent(done: number, total: number): number {
    return Math.round(progressFraction(done, total) * 100);
}

/** The folder path the pane shows under "Saved to", e.g. "My Drive / iPhone". */
export function folderPathParts(folderName: string): { root: string; folder: string } {
    return { root: 'My Drive', folder: folderName };
}

/** Byte count as a short human string, using binary units like the rest of the app. */
export function formatBytes(bytes: number): string {
    if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit += 1;
    }
    return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

/** The upload tray's view of automatic backup: whether its row shows, and its counts. */
export interface AutoBackupTransferView { active: boolean; complete: boolean; done: number; total: number; percent: number; }

/**
 * A row shows only while automatic backup is on, not paused and has items.
 * It is complete once every item is done; the caller then finishes the row.
 */
export function transferView(progress: AutoBackupProgress | null): AutoBackupTransferView {
    if (!progress || progress.enabled === false || progress.paused || progress.total <= 0) {
        return { active: false, complete: false, done: 0, total: 0, percent: 0 };
    }
    const done = Math.min(Math.max(0, progress.done), progress.total);
    const complete = done >= progress.total;
    return { active: !complete, complete, done, total: progress.total, percent: progressPercent(done, progress.total) };
}
