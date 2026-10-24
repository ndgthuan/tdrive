<script lang="ts">
    import { listChannelSourceCandidates, resolvePublicChannelSource } from '../../api';
    import { activateLiveSyncEvents } from '../../modules/channels';
    import { activateChannelSources, addChannel, channelPhoto, closeChannelPicker, showChannel } from '../../modules/channel_sources';
    import { activateContextMenu } from '../../modules/context_menu';
    import { activateDropOverlay } from '../../modules/drop_overlay';
    import { renderEncryptionSettingsEntry } from '../../modules/encryption';
    import { activateFileList } from '../../modules/file_list';
    import { activateGallery } from '../../modules/gallery';
    import { renderBreadcrumb } from '../../modules/navigation';
    import {
        activateNotificationEffects,
        dismissNotification,
        pauseAllNotifications,
        pauseToast,
        resumeAllNotifications,
        resumeToast,
    } from '../../modules/notifications';
    import { ensureProfileLoaded } from '../../modules/profile_menu';
    import { activateConnectivityWatch } from '../../modules/connectivity';
    import { activateRefreshShortcut } from '../../modules/refresh_shortcut';
    import { activateSearchBar } from '../../modules/search';
    import { activateSelectionBar } from '../../modules/selection';
    import { activateSidebar } from '../../modules/sidebar';
    import { activateTransferSurfaces, closeFailedUploads, retryFailedUploads } from '../../modules/transfers';
    import { activateResumableUploads } from '../../modules/resumable_uploads';
    import { activateResumableDownloads } from '../../modules/resumable_downloads';
    import { activateUpdates } from '../../modules/updates';
    import { confirmDelete } from '../../modules/modals/delete';
    import { confirmTrashAction } from '../../modules/trash/controller';
    import {
        cancelEncryptionPassword,
        submitEncryptionBiometric,
        submitEncryptionPassword,
    } from '../../modules/modals/encryption_password';
    import {
        cancelEncryptionSettings,
        removeBiometricEnrollment,
        submitBiometricEnrollment,
        submitEncryptionSettings,
    } from '../../modules/modals/encryption_settings';
    import {
        cancelEncryptionSetup,
        submitEncryptionSetup,
    } from '../../modules/modals/encryption_setup';
    import { submitFolder } from '../../modules/modals/folder';
    import {
        cancelImportOptions,
        confirmImportOptions,
        replanImportOptions,
    } from '../../modules/modals/import_options';
    import { submitJoinDrive } from '../../modules/modals/join_drive';
    import { reloadJoinRequests, resolveRequest } from '../../modules/modals/join_requests';
    import { confirmLeaveDrive } from '../../modules/modals/leave_drive';
    import { confirmLogout } from '../../modules/modals/logout';
    import {
        confirmMove,
        navigateMoveBack,
        navigateMoveCrumb,
        openMoveFolder,
    } from '../../modules/modals/move';
    import { submitNewDrive } from '../../modules/modals/new_drive';
    import { submitRename } from '../../modules/modals/rename';
    import {
        cancelUploadOptions,
        confirmUploadOptions,
    } from '../../modules/modals/upload_options';
    import FileList from '../file_list/file_list.svelte';
    import ContextMenu from '../menus/context_menu.svelte';
    import DeleteModal from '../modals/delete_modal.svelte';
    import EncryptionPasswordModal from '../modals/encryption_password_modal.svelte';
    import EncryptionSettingsModal from '../modals/encryption_settings_modal.svelte';
    import EncryptionSetupModal from '../modals/encryption_setup_modal.svelte';
    import FolderModal from '../modals/folder_modal.svelte';
    import ImportOptionsModal from '../modals/import_options_modal.svelte';
    import JoinDriveModal from '../modals/join_drive_modal.svelte';
    import JoinRequestsModal from '../modals/join_requests_modal.svelte';
    import LeaveDriveModal from '../modals/leave_drive_modal.svelte';
    import LogoutModal from '../modals/logout_modal.svelte';
    import MoveModal from '../modals/move_modal.svelte';
    import NewDriveModal from '../modals/new_drive_modal.svelte';
    import RenameModal from '../modals/rename_modal.svelte';
    import ShareDriveModal from '../modals/share_drive_modal.svelte';
    import MembersDialog from '../members/members_dialog.svelte';
    import { activate_member_roles } from '../members/members_store';
    import FailedUploadsModal from '../modals/failed_uploads_modal.svelte';
    import UploadOptionsModal from '../modals/upload_options_modal.svelte';
    import TrashConfirmModal from '../trash/trash_confirm_modal.svelte';
    import MountSelectionModal from '../mount/mount_selection_modal.svelte';
    import ChannelPickerModal from '../channels/channel_picker_modal.svelte';
    import { channelPickerOpen, channelSources } from '../channels/channel_store';
    import ToastStack from '../notifications/toast_stack.svelte';
    import PreviewModal from '../preview/preview_modal.svelte';
    import DropOverlay from '../transfers/drop_overlay.svelte';
    import VideoModal from '../video/video_modal.svelte';
    import FileViewerModal from '../viewers/file_viewer_modal.svelte';
    import { appView } from './app_store';
    import FeaturePortal from './feature_portal.svelte';

    $effect(() => activateNotificationEffects());

    $effect(() => {
        if ($appView.kind !== 'dashboard') return;

        const disposers = [
            activateContextMenu(),
            activateSidebar(),
            activateSelectionBar(),
            activateFileList(),
            activateGallery(),
            activateDropOverlay(),
            activateTransferSurfaces(),
            activateResumableUploads(),
            activateResumableDownloads(),
            activateSearchBar(),
            activateRefreshShortcut(),
            activateConnectivityWatch(),
            activateLiveSyncEvents(),
            activateUpdates(),
            activateChannelSources(),
            activate_member_roles(),
        ];
        let previewActivationCancelled = false;
        let deactivatePreview = () => {};

        void import('../../modules/modals/preview').then(({ activatePreviewModal }) => {
            if (previewActivationCancelled) return;
            deactivatePreview = activatePreviewModal();
        });

        renderBreadcrumb();
        renderEncryptionSettingsEntry();
        void ensureProfileLoaded();

        return () => {
            cancelEncryptionPassword();
            cancelEncryptionSettings();
            previewActivationCancelled = true;
            deactivatePreview();
            for (let index = disposers.length - 1; index >= 0; index -= 1) {
                disposers[index]();
            }
        };
    });
</script>

<div id="toast-stack" class="toast-stack" role="status" aria-live="polite">
    <ToastStack
        onDismiss={dismissNotification}
        onPauseToast={pauseToast}
        onResumeToast={resumeToast}
        onPauseAll={pauseAllNotifications}
        onResumeAll={resumeAllNotifications}
    />
</div>

{#if $appView.kind === 'dashboard'}
    <FeaturePortal hostId="file-list">
        <FileList />
    </FeaturePortal>

    <div id="context-menu" class="context-menu">
        <ContextMenu />
    </div>
    <DropOverlay />
    <div id="failed-uploads-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <FailedUploadsModal onClose={closeFailedUploads} onRetry={() => { void retryFailedUploads(); }} />
    </div>
    <div id="mount-selection-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <MountSelectionModal />
    </div>

    <div id="delete-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <DeleteModal onConfirm={confirmDelete} />
    </div>
    <div id="rename-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <RenameModal onSubmit={submitRename} />
    </div>
    <div id="folder-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <FolderModal onSubmit={submitFolder} />
    </div>
    <div id="move-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <MoveModal
            onOpenFolder={openMoveFolder}
            onCrumb={navigateMoveCrumb}
            onBack={navigateMoveBack}
            onConfirm={confirmMove}
        />
    </div>
    <div id="new-drive-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <NewDriveModal onSubmit={submitNewDrive} />
    </div>
    <div id="join-drive-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <JoinDriveModal onSubmit={submitJoinDrive} />
    </div>
    <div id="share-drive-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <ShareDriveModal />
    </div>
    <div id="members-dialog" class="modal-overlay" style="display: none;" aria-hidden="true">
        <MembersDialog />
    </div>
    <div id="leave-drive-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <LeaveDriveModal onConfirm={confirmLeaveDrive} />
    </div>
    <div id="join-requests-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <JoinRequestsModal onAction={resolveRequest} onRetry={reloadJoinRequests} />
    </div>
    <div id="encryption-setup-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <EncryptionSetupModal onCancel={cancelEncryptionSetup} onSubmit={submitEncryptionSetup} />
    </div>
    <div id="encryption-password-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <EncryptionPasswordModal onCancel={cancelEncryptionPassword} onSubmit={submitEncryptionPassword} onBiometric={submitEncryptionBiometric} />
    </div>
    <div id="encryption-settings-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <EncryptionSettingsModal onCancel={cancelEncryptionSettings} onSubmit={submitEncryptionSettings} onEnableBiometric={submitBiometricEnrollment} onDisableBiometric={removeBiometricEnrollment} />
    </div>
    <div id="upload-options-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <UploadOptionsModal onCancel={cancelUploadOptions} onConfirm={confirmUploadOptions} />
    </div>
    <div id="import-options-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <ImportOptionsModal
            onCancel={cancelImportOptions}
            onConfirm={confirmImportOptions}
            onToggle={replanImportOptions}
        />
    </div>
    <div id="logout-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <LogoutModal onConfirm={confirmLogout} />
    </div>
    <div id="channel-picker-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <ChannelPickerModal
            open={$channelPickerOpen}
            loadCandidates={listChannelSourceCandidates}
            resolvePublic={resolvePublicChannelSource}
            added={$channelSources.sources}
            loadPhoto={channelPhoto}
            onAdd={addChannel}
            onOpen={showChannel}
            onClose={closeChannelPicker}
        />
    </div>

    <!-- The trash renders into the file list, not a dialog; only the confirm it
         raises is a modal here. -->
    <div id="trash-confirm-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <TrashConfirmModal onConfirm={confirmTrashAction} />
    </div>

    <!-- Viewer controllers stay lazy because they load media/runtime dependencies only when used. -->
    <div id="viewer-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
        <FileViewerModal
            onClose={() => {
                void import('../../modules/modals/file_viewer').then(({ closeFileViewer }) => closeFileViewer());
            }}
            onDownload={() => {
                void import('../../modules/modals/file_viewer').then(({ downloadActiveFile }) => downloadActiveFile());
            }}
        />
    </div>
    <div id="preview-modal" class="modal-overlay preview-overlay" style="display: none;" aria-hidden="true">
        <PreviewModal />
    </div>
    <div id="video-modal" class="modal-overlay video-overlay" style="display: none;" aria-hidden="true">
        <VideoModal
            onPreferencesChange={(value) => {
                void import('../../modules/modals/video').then(({ updatePlaybackPreferences }) =>
                    updatePlaybackPreferences(value),
                );
            }}
            onHidePlaylist={(restoreFocus) => {
                void import('../../modules/modals/video').then(({ hideVideoPlaylist }) =>
                    hideVideoPlaylist(restoreFocus),
                );
            }}
            onSelectPlaylistItem={(index) => {
                void import('../../modules/modals/video').then(({ selectVideoPlaylistItem }) =>
                    selectVideoPlaylistItem(index),
                );
            }}
            onUpdateVideoAutoNext={(enabled) => {
                void import('../../modules/modals/video').then(({ updateVideoAutoNext }) =>
                    updateVideoAutoNext(enabled),
                );
            }}
        />
    </div>
{/if}
