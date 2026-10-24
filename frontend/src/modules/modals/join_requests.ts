import { t } from '../../i18n/i18n';
// Admin modal for approval-required Telegram invite links.

import { approveJoinRequest, listJoinRequests, rejectJoinRequest } from '../channels';
import type { DriveChannel, JoinRequest } from '../../types';
import { notify } from '../notifications';
import { humanizeBackendError } from '../errors';
import {
    joinRequestsList,
    joinRequestsModal,
    type JoinRequestRow,
} from '../../ui/modals/join_requests_modal_store';
let activeDriveId = 0;


export async function openJoinRequestsModal(drive: Pick<DriveChannel, 'id' | 'title'>): Promise<void> {
    const driveId = Number(drive?.id || 0);
    if (!driveId) return;

    activeDriveId = driveId;
    joinRequestsModal.open({ driveId, title: String(drive?.title || t('modals.join_requests_this_drive')) });
    await loadRequests();
}

function toRow(request: JoinRequest): JoinRequestRow {
    return {
        userId: request.userId,
        displayName: request.displayName.trim() || t('messages.join_user_fallback', { id: request.userId || '' }),
        username: request.username,
        requestedAt: request.requestedAt,
    };
}

async function loadRequests(): Promise<void> {
    const driveId = activeDriveId;
    if (!driveId) return;

    joinRequestsList.set({ status: 'loading' });
    try {
        const rows = await listJoinRequests(driveId);
        if (driveId !== activeDriveId) return; // modal moved to another drive
        joinRequestsList.set({
            status: 'ready',
            rows: rows.map(toRow),
            actingUserId: 0,
        });
    } catch (err) {
        if (driveId !== activeDriveId) return;
        joinRequestsList.set({ status: 'error', message: humanizeBackendError(err) });
    }
}

/** Re-runs the load after a failure, for the dialog's Try again control. */
export function reloadJoinRequests(): void {
    void loadRequests();
}

export async function resolveRequest(userId: number, approved: boolean): Promise<void> {
    const driveId = activeDriveId;
    if (!driveId || !userId) return;

    joinRequestsList.update((view) =>
        view.status === 'ready' ? { ...view, actingUserId: userId } : view,
    );
    try {
        if (approved) {
            await approveJoinRequest(driveId, userId);
        } else {
            await rejectJoinRequest(driveId, userId);
        }
        await loadRequests();
    } catch (err) {
        notify({
            level: 'error',
            title: approved ? t('messages.join_approve_failed') : t('messages.join_reject_failed'),
            body: humanizeBackendError(err),
        });
        joinRequestsList.update((view) =>
            view.status === 'ready' ? { ...view, actingUserId: 0 } : view,
        );
    }
}
