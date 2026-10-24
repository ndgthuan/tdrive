// Gate for actions that change a shared drive. A viewer is told once, in words,
// why nothing happened; every other caller gets true and the normal flow runs.
import { get } from 'svelte/store';
import { t } from '../../i18n/i18n';
import { notify } from '../../modules/notifications';
import { sidebarState } from '../sidebar/sidebar_store';
import { drive_access_for, type DriveAccess } from './members_store';

/** The caller's access to a drive, read now. Falls back to the drive that is open. */
export function access_now(driveId?: number | null): DriveAccess {
    const sidebar = get(sidebarState);
    const id = Number(driveId || sidebar.activeChannelId || 0);
    const shared = sidebar.shared.some((drive) => drive.id === id);
    return drive_access_for(id, shared);
}

/**
 * Whether an action that writes to the drive (upload, rename, move, new folder,
 * delete, restore) may run. A viewer gets a notice and false. An unknown role
 * stays true here, because Telegram still refuses what the caller may not do.
 */
export function allow_edit_in(driveId?: number | null, options: { notifyUser?: boolean } = {}): boolean {
    const access = access_now(driveId);
    if (!access.shared || !access.known || access.canUpload) return true;
    if (options.notifyUser !== false) {
        notify({ level: 'info', title: t('members.view_only'), body: t('members.view_only_upload') });
    }
    return false;
}

/** Whether the caller may delete items the caller did not upload in a drive. */
export function may_delete_others_in(driveId?: number | null): boolean {
    const access = access_now(driveId);
    return !access.shared || !access.known || access.canDeleteOthers;
}
