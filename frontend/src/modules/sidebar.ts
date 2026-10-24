import { t } from '../i18n/i18n';
// Sidebar — renders the list of drives (personal + shared) and routes
// clicks to channels.js. Re-renders on every loadChannels() call.

import { state } from '../state';
import {
    switchActiveChannel,
    getInviteLink,
    getApprovalInviteLink,
    checkPendingJoin,
    removePendingJoin,
} from './channels';
import { openShareDriveModal } from './modals/share_drive';
import { openLeaveDriveModal } from './modals/leave_drive';
import { openNewDriveModal } from './modals/new_drive';
import { openJoinDriveModal } from './modals/join_drive';
import { openJoinRequestsModal } from './modals/join_requests';
import { notify } from './notifications';
import { humanizeBackendError } from './errors';
import { enterPhotos, exitPhotos } from './gallery';
import { closeChannel } from '../ui/channels/channel_store';
import { closeTrash } from './trash/controller';
import { showContextMenu, type ContextMenuItem } from './context_menu';
import {
    setSidebarState,
    type SidebarActionMenuRequest,
} from '../ui/sidebar/sidebar_store';
import type { DriveChannel, PendingJoin } from '../types';


export function activateSidebar(): () => void {
    const newButton = document.getElementById('open-new-drive');
    const joinButton = document.getElementById('open-join-drive');
    const photosButton = document.getElementById('nav-photos');
    const onNewDrive = () => openNewDriveModal();
    const onJoinDrive = () => openJoinDriveModal();
    const onPhotos = () => enterPhotos();

    newButton?.addEventListener('click', onNewDrive);
    joinButton?.addEventListener('click', onJoinDrive);
    photosButton?.addEventListener('click', onPhotos);
    renderSidebar();

    return () => {
        newButton?.removeEventListener('click', onNewDrive);
        joinButton?.removeEventListener('click', onJoinDrive);
        photosButton?.removeEventListener('click', onPhotos);
    };
}

export function renderSidebar() {

    const channels = state.channels || [];
    const personal = channels.filter((channel) => channel.kind === 'personal');
    const shared = channels.filter((channel) => channel.kind === 'shared');
    const pending = state.pendingJoins.filter((request) => request.inviteHash);

    const virtualView = state.virtualView;
    const photosActive = virtualView === 'photos';
    setSidebarState({
        personal,
        shared,
        pending,
        activeChannelId: state.activeChannel ? Number(state.activeChannel.id) : null,
        virtualView,
    });

    // The Photos item owns the current-route semantics while the gallery is open.
    const photosNav = document.getElementById('nav-photos');
    photosNav?.classList.toggle('active', photosActive);
    if (photosActive) photosNav?.setAttribute('aria-current', 'page');
    else photosNav?.removeAttribute('aria-current');
}


export function handleDriveClick(channelId: number): void {
    closeChannel();
    if (Number(channelId) === Number(state.activeChannel?.id)) {
        // Clicking the already-active drive is how you leave one of its virtual
        // views. Without this the trash is a dead end: it has no breadcrumb to
        // climb out by, and the drive you would click is the one you are in.
        if (state.virtualView === 'photos') exitPhotos();
        else if (state.virtualView === 'trash') closeTrash();
        return;
    }
    void switchActiveChannel(Number(channelId));
}

export async function handlePendingClick(inviteHash: string): Promise<void> {
    const pending = state.pendingJoins.find((item) => item.inviteHash === inviteHash);
    if (!pending) return;

    try {
        const result = await checkPendingJoin(inviteHash);
        if (result?.status === 'joined') {
            notify({
                level: 'success',
                title: t('messages.request_approved'),
                body: t('messages.joined_drive_body'),
            });
        } else {
            const lastError = result?.pending?.lastError.trim() ?? '';
            notify({
                level: lastError ? 'error' : 'info',
                title: lastError ? t('leftovers.could_not_check_request') : t('leftovers.still_waiting_approval'),
                body: lastError,
            });
        }
    } catch (err) {
        notify({
            level: 'error',
            title: t('messages.check_request_failed'),
            body: humanizeBackendError(err),
        });
    }
}

export function showSharedActionsMenu(request: SidebarActionMenuRequest, c: DriveChannel): void {
    const items: ContextMenuItem[] = [
        {
            label: t('leftovers.copy_invite_link'),
            action: async () => {
                try {
                    const link = await getInviteLink(Number(c.id));
                    openShareDriveModal(link, { approvalRequired: false });
                } catch (err) {
                    notify({
                        level: 'error',
                        title: t('messages.invite_link_failed'),
                        body: humanizeBackendError(err),
                    });
                }
            },
        },
        {
            label: t('leftovers.copy_approval_link'),
            action: async () => {
                try {
                    const link = await getApprovalInviteLink(Number(c.id));
                    openShareDriveModal(link, { approvalRequired: true });
                } catch (err) {
                    notify({
                        level: 'error',
                        title: t('messages.approval_link_failed'),
                        body: humanizeBackendError(err),
                    });
                }
            },
        },
        {
            label: t('leftovers.join_requests'),
            action: () => openJoinRequestsModal({ id: Number(c.id), title: c.title }),
        },
        {
            label: t('leftovers.leave_drive'),
            danger: true,
            action: () => openLeaveDriveModal({ id: Number(c.id), title: c.title }),
        },
    ];
    showContextMenu(request.x, request.y, items);
}

export function showPendingActionsMenu(request: SidebarActionMenuRequest, p: PendingJoin): void {
    showContextMenu(request.x, request.y, [
        {
            label: t('leftovers.check_now'),
            action: async () => {
                try {
                    const result = await checkPendingJoin(p.inviteHash);
                    const lastError = result?.pending?.lastError.trim() ?? '';
                    notify({
                        level: result?.status === 'joined' ? 'success' : (lastError ? 'error' : 'info'),
                        title: result?.status === 'joined'
                            ? t('messages.request_approved')
                            : (lastError ? t('messages.check_request_failed') : t('messages.still_waiting')),
                        body: result?.status === 'joined' ? t('messages.joined_drive_body') : lastError,
                    });
                } catch (err) {
                    notify({
                        level: 'error',
                        title: t('messages.check_request_failed'),
                        body: humanizeBackendError(err),
                    });
                }
            },
        },
        {
            label: t('leftovers.remove_request'),
            danger: true,
            action: async () => {
                try {
                    await removePendingJoin(p.inviteHash);
                    notify({
                        level: 'success',
                        title: t('messages.request_removed'),
                    });
                } catch (err) {
                    notify({
                        level: 'error',
                        title: t('messages.remove_request_failed'),
                        body: humanizeBackendError(err),
                    });
                }
            },
        },
    ]);
}
