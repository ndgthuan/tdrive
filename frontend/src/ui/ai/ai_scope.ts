// The drive and folder the assistant works in. By default that is the drive and
// folder open in the app right now. The composer chip can pick another drive or
// the drive root; that choice holds only while the same drive stays open, so a
// drive switch returns the chip to the open drive.
import { derived, get, writable } from 'svelte/store';
import { state } from '../../state';
import { sidebarState } from '../sidebar/sidebar_store';
import { active_drive_of, sidebar_drives } from '../chrome/drive_root_name';
import type { Scope } from './ai_api';

export interface ScopeOverride {
    /** The drive the user picked. */
    drive_id: number;
    /** True keeps the open folder; false means the drive root. */
    folder: boolean;
    /** The drive that was open when the user picked, so the choice ends with the drive switch. */
    open_drive_id: number;
}

export const scope_override = writable<ScopeOverride | null>(null);

/** The id of the drive open in the app, or 0 before one is open. */
export function open_drive_id(): number {
    return state.activeChannel?.id ?? get(sidebarState).activeChannelId ?? 0;
}

/** The drive the assistant works in now, and whether it covers the open folder. */
export function effective_scope(override: ScopeOverride | null, open_id: number): { drive_id: number; folder: boolean } {
    if (override && override.open_drive_id === open_id) {
        return { drive_id: override.drive_id, folder: override.folder };
    }
    return { drive_id: open_id, folder: true };
}

/** The chip's drive, reactive to the open drive and to picks. */
export const scope_drive = derived([sidebarState, scope_override], ([sidebar, override]) => {
    const open_id = state.activeChannel?.id ?? sidebar.activeChannelId ?? 0;
    const choice = effective_scope(override, open_id);
    const drive = sidebar_drives(sidebar).find((entry) => entry.id === choice.drive_id) ?? active_drive_of(sidebar);
    return { ...choice, drive };
});

/** Scope for a request, read at the moment it is sent. */
export function build_scope(): Scope {
    const open_id = open_drive_id();
    const choice = effective_scope(get(scope_override), open_id);
    const folder_id = choice.folder && choice.drive_id === open_id ? state.currentFolderId || '' : '';
    return { channel_id: choice.drive_id, folder_id };
}

/** Picks a drive, or the open drive's root or folder, for the assistant. */
export function pick_scope(drive_id: number, folder: boolean): void {
    scope_override.set({ drive_id, folder, open_drive_id: open_drive_id() });
}
