import { t } from '../../i18n/i18n';
// "Join shared drive" modal.

import { joinSharedDrive } from '../channels';
import { notify, dismissNotification } from '../notifications';
import { humanizeBackendError } from '../errors';
import { joinDriveModal } from '../../ui/modals/join_drive_modal_store';


export function openJoinDriveModal() {
    joinDriveModal.open(null);
}

export async function submitJoinDrive(link: string): Promise<void> {
    const progressId = notify({
        id: 'joining-drive',
        level: 'info',
        title: t('messages.join_joining'),
        body: t('messages.join_joining_body'),
        spinner: true,
    });
    joinDriveModal.setBusy(true);
    try {
        const result = await joinSharedDrive(link);
        joinDriveModal.close();
        dismissNotification(progressId);
        if (result?.status === 'pending') {
            notify({
                level: 'success',
                title: t('messages.join_request_sent'),
                body: t('messages.join_request_sent_body'),
            });
        } else {
            notify({
                level: 'success',
                title: t('messages.joined_drive'),
                body: result?.channel?.title ? String(result.channel.title) : '',
            });
        }
    } catch (err) {
        dismissNotification(progressId);
        notify({
            level: 'error',
            title: t('messages.join_failed'),
            body: humanizeBackendError(err),
        });
    } finally {
        joinDriveModal.setBusy(false);
    }
}
