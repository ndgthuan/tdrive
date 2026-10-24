<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { closeDeleteModalView, deleteModalState } from './delete_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onConfirm: () => void | Promise<void>;
    }

    let { onConfirm }: Props = $props();

    function close(): void {
        closeDeleteModalView();
    }

    function confirm(): void {
        // The module closes the modal itself before starting the async delete,
        // so a slow backend can't leave a confirmable dialog on screen.
        void onConfirm();
    }
</script>

<ModalShell
    hostId="delete-modal"
    open={$deleteModalState.open}
    title={$deleteModalState.title}
    titleId="delete-modal-title"
    subtitle={$deleteModalState.subtitle}
    initialFocus="#delete-cancel"
    restoreFocus="#file-list"
    onClose={close}
>
    {#if $deleteModalState.itemName}
        <div class="delete-target-name" title={$deleteModalState.itemName}>
            {$deleteModalState.itemName}
        </div>
    {/if}

    {#snippet actions()}
        <button id="delete-cancel" class="secondary-btn" type="button" onclick={close}>{$t_store('common.cancel')}</button>
        <button id="delete-confirm" class="primary-btn danger-btn" type="button" onclick={confirm}>
            {$deleteModalState.confirmLabel || $t_store('modals.delete_confirm')}
        </button>
    {/snippet}
</ModalShell>
