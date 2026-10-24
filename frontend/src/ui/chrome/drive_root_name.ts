// What a drive's root is called wherever the app names it: the breadcrumb, the
// move dialog, the AI scope and the organize preview. The personal drive is
// "My Drive" in every language; a shared drive is named by its own title.
import { derived, get } from 'svelte/store';
import type { Translator } from '../../i18n/i18n';
import { t, t_store } from '../../i18n/i18n';
import { sidebarState, type SidebarState } from '../sidebar/sidebar_store';
import type { DriveChannel } from '../../types';

/** The root name for one drive: "My Drive" for the personal drive, otherwise the drive's own title. */
export function drive_root_name(drive: Pick<DriveChannel, 'kind' | 'title'> | null | undefined, translate: Translator): string {
    if (!drive || drive.kind === 'personal') return translate('chrome.my_drive');
    return drive.title.trim() || translate('sidebar.untitled');
}

/** Every drive the sidebar lists, personal first. */
export function sidebar_drives(sidebar: SidebarState): DriveChannel[] {
    return [...sidebar.personal, ...sidebar.shared];
}

/** The drive the sidebar has open, or null before the drive list loads. */
export function active_drive_of(sidebar: SidebarState): DriveChannel | null {
    return sidebar_drives(sidebar).find((drive) => drive.id === sidebar.activeChannelId) ?? null;
}

/** Live name of the open drive's root. Reads the sidebar, so a drive switch updates it. */
export const active_drive_root_name = derived(
    [sidebarState, t_store],
    ([sidebar, translate]) => drive_root_name(active_drive_of(sidebar), translate),
);

/** The root name of a drive by id, read now. For code that is not a component. */
export function drive_root_name_for_id(channel_id: number, translate: Translator = t): string {
    const drive = sidebar_drives(get(sidebarState)).find((entry) => entry.id === channel_id) ?? null;
    return drive_root_name(drive, translate);
}
