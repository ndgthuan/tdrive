import { t } from '../../i18n/i18n';
// "Leave drive" confirm modal.

import { leaveSharedDrive } from '../channels';
import { notify, dismissNotification } from '../notifications';
import { humanizeBackendError } from '../errors';
import {
    closeLeaveDriveModalView,
    openLeaveDriveModalView,
    setLeaveDriveModalInFlight,
    type LeaveDriveTarget,
} from '../../ui/modals/leave_drive_modal_store';
let inFlight = false;


export async function confirmLeaveDrive(target: LeaveDriveTarget): Promise<void> {
    if (inFlight) return;
    inFlight = true;
    setLeaveDriveModalInFlight(true);

    const progressId = notify({
        id: 'leaving-drive',
        level: 'info',
        title: t('messages.leave_leaving'),
        spinner: true,
    });

    try {
        await leaveSharedDrive(target.id);
        closeLeaveDriveModalView();
        dismissNotification(progressId);
        notify({
            level: 'success',
            title: t('messages.left_drive'),
            body: target.title ? String(target.title) : '',
        });
    } catch (err) {
        dismissNotification(progressId);
        notify({
            level: 'error',
            title: t('messages.leave_failed'),
            body: humanizeBackendError(err),
        });
    } finally {
        inFlight = false;
        setLeaveDriveModalInFlight(false);
    }
}

export function openLeaveDriveModal({ id, title }: { id: number | string; title?: string }) {
    openLeaveDriveModalView({ id: Number(id), title: String(title || '') });
}
