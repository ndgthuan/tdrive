<script lang="ts">
    import EyeIcon from 'phosphor-svelte/lib/EyeIcon';
    import EyeOffIcon from 'phosphor-svelte/lib/EyeSlashIcon';
    import ModalShell from './modal_shell.svelte';
    import { encryptionSettingsModal } from './encryption_settings_modal_store';
    import { biometricLabel, biometricUnavailableMessage } from './biometric_labels';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onCancel: () => void;
        onSubmit: (currentPassword: string, newPassword: string, confirmPassword: string, hint: string) => void | Promise<void>;
        onEnableBiometric?: (password: string) => void | Promise<void>;
        onDisableBiometric?: () => void | Promise<void>;
    }

    let { onCancel, onSubmit, onEnableBiometric = () => {}, onDisableBiometric = () => {} }: Props = $props();
    let currentPassword = $state('');
    let newPassword = $state('');
    let confirmPassword = $state('');
    let biometricPassword = $state('');
    let currentPasswordInput = $state<HTMLInputElement | null>(null);
    let newPasswordInput = $state<HTMLInputElement | null>(null);
    let confirmPasswordInput = $state<HTMLInputElement | null>(null);
    let biometricPasswordInput = $state<HTMLInputElement | null>(null);
    let hint = $state('');
    let revealCurrent = $state(false);
    let revealNew = $state(false);
    let wasOpen = false;
    let activePane = $state<'password' | 'device'>('password');

    const view = encryptionSettingsModal.state;
    const biometric = $derived($view.payload?.biometric);
    const biometricBusy = $derived(Boolean($view.payload?.biometricBusy));

    function clearPasswords(): void {
        currentPassword = '';
        newPassword = '';
        confirmPassword = '';
        biometricPassword = '';
        if (currentPasswordInput) currentPasswordInput.value = '';
        if (newPasswordInput) newPasswordInput.value = '';
        if (confirmPasswordInput) confirmPasswordInput.value = '';
        if (biometricPasswordInput) biometricPasswordInput.value = '';
        revealCurrent = false;
        revealNew = false;
    }

    function cancel(): void {
        if ($view.busy && !biometricBusy) return;
        clearPasswords();
        onCancel();
    }

    function submit(): void {
        if ($view.busy) return;
        const submittedCurrentPassword = currentPassword;
        const submittedNewPassword = newPassword;
        const submittedConfirmation = confirmPassword;
        const submittedHint = hint;
        clearPasswords();
        void onSubmit(submittedCurrentPassword, submittedNewPassword, submittedConfirmation, submittedHint);
    }

    function enableBiometric(): void {
        if ($view.busy) return;
        const submittedPassword = biometricPassword;
        clearPasswords();
        void onEnableBiometric(submittedPassword);
    }

    function disableBiometric(): void {
        if ($view.busy) return;
        clearPasswords();
        void onDisableBiometric();
    }

    function switchPane(pane: 'password' | 'device'): void {
        clearPasswords();
        encryptionSettingsModal.setError('');
        activePane = pane;
    }

    // Hold-to-show: the input stays masked except while the eye button is
    // actively pressed (pointer or Space/Enter held).
    function holdHandlers(set: (visible: boolean) => void) {
        return {
            onpointerdown: (event: PointerEvent) => {
                event.preventDefault();
                set(true);
            },
            onpointerup: () => set(false),
            onpointerleave: () => set(false),
            onpointercancel: () => set(false),
            onblur: () => set(false),
            onkeydown: (event: KeyboardEvent) => {
                if (event.key === ' ' || event.key === 'Enter') {
                    event.preventDefault();
                    set(true);
                }
            },
            onkeyup: () => set(false),
        };
    }

    const currentHold = holdHandlers((visible) => (revealCurrent = visible));
    const newHold = holdHandlers((visible) => (revealNew = visible));

    $effect.pre(() => {
        if (!$view.open) {
            clearPasswords();
            hint = '';
            wasOpen = false;
            return;
        }
        if (!wasOpen) {
            clearPasswords();
            hint = $view.payload?.hint ?? '';
            activePane = 'password';
        }
        wasOpen = true;
    });
</script>

<ModalShell
    hostId="encryption-settings-modal"
    open={$view.open}
    title={$t_store('modals.enc_settings_title')}
    titleId="encryption-settings-title"
    cardClass="encryption-settings-card"
    subtitle={$t_store('modals.enc_settings_subtitle')}
    initialFocus="#encryption-current-password"
    restoreFocus="#profile-trigger"
    onClose={cancel}
>
    <nav class="encryption-settings-panes" aria-label={$t_store('modals.enc_panes_aria')}>
        <button type="button" class="secondary-btn" aria-pressed={activePane === 'password'} disabled={$view.busy} onclick={() => switchPane('password')}>{$t_store('modals.enc_tab_password')}</button>
        <button id="encryption-device-unlock-pane" type="button" class="secondary-btn" aria-pressed={activePane === 'device'} disabled={$view.busy} onclick={() => switchPane('device')}>{$t_store('modals.enc_tab_device')}</button>
    </nav>
    {#if activePane === 'password'}
    <div class="input-with-action">
        <input
            id="encryption-current-password"
            type={revealCurrent ? 'text' : 'password'}
            aria-label={$t_store('modals.enc_current_aria')}
            aria-describedby={$view.error ? 'encryption-settings-error' : undefined}
            placeholder={$t_store('modals.enc_current_placeholder')}
            autocomplete="current-password"
            bind:this={currentPasswordInput}
            bind:value={currentPassword}
            disabled={$view.busy}
        />
        <button
            class="input-action-btn reveal-on-hold"
            type="button"
            data-state={revealCurrent ? 'visible' : undefined}
            disabled={$view.busy}
            aria-label={$t_store('modals.enc_reveal_current_aria')}
            title={$t_store('modals.enc_reveal_title')}
            {...currentHold}
        >
            <EyeIcon class="input-action-icon icon-eye" size={18} aria-hidden="true" />
            <EyeOffIcon class="input-action-icon icon-eye-off" size={18} aria-hidden="true" />
        </button>
    </div>
    <div class="input-with-action">
        <input
            id="encryption-new-password"
            type={revealNew ? 'text' : 'password'}
            aria-label={$t_store('modals.enc_new_aria')}
            aria-describedby={$view.error ? 'encryption-settings-error' : undefined}
            placeholder={$t_store('modals.enc_new_placeholder')}
            autocomplete="new-password"
            bind:this={newPasswordInput}
            bind:value={newPassword}
            disabled={$view.busy}
        />
        <button
            class="input-action-btn reveal-on-hold"
            type="button"
            data-state={revealNew ? 'visible' : undefined}
            disabled={$view.busy}
            aria-label={$t_store('modals.enc_reveal_new_aria')}
            title={$t_store('modals.enc_reveal_title')}
            {...newHold}
        >
            <EyeIcon class="input-action-icon icon-eye" size={18} aria-hidden="true" />
            <EyeOffIcon class="input-action-icon icon-eye-off" size={18} aria-hidden="true" />
        </button>
    </div>
    <input
        id="encryption-new-password-confirm"
        type="password"
        aria-label={$t_store('modals.enc_confirm_new_aria')}
        aria-describedby={$view.error ? 'encryption-settings-error' : undefined}
        placeholder={$t_store('modals.enc_confirm_new_placeholder')}
        autocomplete="new-password"
        bind:this={confirmPasswordInput}
        bind:value={confirmPassword}
        disabled={$view.busy}
    />
    <input
        id="encryption-settings-hint"
        type="text"
        aria-label={$t_store('modals.enc_hint_aria')}
        aria-describedby={$view.error ? 'encryption-settings-hint-help encryption-settings-error' : 'encryption-settings-hint-help'}
        placeholder={$t_store('modals.enc_hint_placeholder')}
        autocomplete="off"
        bind:value={hint}
        disabled={$view.busy}
    />
    <p id="encryption-settings-hint-help" class="field-help">
        {$t_store('modals.enc_hint_help')}
    </p>
    {:else if biometric}
        <section class="biometric-settings" aria-labelledby="encryption-biometric-title">
            <h4 id="encryption-biometric-title">{$t_store('modals.enc_device_heading')}</h4>
            <p id="encryption-biometric-help" class="field-help biometric-help">
                {#if biometric.enrolled}
                    {$t_store('modals.enc_bio_enrolled', { method: biometricLabel(biometric.kind, $t_store) })}
                {:else if !biometric.available}
                    {$t_store('modals.enc_bio_plain')}
                {:else}
                    {$t_store('modals.enc_bio_enable_help', { method: biometricLabel(biometric.kind, $t_store) })}
                {/if}
            </p>
            {#if !biometric.available}
                <p class="field-help biometric-help">{biometricUnavailableMessage(biometric.reason, $t_store)}</p>
            {/if}
            {#if biometric.enrolled}
                <button id="encryption-biometric-disable" class="secondary-btn biometric-setting-action" type="button" disabled={$view.busy} onclick={disableBiometric}>
                    {$t_store('modals.bio_disable', { method: biometricLabel(biometric.kind, $t_store) })}
                </button>
            {:else if biometric.available}
                <div class="modal-field">
                    <label class="field-label" for="encryption-biometric-password">{$t_store('modals.enc_password_label')}</label>
                    <input
                        id="encryption-biometric-password"
                        type="password"
                        aria-describedby={$view.error ? 'encryption-biometric-help encryption-settings-error' : 'encryption-biometric-help'}
                        placeholder={$t_store('modals.enc_bio_password_placeholder')}
                        autocomplete="current-password"
                        bind:this={biometricPasswordInput}
                        bind:value={biometricPassword}
                        disabled={$view.busy}
                        onkeydown={(event) => { if (event.key === 'Enter') { event.preventDefault(); enableBiometric(); } }}
                    />
                </div>
                <button id="encryption-biometric-enable" class="secondary-btn biometric-setting-action" type="button" disabled={$view.busy} onclick={enableBiometric}>
                    {biometricBusy ? $t_store('modals.bio_waiting') : $t_store('modals.bio_enable', { method: biometricLabel(biometric.kind, $t_store) })}
                </button>
            {/if}
        </section>
    {:else}
        <p class="field-help biometric-help" role="status">{$t_store('modals.bio_checking')}</p>
    {/if}
    {#if $view.error}
        <div id="encryption-settings-error" class="modal-error" role="alert">{$view.error}</div>
    {/if}

    {#snippet actions()}
        <button id="encryption-settings-cancel" class="secondary-btn" type="button" disabled={$view.busy && !biometricBusy} onclick={cancel}>
            {$t_store('common.cancel')}
        </button>
        {#if activePane === 'password'}
            <button id="encryption-settings-confirm" class="primary-btn" type="button" disabled={$view.busy} onclick={submit}>
                {$t_store('common.save')}
            </button>
        {/if}
    {/snippet}
</ModalShell>

<style>
    .biometric-settings { border-top: 1px solid var(--border); padding-top: var(--space-4); margin-bottom: var(--space-4); }
    .encryption-settings-panes { display: flex; gap: var(--space-2); margin-bottom: var(--space-4); }
    .encryption-settings-panes button { flex: 1; min-width: 0; }
    .encryption-settings-panes button[aria-pressed='true'] { border-color: var(--accent); color: var(--text-main); background: var(--overlay-white-1); }
    .biometric-settings h4 { font-size: var(--type-base); color: var(--color-text); font-weight: var(--weight-semibold); margin-bottom: var(--space-2); }
    .biometric-help { margin-top: 0; }
    .biometric-setting-action { width: 100%; }
    :global(.encryption-settings-card:not(.modal-sheet)) { max-height: calc(100dvh - 48px); overflow-y: auto; overscroll-behavior: contain; }
</style>
