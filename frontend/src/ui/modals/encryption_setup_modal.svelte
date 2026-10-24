<script lang="ts">
    import ModalShell from './modal_shell.svelte';
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
    onClose={cancel}
>
    <p id="encryption-setup-help" class="modal-subtitle">
        {$t_store('modals.enc_setup_help')}
        <strong>{$t_store('modals.enc_setup_warning')}</strong>
    </p>
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
    <p id="encryption-setup-hint-help" class="field-help">
        {$t_store('modals.enc_hint_help')}
    </p>
    {#if $view.error}
        <div id="encryption-setup-error" class="modal-error" role="alert">{$view.error}</div>
    {/if}

    {#snippet actions()}
        <button id="encryption-setup-cancel" class="secondary-btn" type="button" disabled={$view.busy} onclick={cancel}>
            {$t_store('common.cancel')}
        </button>
        <button id="encryption-setup-confirm" class="primary-btn" type="button" disabled={$view.busy} onclick={submit}>
            {$t_store('modals.enc_set_password')}
        </button>
    {/snippet}
</ModalShell>
