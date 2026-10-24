<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { closeLeaveDriveModalView, leaveDriveModalState, type LeaveDriveTarget } from './leave_drive_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onConfirm: (target: LeaveDriveTarget) => void | Promise<void>;
    }

    let { onConfirm }: Props = $props();

    const subtitle = $derived.by(() => {
        const title = $leaveDriveModalState.target?.title || '';
        return title
            ? $t_store('modals.leave_drive_subtitle_named', { title })
            : $t_store('modals.leave_drive_subtitle');
    });

    function canClose(): boolean {
        return !$leaveDriveModalState.inFlight;
    }

    function close(): void {
        if (!canClose()) return;
        closeLeaveDriveModalView();
    }

    function confirm(): void {
        const target = $leaveDriveModalState.target;
        if (!target || $leaveDriveModalState.inFlight) return;
        void onConfirm(target);
    }
</script>

<ModalShell
    hostId="leave-drive-modal"
    open={$leaveDriveModalState.open}
    title={$t_store('modals.leave_drive_title')}
    titleId="leave-drive-title"
    {subtitle}
    initialFocus="#leave-drive-cancel"
    restoreFocus="#drives-nav"
    onClose={close}
>
    {#snippet actions()}
        <button
            id="leave-drive-cancel"
            class="secondary-btn"
            type="button"
            disabled={$leaveDriveModalState.inFlight}
            onclick={close}
        >
            {$t_store('common.cancel')}
        </button>
        <button
            id="leave-drive-confirm"
            class="primary-btn danger-btn"
            type="button"
            disabled={$leaveDriveModalState.inFlight}
            onclick={confirm}
        >
            {$t_store('modals.leave_drive_confirm')}
        </button>
    {/snippet}
</ModalShell>
