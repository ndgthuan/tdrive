<script lang="ts">
    import ModalShell from './modal_shell.svelte';
    import Button from '../button.svelte';
    import LockIcon from 'phosphor-svelte/lib/LockKeyIcon';
    import { encryptionSetupModal } from './encryption_setup_modal_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onCancel: () => void;
        onSubmit: (password: string, confirmPassword: string, hint: string) => void | Promise<void>;
    }

    let { onCancel, onSubmit }: Props = $props();
    let password = $state('');
    let confirmPassword = $state('');
    let passwordInput = $state<HTMLInputElement | null>(null);
    let confirmPasswordInput = $state<HTMLInputElement | null>(null);
    let hint = $state('');
    let wasOpen = false;

    const view = encryptionSetupModal.state;

    function clearPasswords(): void {
        password = '';
        confirmPassword = '';
        if (passwordInput) passwordInput.value = '';
        if (confirmPasswordInput) confirmPasswordInput.value = '';
    }

    function cancel(): void {
        if ($view.busy) return;
        clearPasswords();
        onCancel();
    }

    function submit(): void {
        if ($view.busy) return;
        const submittedPassword = password;
        const submittedConfirmation = confirmPassword;
        const submittedHint = hint;
        clearPasswords();
        void onSubmit(submittedPassword, submittedConfirmation, submittedHint);
    }

    $effect.pre(() => {
        if (!$view.open) {
            clearPasswords();
            hint = '';
            wasOpen = false;
            return;
        }
        if (!wasOpen) {
            clearPasswords();
            hint = '';
        }
        wasOpen = true;
    });
</script>

<ModalShell
    hostId="encryption-setup-modal"
    open={$view.open}
    title={$t_store('modals.enc_setup_title')}
    titleId="encryption-setup-title"
    initialFocus="#encryption-setup-password"
    restoreFocus="#file-list"
    cardClass="encryption-setup-card"
    onClose={cancel}
>
    <div class="enc-intro"><span class="enc-mark" aria-hidden="true"><LockIcon size={24} /></span><p id="encryption-setup-help" class="modal-subtitle">
        {$t_store('modals.enc_setup_help')}
    </p></div>
    <div class="enc-fields">
    <label for="encryption-setup-password">{$t_store('modals.enc_password_label')}</label>
    <input
        id="encryption-setup-password"
        type="password"
        aria-label={$t_store('modals.enc_password_label')}
        aria-describedby={$view.error ? 'encryption-setup-help encryption-setup-error' : 'encryption-setup-help'}
        placeholder={$t_store('modals.enc_password_placeholder')}
        autocomplete="new-password"
        bind:this={passwordInput}
        bind:value={password}
        disabled={$view.busy}
    />
    <label for="encryption-setup-password-confirm">{$t_store('modals.enc_confirm_aria')}</label>
    <input
        id="encryption-setup-password-confirm"
        type="password"
        aria-label={$t_store('modals.enc_confirm_aria')}
        aria-describedby={$view.error ? 'encryption-setup-help encryption-setup-error' : 'encryption-setup-help'}
        placeholder={$t_store('modals.enc_confirm_placeholder')}
        autocomplete="new-password"
        bind:this={confirmPasswordInput}
        bind:value={confirmPassword}
        disabled={$view.busy}
    />
    <label for="encryption-setup-hint">{$t_store('modals.enc_hint_aria')}</label>
    <input
        id="encryption-setup-hint"
        type="text"
        aria-label={$t_store('modals.enc_hint_aria')}
        aria-describedby={$view.error ? 'encryption-setup-hint-help encryption-setup-error' : 'encryption-setup-hint-help'}
        placeholder={$t_store('modals.enc_hint_placeholder')}
        autocomplete="off"
        bind:value={hint}
        disabled={$view.busy}
    />
    </div>
    <details class="enc-hint-details"><summary>{$t_store('photos.details')}</summary><p id="encryption-setup-hint-help" class="field-help">
        {$t_store('modals.enc_hint_help')}
    </p></details>
    <p class="enc-warning">{$t_store('modals.enc_setup_warning')}</p>
    {#if $view.error}
        <div id="encryption-setup-error" class="modal-error" role="alert">{$view.error}</div>
    {/if}

    {#snippet actions()}
        <Button id="encryption-setup-cancel" variant="secondary" disabled={$view.busy} onclick={cancel}>
            {$t_store('common.cancel')}
        </Button>
        <Button id="encryption-setup-confirm" loading={$view.busy} onclick={submit}>
            {$t_store('modals.enc_set_password')}
        </Button>
    {/snippet}
</ModalShell>

<style>
    .enc-intro { display: flex; align-items: flex-start; gap: var(--space-3); margin: var(--space-3) 0 var(--space-4); }
    .enc-intro p { margin: 0; font-size: var(--font-size-sm); line-height: 1.5; }
    .enc-mark { display: grid; place-items: center; width: 44px; height: 44px; flex-shrink: 0; background: var(--overlay-accent-1); border-radius: var(--radius-md); color: var(--accent); }
    .enc-fields { display: grid; gap: var(--space-2); }
    .enc-fields label { font-size: var(--font-size-xs); color: var(--text-muted); font-weight: var(--weight-strong); margin-top: var(--space-2); }
    .enc-fields input { width: 100%; box-sizing: border-box; min-height: 46px; margin: 0; padding: var(--space-3); line-height: 1.35; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-control); color: var(--text-main); font: inherit; }
    .enc-warning { margin: var(--space-3) 0; padding: var(--space-3); border: 1px solid var(--border); border-radius: var(--radius-md); font-size: var(--font-size-xs); color: var(--text-muted); line-height: 1.5; }
    :global(.encryption-setup-card .field-help) { font-size: var(--font-size-xs); line-height: 1.5; margin-top: var(--space-2); }
    :global(.encryption-setup-card .modal-actions .ui-button) { flex: 1; }
    .enc-hint-details { margin-top: var(--space-2); color: var(--text-muted); font-size: var(--font-size-xs); }
    .enc-hint-details summary { cursor: pointer; }
    :global(html.mobile .encryption-setup-card.modal-sheet) { max-height: calc(100dvh - var(--inset-top) - var(--inset-keyboard) - var(--space-2)); }
    :global(.encryption-setup-card .modal-actions) { flex: 0 0 auto; display: flex; flex-direction: row; align-items: stretch; gap: var(--space-2); }
    :global(.encryption-setup-card .modal-actions .ui-button) { min-height: 44px; }
</style>
