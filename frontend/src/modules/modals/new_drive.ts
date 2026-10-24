import { t } from '../../i18n/i18n';
// "New shared drive" modal.

import { createSharedDrive } from '../channels';
import { openShareDriveModal } from './share_drive';
import { notify, dismissNotification } from '../notifications';
import { humanizeBackendError } from '../errors';
import { newDriveModal } from '../../ui/modals/new_drive_modal_store';


export function openNewDriveModal() {
    newDriveModal.open(null);
}

export async function submitNewDrive(title: string, requireApproval: boolean): Promise<void> {
    const progressId = notify({
        id: 'creating-drive',
        level: 'info',
        title: t('messages.new_creating'),
        spinner: true,
    });
    newDriveModal.setBusy(true);
    try {
        const info = await createSharedDrive(title, requireApproval);
        newDriveModal.close();
        dismissNotification(progressId);
        notify({ level: 'success', title: t('messages.new_created', { title }) });
        if (info.inviteLink) {
            openShareDriveModal(info.inviteLink, { approvalRequired: requireApproval });
        }
    } catch (err) {
        dismissNotification(progressId);
        notify({
            level: 'error',
            title: t('messages.new_failed'),
            body: humanizeBackendError(err),
        });
    } finally {
        newDriveModal.setBusy(false);
    }
}
