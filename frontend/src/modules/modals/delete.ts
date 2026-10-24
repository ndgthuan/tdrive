import { isMobilePlatform } from '../../api';
// Delete modal for TDrive frontend.
//
// Delete does not destroy anything: the backend moves the file or folder into
// the Trash (backend/services/file/mutations.go), where it stays restorable
// until the Trash is emptied or the retention sweep purges it. The dialog says
// exactly that, and a successful delete offers a one-tap Undo that restores the
// same object by its trash id (`f:<msgId>` for a file, the folder's own `d:…`
// id for a folder).

import { deleteFile, deleteFolder, type OperationResult } from '../../api';
import { restoreFromTrash } from '../../api/trash';
import { invalidateFolderIndex, state } from '../../state';
import { clearSelection } from '../selection';
import { allow_edit_in } from '../../ui/members/drive_guard';
import { ensureNotInsideDeletedFolder } from '../navigation';
import { dismissNotification, notify } from '../notifications';
import { humanizeBackendError } from '../errors';
import { appActions } from '../app_actions';
import { callWithPasswordRetry } from './encryption_password';
import { closeDeleteModalView, openDeleteModalView } from '../../ui/modals/delete_modal_store';
import { markRowsBusy } from '../../ui/file_list/busy_rows';
import { t } from '../../i18n/i18n';
import type {
    FileCommandItem,
    FileCommandTarget,
    FolderCommandItem,
} from '../../ui/file_list/types';
let pendingTarget: FileCommandTarget | null = null;
let pendingChannelId: number | null = null;

/**
 * One line for a whole batch. A selection of twelve used to answer with twelve
 * toasts into a stack that holds two on a phone, so eleven of them evicted each
 * other on the way past and the user read whichever happened to be last.
 */
function failureBatchTitle(items: FileCommandItem[]): string {
    if (items.length === 1) return failureTitle(items[0]);
    const folders = items.filter(isFolder).length;
    const files = items.length - folders;
    if (folders === 0) return t('messages.delete_failed_files', { count: files });
    if (files === 0) return t('messages.delete_failed_folders', { count: folders });
    return t('messages.delete_failed_items', { count: items.length });
}

function failureTitle(item: FileCommandItem): string {
    const name = item.name.trim();
    if (!name) {
        return item.type === 'folder'
            ? t('messages.delete_failed_folder_unnamed')
            : t('messages.delete_failed_file_unnamed');
    }
    return item.type === 'folder'
        ? t('messages.delete_failed_folder_named', { name })
        : t('messages.delete_failed_file_named', { name });
}

function isFolder(item: FileCommandItem): item is FolderCommandItem {
    return item.type === 'folder';
}

/** The trash handle for an item: a folder carries its own `d:…` id, a file is addressed by message id. */
function trashObjectId(item: FileCommandItem): string {
    return item.type === 'folder' ? item.id : `f:${item.id}`;
}

function activeChannelId(): number {
    return Number(state.activeChannel?.id ?? 0);
}

function deleteInChannel(channelId: number, operation: () => Promise<OperationResult>): Promise<OperationResult> {
    // Password retry may invoke this callback again after a modal interaction.
    // Recheck at each invocation because the backend uses the active drive.
    if (activeChannelId() !== channelId) {
        return Promise.resolve({
            ok: false,
            error: { code: 'canceled', message: t('messages.delete_drive_changed') },
        });
    }
    return operation();
}

export function openDeleteModal(target: FileCommandTarget): void {
    const activeId = activeChannelId();
    if (!allow_edit_in(target.type === 'bulk' ? undefined : target.channelId, { notifyUser: true })) return;
    const channels = target.type === 'bulk'
        ? target.items.map((item) => item.channelId ?? activeId)
        : [target.channelId ?? activeId];
    const channelId = channels[0] ?? activeId;
    if (channelId !== activeId || channels.some((id) => id !== channelId)) {
        notify({ level: 'info', title: t('messages.switch_drive_delete') });
        return;
    }
    pendingTarget = target;
    pendingChannelId = channelId;

    let title: string;
    let itemName = '';
    let subtitle: string;
    let confirmLabel: string;

    if (target.type === 'bulk') {
        const allowed = target.items.filter((item) => item.canDelete !== false);
        const skipped = target.items.length - allowed.length;
        pendingTarget = { ...target, items: allowed };

        const total = allowed.length;
        const folders = allowed.filter(isFolder).length;
        const files = total - folders;
        title = t('messages.delete_trash_title', { count: total });
        const skippedNote = skipped > 0
            ? ` ${t('messages.delete_skipped_note', { count: skipped })}`
            : '';
        if (total === 0) {
            subtitle = t('messages.delete_nothing_deletable', { count: skipped });
        } else if (folders > 0 && files > 0) {
            subtitle = t('messages.delete_mixed_subtitle', { folders, files }) + skippedNote;
        } else if (folders > 0) {
            subtitle = t('messages.delete_folders_subtitle', { folders }) + skippedNote;
        } else {
            subtitle = t('messages.delete_files_subtitle', { files }) + skippedNote;
        }
        confirmLabel = total === 0 ? t('common.close') : t('messages.delete_move_to_trash');
    } else if (target.type === 'folder') {
        title = t('messages.delete_folder_title');
        itemName = target.name.trim();
        subtitle = t('messages.delete_folder_subtitle');
        confirmLabel = t('messages.delete_move_to_trash');
    } else {
        title = t('messages.delete_file_title');
        itemName = target.name.trim();
        subtitle = t('messages.delete_file_subtitle');
        confirmLabel = t('messages.delete_move_to_trash');
    }
    // The phone's confirm names the item inside the sentence, the way the board
    // draws it, instead of on a separate line under the question.
    if (isMobilePlatform() && target.type !== 'bulk') {
        title = t('mobile.trash_confirm_title');
        subtitle = t('mobile.trash_confirm_body', { name: itemName });
        itemName = '';
    }

    openDeleteModalView({ title, itemName, subtitle, confirmLabel });
}

/**
 * Restores the just-trashed items, if the drive is still the one they were
 * deleted from. A trash id is unique only within its channel, so a restore
 * after a drive switch could land on a different object that happens to share
 * the id; rather than risk that, Undo declines once the active drive changes.
 */
async function undoDelete(objectIds: string[], channelId: number): Promise<void> {
    if (activeChannelId() !== channelId) {
        notify({ level: 'info', title: t('messages.switch_drive_restore') });
        return;
    }
    let restored = 0;
    const reasons: string[] = [];
    let switchedDrive = false;
    for (const objectId of objectIds) {
        if (activeChannelId() !== channelId) {
            switchedDrive = true;
            break;
        }
        try {
            const result = await restoreFromTrash(channelId, objectId);
            if (result.ok) restored += 1;
            else reasons.push(humanizeBackendError(result.error));
        } catch (error) {
            reasons.push(humanizeBackendError(error));
        }
    }
    if (restored > 0) {
        invalidateFolderIndex();
        if (activeChannelId() === channelId) appActions().refreshFiles();
    }
    if (switchedDrive) {
        notify({ level: 'info', title: t('messages.switch_drive_restore') });
    }
    if (reasons.length > 0) {
        notify({ level: 'error', title: t('messages.restore_failed_title'), body: reasons[0] });
    }
}

/** Confirms the move to Trash, with a one-tap Undo on the items that made it. */
function notifyMovedToTrash(items: FileCommandItem[], channelId: number): void {
    if (items.length === 0) return;
    const objectIds = items.map(trashObjectId);
    const title = items.length === 1
        ? t('messages.delete_moved_one')
        : t('messages.delete_moved_many', { count: items.length });
    const toastId = notify({
        level: 'success',
        title,
        action: {
            label: t('messages.undo'),
            run: () => {
                dismissNotification(toastId);
                void undoDelete(objectIds, channelId);
            },
        },
    });
}

export async function confirmDelete(): Promise<void> {
    const target = pendingTarget;
    const channelId = pendingChannelId;
    pendingTarget = null;
    pendingChannelId = null;
    closeDeleteModalView();
    if (!target || channelId === null) return;
    // The backend delete API uses the active drive. Never let an open dialog
    // redirect an item's ID after the user switches drives.
    if (activeChannelId() !== channelId) {
        notify({ level: 'info', title: t('messages.switch_drive_delete') });
        return;
    }

    // The rows being deleted go quiet for the duration and then disappear.
    // That is the report; a notice about rows the user is already looking at,
    // parked over the list, was telling them something the list should say.
    // Rows carry their id as a string; a Telegram file's is a number here.
    const releaseBusy = markRowsBusy(
        (target.type === 'bulk' ? target.items : [target]).map((item) => String(item.id)),
    );

    try {
        if (target.type === 'bulk') {
            if (target.items.length === 0) return;
            const folders = target.items.filter(isFolder);
            const files = target.items.filter((item) => item.type === 'file');
            const succeeded: FileCommandItem[] = [];
            const failures: Array<{ item: FileCommandItem; error: string }> = [];
            let interrupted = false;

            for (const folder of folders) {
                if (activeChannelId() !== channelId) { interrupted = true; break; }
                try {
                    const result = await callWithPasswordRetry(() => deleteInChannel(channelId, () => deleteFolder(folder.id)));
                    if (!result.ok) {
                        failures.push({ item: folder, error: humanizeBackendError(result.error) });
                        continue;
                    }
                    if (activeChannelId() === channelId) ensureNotInsideDeletedFolder(folder.id);
                    succeeded.push(folder);
                } catch (error) {
                    console.error('Delete folder failed:', folder, error);
                    failures.push({ item: folder, error: humanizeBackendError(error) });
                }
            }

            for (const file of files) {
                if (interrupted) break;
                if (activeChannelId() !== channelId) { interrupted = true; break; }
                try {
                    const result = await callWithPasswordRetry(() => deleteInChannel(channelId, () => deleteFile(file.id)));
                    if (!result.ok) {
                        failures.push({ item: file, error: humanizeBackendError(result.error) });
                        continue;
                    }
                    succeeded.push(file);
                } catch (error) {
                    console.error('Delete file failed:', file, error);
                    failures.push({ item: file, error: humanizeBackendError(error) });
                }
            }

            if (interrupted || activeChannelId() !== channelId) {
                notify({ level: 'info', title: t('messages.switch_drive_finish') });
            }

            clearSelection();
            if (failures.length > 0) {
                // The reasons are worth keeping, but only the first is worth a
                // toast: past two or three they stop being read and start
                // being dismissed. The rest stay in the bell's history.
                const [first] = failures;
                notify({
                    level: 'error',
                    title: failureBatchTitle(failures.map((entry) => entry.item)),
                    body: failures.length === 1
                        ? first.error
                        : t('messages.delete_rest_hint', { error: first.error }),
                });
            }
            if (succeeded.length > 0) {
                invalidateFolderIndex();
                notifyMovedToTrash(succeeded, channelId);
            }
            appActions().refreshFiles();
            return;
        }

        const result = target.type === 'folder'
            ? await callWithPasswordRetry(() => deleteInChannel(channelId, () => deleteFolder(target.id)))
            : await callWithPasswordRetry(() => deleteInChannel(channelId, () => deleteFile(target.id)));

        if (!result.ok) {
            notify({
                level: 'error',
                title: failureTitle(target),
                body: humanizeBackendError(result.error),
            });
            appActions().refreshFiles();
            return;
        }
        if (target.type === 'folder' && activeChannelId() === channelId) ensureNotInsideDeletedFolder(target.id);
        invalidateFolderIndex();
        notifyMovedToTrash([target], channelId);
        appActions().refreshFiles();
    } catch (error) {
        console.error('Delete failed:', error);
        notify({
            level: 'error',
            title: t('messages.delete_failed_title'),
            body: humanizeBackendError(error),
        });
    } finally {
        // A row left marked is a row the user can no longer touch.
        releaseBusy();
    }
}
