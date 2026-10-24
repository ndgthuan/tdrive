import { t } from '../../i18n/i18n';
import { allow_edit_in } from '../../ui/members/drive_guard';
// Rename modal for TDrive frontend

import { addTelegramFileToDrive, renameFile, renameFolder } from '../../api';
import { invalidateFolderIndex } from '../../state';
import { callWithPasswordRetry } from './encryption_password';
import { humanizeBackendError } from '../errors';
import { appActions } from '../app_actions';
import {
    closeRenameModalView,
    openRenameModalView,
    setRenameModalError,
    setRenameModalInFlight,
    type RenameModalTarget,
} from '../../ui/modals/rename_modal_store';
import type { FileCommandItem } from '../../ui/file_list/types';

async function ensureFileInTdriveSystem(target: RenameModalTarget): Promise<void> {
    if (target.type !== 'file' || target.source !== 'tg') return;

    const result = await addTelegramFileToDrive(
        target.id,
        target.name,
        target.size,
        target.parentId,
    );

    if (!result.ok) throw new Error(humanizeBackendError(result.error));
}


export function openRenameModal(target: FileCommandItem): void {
    if (!allow_edit_in(target.channelId)) return;
    openRenameModalView(target);
}

export async function submitRename(target: RenameModalTarget, rawName: string): Promise<void> {
    const nextName = (rawName || '').trim();
    if (!nextName) {
        setRenameModalError(t('messages.rename_empty'));
        return;
    }
    if (/[\\/]/.test(nextName)) {
        setRenameModalError(t('messages.rename_invalid_chars'));
        return;
    }

    setRenameModalError('');
    setRenameModalInFlight(true);
    try {
        let result;
        if (target.type === 'folder') {
            result = await callWithPasswordRetry(() => renameFolder(target.id, nextName));
        } else {
            await ensureFileInTdriveSystem(target);
            result = await callWithPasswordRetry(() => renameFile(target.id, nextName));
        }

        if (!result.ok) {
            setRenameModalError(humanizeBackendError(result.error));
            return;
        }
        invalidateFolderIndex();
        closeRenameModalView();
        appActions().refreshFiles();
    } catch (err) {
        setRenameModalError(humanizeBackendError(err));
    } finally {
        setRenameModalInFlight(false);
    }
}
