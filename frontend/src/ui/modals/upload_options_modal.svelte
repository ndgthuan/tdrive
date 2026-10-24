<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import { uploadOptionsModal } from './upload_options_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onCancel: () => void;
        onConfirm: (choice: { encrypt: boolean }) => void;
    }

    let { onCancel, onConfirm }: Props = $props();
    let uploadMode = $state<'plain' | 'encrypt'>('plain');
    let wasOpen = false;

    const view = uploadOptionsModal.state;
    const summary = $derived($t_store('modals.upload_summary', { count: $view.payload?.count ?? 0 }));

    function confirm(): void {
        onConfirm({ encrypt: uploadMode === 'encrypt' });
    }

    $effect(() => {
        if ($view.open && !wasOpen) {
            // Default to plain upload every time. Encryption is explicit per batch.
            uploadMode = 'plain';
        }
        wasOpen = $view.open;
    });
</script>

<ModalShell
    hostId="upload-options-modal"
    open={$view.open}
    title={$t_store('modals.upload_title')}
    titleId="upload-options-title"
    subtitle={summary}
    initialFocus='input[name="upload-mode"]:checked'
    restoreFocus="#file-list"
    onClose={onCancel}
>
    <div class="modal-choice-group" role="radiogroup" aria-labelledby="upload-options-title">
    <label class="modal-choice-row">
        <input type="radio" name="upload-mode" value="plain" bind:group={uploadMode} />
        <span>
            <strong>{$t_store('modals.upload_plain_title')}</strong>
            <small>{$t_store('modals.upload_plain_desc')}</small>
        </span>
    </label>
    <label class="modal-choice-row">
        <input type="radio" name="upload-mode" value="encrypt" bind:group={uploadMode} />
        <span>
            <strong>{$t_store('modals.upload_encrypt_title')}</strong>
            <small>{$t_store('modals.upload_encrypt_desc')}</small>
        </span>
    </label>
    </div>

    {#snippet actions()}
        <button id="upload-options-cancel" class="secondary-btn" type="button" onclick={onCancel}>{$t_store('common.cancel')}</button>
        <button id="upload-options-confirm" class="primary-btn" type="button" onclick={confirm}>{$t_store('modals.upload_continue')}</button>
    {/snippet}
</ModalShell>
