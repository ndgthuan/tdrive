<script lang="ts">
    import CountryCodePicker from './country_code_picker.svelte';
    import { normalizePhoneNumber, countryForInternationalNumber, nationalNumberForCountryChange } from './phone_number';
    import type { CountryCode } from 'libphonenumber-js/min';
    import tdriveLogo from '../../assets/images/tdrive_logo.png';
    import EyeIcon from 'phosphor-svelte/lib/EyeIcon';
    import EyeOffIcon from 'phosphor-svelte/lib/EyeSlashIcon';
    import LockSimpleIcon from 'phosphor-svelte/lib/LockSimpleIcon';
    import PaperPlaneIcon from 'phosphor-svelte/lib/PaperPlaneTiltIcon';
    import { tick } from 'svelte';
    import { isMobilePlatform, openExternalUrl } from '../../api';
    import { authScreen } from '../app/app_store';
    import {
        failAuthSubmission,
        authHint,
        authPhone,
        authSubmission,
        type AuthFlow,
    } from './auth_store';
    import { personalDriveSetup } from './personal_drive_store';
    import PersonalDriveSetup from './personal_drive_setup.svelte';
    import { installUpdate, openReleasePage } from '../../modules/updates';
    import { isVersionSkipped } from '../updates/update_model';
    import { updatePrefs, updateState } from '../updates/update_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        onSetup: (apiId: string, apiHash: string) => void | Promise<void>;
        onPhone: (phone: string) => void | Promise<void>;
        onCode: (code: string) => void | Promise<void>;
        onPassword: (password: string) => void | Promise<void>;
        onBackToPhone: () => void;
        onDriveSelect?: (channelID: string) => void;
        onDriveCreate?: () => void;
        onDriveRetry?: () => void | Promise<void>;
    }

    let {
        onSetup,
        onPhone,
        onCode,
        onPassword,
        onBackToPhone,
        onDriveSelect = () => undefined,
        onDriveCreate = () => undefined,
        onDriveRetry = () => undefined,
    }: Props = $props();

    const TELEGRAM_APPS_URL = 'https://my.telegram.org/apps';

    // Phones get full-bleed pages and a welcome page in front of the
    // credentials form. The welcome is local presentation only: the auth
    // state machine stays on 'setup' underneath it.
    const mobile = isMobilePlatform();
    let welcomed = $state(false);
    const step = $derived(mobile && $authScreen === 'setup' && !welcomed ? 'welcome' : $authScreen);

    // Update discoverability for users who are stuck before login (e.g. a
    // Telegram API change breaks sign-in). The updater runs independently of
    // auth, so a ready build can be installed straight from here.
    const updateFooter = $derived.by(() => {
        const s = $updateState;
        const latest = s.latest;
        if (!latest || isVersionSkipped(latest.version, $updatePrefs.skippedVersion)) return null;
        if (s.phase === 'ready') {
            return { version: latest.version, label_key: 'auth.update_restart', action: installUpdate };
        }
        if (s.phase === 'available' && s.installable) {
            return { version: latest.version, label_key: 'auth.update_get', action: openReleasePage };
        }
        return null;
    });

    let apiId = $state('');
    let apiHash = $state('');
    let phone = $state('');
    let country = $state<CountryCode | undefined>();

    function updatePhone(event: Event): void {
        phone = (event.currentTarget as HTMLInputElement).value;
        if (phone.trim().startsWith('+')) country = countryForInternationalNumber(phone);
    }

    function selectCountry(next: CountryCode): void {
        phone = nationalNumberForCountryChange(phone);
        country = next;
    }

    function sendPhone(): void | Promise<void> {
        const result = normalizePhoneNumber(phone, country);
        if ('error' in result) {
            failAuthSubmission('phone', result.error);
            phoneEl?.focus();
            return;
        }
        return onPhone(result.number);
    }
    let code = $state('');
    let password = $state('');
    let revealPassword = $state(false);

    // The code boxes show five cells, and more once the code is longer.
    const otp_min_cells = 5;
    const otp_cells = $derived(
        Array.from({ length: Math.max(otp_min_cells, code.length) }, (_, index) => code.charAt(index)),
    );

    let apiIdEl = $state<HTMLInputElement | null>(null);
    let apiHashEl = $state<HTMLInputElement | null>(null);
    let phoneEl = $state<HTMLInputElement | null>(null);
    let codeEl = $state<HTMLInputElement | null>(null);
    let passwordEl = $state<HTMLInputElement | null>(null);

    // A new step starts clean, while a failed request stays on the same step and
    // therefore keeps the value the user entered.
    let lastStep: string | null = null;
    $effect(() => {
        const screen = step;
        if (screen === lastStep) return;
        lastStep = screen;
        if (screen === 'code') code = '';
        if (screen === 'password') {
            password = '';
            revealPassword = false;
        }
        void tick().then(() => {
            if (step !== screen) return;
            const el = screen === 'setup' ? apiIdEl
                : screen === 'phone' ? phoneEl
                : screen === 'code' ? codeEl
                : screen === 'password' ? passwordEl
                : null;
            el?.focus();
        });
    });

    function submit(event: SubmitEvent, flow: AuthFlow, action: () => void | Promise<void>): void {
        event.preventDefault();
        if ($authSubmission[flow].busy) return;
        void action();
    }

    // The phone keyboard's return key moves on to the hash instead of
    // submitting half a credential pair.
    function advanceTo(event: KeyboardEvent, next: HTMLInputElement | null): void {
        if (event.key !== 'Enter' || event.isComposing) return;
        event.preventDefault();
        next?.focus();
    }

</script>

{#if $authScreen && updateFooter}
    <div class="auth-update-footer">
        <button type="button" onclick={() => void updateFooter.action()}>
            TDrive <span class="auth-update-accent">{updateFooter.version}</span> · {$t_store(updateFooter.label_key)}
        </button>
    </div>
{/if}

{#if step === 'welcome'}
    <section class="auth-box auth-welcome" aria-labelledby="auth-welcome-title">
        <div class="auth-page-body">
            <img class="auth-mark" src={tdriveLogo} alt="" width="112" height="112" />
            <h2 id="auth-welcome-title">{$t_store('auth.welcome_title')}</h2>
            <p class="auth-intro">
                {$t_store('auth.welcome_body')}
            </p>
        </div>
        <div class="auth-actions">
            <button class="primary-btn auth-submit" type="button" onclick={() => { welcomed = true; }}>
                {$t_store('auth.continue')}
            </button>
        </div>
    </section>
{:else if step === 'setup'}
    <form
        class="auth-box auth-form"
        aria-labelledby="auth-setup-title"
        aria-busy={$authSubmission.setup.busy}
        novalidate
        onsubmit={(event) => submit(event, 'setup', () => onSetup(apiId, apiHash))}
    >
        <div class="auth-page-body">
            <div class="auth-icon-box">
                <PaperPlaneIcon size={16} aria-hidden="true" />
            </div>
            <h2 id="auth-setup-title">{$t_store('auth.setup_title')}</h2>
            {#if mobile}
                <p id="telegram-credentials-help" class="auth-intro">
                    {$t_store('auth.setup_mobile_before')}
                    <button
                        class="auth-inline-link"
                        type="button"
                        onclick={() => openExternalUrl(TELEGRAM_APPS_URL)}
                    >my.telegram.org/apps</button>
                    {$t_store('auth.setup_mobile_after')}
                </p>
            {:else}
                <p class="auth-intro">{$t_store('auth.setup_desktop_intro')}</p>
                <ol id="telegram-credentials-help" class="auth-steps">
                    <li>{$t_store('auth.setup_step_1_open')} <strong>my.telegram.org/apps</strong> {$t_store('auth.setup_step_1_rest')}</li>
                    <li>{$t_store('auth.setup_step_2')}</li>
                    <li>{$t_store('auth.setup_step_3')}</li>
                </ol>
            {/if}
            <div class="auth-fields">
                <div class="auth-field">
                    <label for="telegram-api-id">{$t_store('auth.setup_api_id_label')}</label>
                    <div>
                        <input
                            bind:this={apiIdEl}
                            bind:value={apiId}
                            id="telegram-api-id"
                            name="api-id"
                            type="text"
                            inputmode="numeric"
                            enterkeyhint={mobile ? 'next' : undefined}
                            autocomplete="off"
                            pattern="[0-9]*"
                            placeholder="12345678"
                            required
                            disabled={$authSubmission.setup.busy}
                            aria-invalid={$authSubmission.setup.error ? 'true' : undefined}
                            aria-describedby={$authSubmission.setup.error
                                ? 'telegram-credentials-help setup-storage-note setup-error'
                                : 'telegram-credentials-help setup-storage-note'}
                            onkeydown={mobile ? (event) => advanceTo(event, apiHashEl) : undefined}
                        />
                    </div>
                </div>
                <div class="auth-field">
                    <label for="telegram-api-hash">{$t_store('auth.setup_api_hash_label')}</label>
                    <div>
                        <input
                            bind:this={apiHashEl}
                            bind:value={apiHash}
                            id="telegram-api-hash"
                            name="api-hash"
                            type="password"
                            inputmode="text"
                            enterkeyhint={mobile ? 'done' : undefined}
                            autocomplete="off"
                            autocapitalize="none"
                            spellcheck="false"
                            placeholder={$t_store('auth.setup_api_hash_placeholder')}
                            required
                            disabled={$authSubmission.setup.busy}
                            aria-invalid={$authSubmission.setup.error ? 'true' : undefined}
                            aria-describedby={$authSubmission.setup.error
                                ? 'telegram-credentials-help setup-storage-note setup-error'
                                : 'telegram-credentials-help setup-storage-note'}
                        />
                    </div>
                </div>
            </div>
            {#if mobile}
                <p id="setup-storage-note" class="auth-privacy">
                    {$t_store('auth.setup_storage_mobile')}
                </p>
            {:else}
                <p id="setup-storage-note" class="auth-privacy">
                    <LockSimpleIcon size={16} aria-hidden="true" />{$t_store('auth.setup_storage_desktop')}
                </p>
            {/if}
            <p id="setup-error" class="auth-error" aria-live="polite" aria-atomic="true">{$authSubmission.setup.error}</p>
        </div>
        <div class="auth-actions">
            <button class="primary-btn auth-submit" type="submit" disabled={$authSubmission.setup.busy}>
                {$authSubmission.setup.busy ? $t_store('auth.setup_saving') : $t_store('auth.setup_save')}
            </button>
        </div>
    </form>
{:else if step === 'phone'}
    <form
        class="auth-box auth-form"
        aria-labelledby="auth-phone-title"
        aria-busy={$authSubmission.phone.busy}
        novalidate
        onsubmit={(event) => submit(event, 'phone', sendPhone)}
    >
        <div class="auth-page-body">
            <h2 id="auth-phone-title">{$t_store('auth.phone_title')}</h2>
            <p class="auth-intro">{$t_store('auth.phone_intro')}</p>
            <div class="phone-entry">
                <div class="auth-field country-field">
                    <label for="telegram-country" class:sr-only={!mobile}>{$t_store('auth.phone_country_label')}</label>
                    <CountryCodePicker {country} disabled={$authSubmission.phone.busy} onSelect={selectCountry} />
                </div>
                <div class="auth-field phone-field">
                    <label for="telegram-phone">{$t_store('auth.phone_number_label')}</label>
                    <input
                        bind:this={phoneEl}
                        value={phone}
                        oninput={updatePhone}
                        id="telegram-phone"
                        name="phone"
                        type="tel"
                        inputmode="tel"
                        enterkeyhint={mobile ? 'send' : undefined}
                        autocomplete="tel"
                        autocapitalize="none"
                        spellcheck="false"
                        placeholder={country ? $t_store('auth.phone_placeholder') : '+1 202 555 0123'}
                        required
                        disabled={$authSubmission.phone.busy}
                        aria-invalid={$authSubmission.phone.error ? 'true' : undefined}
                        aria-describedby={$authSubmission.phone.error ? 'phone-session-note phone-error' : 'phone-session-note'}
                    />
                </div>
            </div>
            <p class="auth-intro phone-hint">{$t_store('auth.phone_hint')}</p>
            <p id="phone-session-note" class="auth-privacy">
                {$t_store('auth.phone_session_note')}
            </p>
            <p id="phone-error" class="auth-error" aria-live="polite" aria-atomic="true">{$authSubmission.phone.error}</p>
        </div>
        <div class="auth-actions">
            <button class="primary-btn auth-submit" type="submit" disabled={$authSubmission.phone.busy}>
                {$authSubmission.phone.busy ? $t_store('auth.phone_sending') : $t_store('auth.phone_send')}
            </button>
        </div>
    </form>
{:else if step === 'code'}
    <form
        class="auth-box auth-form"
        aria-labelledby="auth-code-title"
        aria-busy={$authSubmission.code.busy}
        novalidate
        onsubmit={(event) => submit(event, 'code', () => onCode(code))}
    >
        <div class="auth-page-body">
            <h2 id="auth-code-title">{$t_store('auth.code_title')}</h2>
            <p class="auth-intro">{$t_store('auth.code_intro')}</p>
            {#if $authPhone}
                <div class="auth-helper-row">
                    <span class="auth-helper-label">{$t_store('auth.code_sent_to')}</span>
                    <span class="auth-helper-pill">{$authPhone}</span>
                    <button type="button" class="auth-link" disabled={$authSubmission.code.busy} onclick={onBackToPhone}>
                        {$t_store('auth.code_change')}
                    </button>
                </div>
            {/if}
            <div class="auth-field is-last">
                <label for="telegram-code">{$t_store('auth.code_label')}</label>
                <div class="otp-field">
                <input
                    bind:this={codeEl}
                    bind:value={code}
                    id="telegram-code"
                    class="code-input otp-input"
                    name="login-code"
                    type="text"
                    inputmode="numeric"
                    enterkeyhint={mobile ? 'go' : undefined}
                    autocomplete="one-time-code"
                    autocapitalize="none"
                    spellcheck="false"
                    pattern="[0-9]*"
                    placeholder="12345"
                    required
                    disabled={$authSubmission.code.busy}
                    aria-invalid={$authSubmission.code.error ? 'true' : undefined}
                    aria-describedby={$authSubmission.code.error ? 'code-error' : undefined}
                />
                <div class="otp-cells" aria-hidden="true">
                    {#each otp_cells as cell, index (index)}
                        <span class="otp-cell" class:is-next={index === code.length}>{cell}</span>
                    {/each}
                </div>
                </div>
            </div>
            <p id="code-error" class="auth-error" aria-live="polite" aria-atomic="true">{$authSubmission.code.error}</p>
        </div>
        <div class="auth-actions">
            <button class="primary-btn auth-submit" type="submit" disabled={$authSubmission.code.busy}>
                {$authSubmission.code.busy ? $t_store('auth.code_verifying') : $t_store('auth.code_verify')}
            </button>
        </div>
        {#if !mobile}
            <p class="auth-note">{$t_store('auth.code_two_step_note')}</p>
        {/if}
    </form>
{:else if step === 'password'}
    <form
        class="auth-box auth-form"
        aria-labelledby="auth-password-title"
        aria-busy={$authSubmission.password.busy}
        novalidate
        onsubmit={(event) => submit(event, 'password', () => onPassword(password))}
    >
        <div class="auth-page-body">
            <h2 id="auth-password-title">{$t_store('auth.password_title')}</h2>
            <p class="auth-intro">{$t_store('auth.password_intro')}</p>
            {#if $authHint}
                <p id="password-hint" class="auth-caption">{$t_store('auth.password_hint_label')} <span>{$authHint}</span></p>
            {/if}
            <div class="auth-field is-last">
                <label for="telegram-password">{$t_store('auth.password_label')}</label>
                <div class="input-with-action">
                    <input
                        bind:this={passwordEl}
                        bind:value={password}
                        id="telegram-password"
                        name="password"
                        type={revealPassword ? 'text' : 'password'}
                        enterkeyhint={mobile ? 'go' : undefined}
                        autocomplete="current-password"
                        placeholder={$t_store('auth.password_placeholder')}
                        required
                        disabled={$authSubmission.password.busy}
                        aria-invalid={$authSubmission.password.error ? 'true' : undefined}
                        aria-describedby={$authSubmission.password.error
                            ? ($authHint ? 'password-hint password-error' : 'password-error')
                            : ($authHint ? 'password-hint' : undefined)}
                    />
                    <button
                        class="input-action-btn"
                        type="button"
                        data-state={revealPassword ? 'visible' : 'hidden'}
                        aria-label={revealPassword ? $t_store('auth.password_hide') : $t_store('auth.password_show')}
                        title={revealPassword ? $t_store('auth.password_hide') : $t_store('auth.password_show')}
                        disabled={$authSubmission.password.busy}
                        onclick={() => { revealPassword = !revealPassword; passwordEl?.focus(); }}
                    >
                        <EyeIcon class="input-action-icon icon-eye" size={20} aria-hidden="true" />
                        <EyeOffIcon class="input-action-icon icon-eye-off" size={20} aria-hidden="true" />
                    </button>
                </div>
            </div>
            <p id="password-error" class="auth-error" aria-live="polite" aria-atomic="true">{$authSubmission.password.error}</p>
        </div>
        <div class="auth-actions">
            <button class="primary-btn auth-submit" type="submit" disabled={$authSubmission.password.busy}>
                {$authSubmission.password.busy ? $t_store('auth.password_unlocking') : $t_store('auth.password_unlock')}
            </button>
        </div>
    </form>
{:else if step === 'drive'}
    <PersonalDriveSetup
        phase={$personalDriveSetup.phase}
        candidates={$personalDriveSetup.candidates}
        error={$personalDriveSetup.error}
        detail={$personalDriveSetup.detail}
        scan={$personalDriveSetup.scan}
        waitSeconds={$personalDriveSetup.waitSeconds}
        createRetry={$personalDriveSetup.createRetry}
        onSelect={onDriveSelect}
        onCreate={onDriveCreate}
        onRetry={onDriveRetry}
    />
{/if}

<style>
    /* Desktop card: the mockup's rhythm. Every rule that draws a box or changes
       a control's size is scoped to a desktop page, so the phone rules below
       keep their own geometry. */
    /* One visible label, above the prefix box and the number. On desktop the two
       field wrappers dissolve into the grid, so the phone label can span the row
       while the picker's own label stays for assistive technology only. */
    .phone-entry {
        display: grid;
        grid-template-columns: auto minmax(0, 1fr);
        column-gap: var(--space-2);
    }

    :global(html:not(.mobile)) .country-field,
    :global(html:not(.mobile)) .phone-field {
        display: contents;
    }

    :global(html:not(.mobile)) .phone-field > label {
        grid-row: 1;
        grid-column: 1 / -1;
    }

    :global(html:not(.mobile)) .country-field > :global(.country-code-trigger) {
        grid-row: 2;
        grid-column: 1;
    }

    :global(html:not(.mobile)) .phone-field > input {
        grid-row: 2;
        grid-column: 2;
    }

    .sr-only {
        position: absolute;
        width: 1px;
        height: 1px;
        padding: 0;
        margin: -1px;
        overflow: hidden;
        clip: rect(0, 0, 0, 0);
        white-space: nowrap;
        border: 0;
    }

    .phone-hint {
        margin: var(--space-2) 0 var(--space-6) 0;
        font-size: var(--type-sm);
        color: var(--color-text-subtle);
    }

    .auth-form {
        display: flex;
        flex-direction: column;
    }

    .auth-steps {
        margin: 0 0 var(--space-6) 0;
        padding: var(--space-3) var(--space-3) var(--space-3) var(--space-8);
        color: var(--color-text-muted);
        font-size: var(--type-sm);
        line-height: 1.8;
        background: var(--color-surface-2);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
    }

    .auth-steps strong {
        color: var(--color-text-soft);
        font-weight: 600;
    }

    .auth-privacy {
        margin: 0 0 var(--space-6) 0;
        color: var(--color-text-subtle);
        font-size: var(--type-xs);
        line-height: 1.5;
        text-align: left;
    }

    .auth-privacy :global(svg) {
        display: inline-block;
        margin-right: var(--space-1-5);
        vertical-align: -2px;
    }

    .auth-fields {
        display: grid;
        gap: var(--space-4);
        margin-bottom: var(--space-6);
    }

    .auth-field {
        text-align: left;
    }

    .auth-field > label {
        display: block;
        margin-bottom: var(--space-2);
        color: var(--color-text-muted);
        font-size: var(--type-sm);
        font-weight: 500;
        letter-spacing: 0.04em;
        text-transform: uppercase;
    }

    .auth-field input {
        margin-bottom: 0;
    }

    .auth-field input[aria-invalid='true'] {
        border-color: var(--danger);
    }

    .auth-field.is-last {
        margin-bottom: var(--space-6);
    }

    .auth-error {
        margin: 0 0 var(--space-4) 0;
        color: var(--danger);
        font-size: var(--type-sm);
        line-height: 1.4;
        text-align: left;
    }

    .auth-error:empty {
        display: none;
    }

    .auth-caption {
        margin: 0 0 var(--space-6) 0;
        padding: var(--space-3) var(--space-4);
        background: var(--color-surface-2);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
    }

    /* Five boxes mirror the one real input, which keeps paste, autofill and the
       one-time-code hint. The input sits over the boxes, transparent. */
    .otp-field {
        position: relative;
        width: max-content;
    }

    .otp-cells {
        display: flex;
        gap: var(--space-2);
    }

    .otp-cell {
        display: flex;
        align-items: center;
        justify-content: center;
        width: 40px;
        height: 32px;
        color: var(--color-text);
        font-size: var(--type-base);
        font-weight: 500;
        font-variant-numeric: tabular-nums;
        background: var(--color-canvas);
        border: 1px solid var(--color-border);
        border-radius: var(--radius-sm);
    }

    .otp-field:focus-within .otp-cell.is-next {
        border-color: var(--accent);
        box-shadow: var(--focus-ring);
    }

    .auth-note {
        margin: var(--space-4) 0 0 0;
        padding: var(--space-3) var(--space-4);
        color: var(--color-text-muted);
        font-size: var(--type-sm);
        line-height: 1.6;
        background: var(--color-surface-2);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
    }

    .auth-form button:disabled,
    .auth-form input:disabled {
        cursor: not-allowed;
        opacity: 0.62;
    }

    .auth-form .primary-btn:disabled:hover {
        background: var(--accent);
        transform: none;
    }

    .input-with-action input {
        padding-right: var(--space-8);
    }

    .input-with-action .input-action-btn {
        right: var(--space-1);
        width: 24px;
        height: 24px;
        border-radius: var(--radius-xs);
    }

    .input-with-action :global(.input-action-icon) {
        width: 16px;
        height: 16px;
    }

    /* On desktop the single input is laid over the boxes, transparent, so a
       click or a paste lands in it. Phones keep the plain field. */
    :global(html:not(.mobile)) .otp-input {
        position: absolute;
        inset: 0;
        width: 100%;
        height: 100%;
        min-height: 0;
        padding: 0;
        margin: 0;
        border: 0;
        opacity: 0;
        cursor: text;
        font-size: 16px;
        box-shadow: none;
    }

    :global(html.mobile) .otp-cells {
        display: none;
    }

    /* Phone pages. The wrapper, inputs and action bar are restyled in
       auth.css; this block only re-tunes the component's own type and
       spacing to the mobile scale (15 body, 13 fine print, 16 inputs). */
    :global(html.mobile) .phone-entry {
        display: grid;
        grid-template-columns: minmax(115px, 0.8fr) minmax(0, 1.4fr);
        gap: 12px;
        margin-bottom: 0.75rem;
    }

    :global(html.mobile) .auth-intro {
        margin: 0 0 1.5rem;
        font-size: 0.9375rem;
        line-height: 1.5;
    }

    :global(html.mobile) .auth-welcome .auth-intro {
        margin-bottom: 0;
    }

    /* Optically centred, not mathematically. A block placed at exact centre on
       a tall phone screen reads as having sunk, because the eye gives the upper
       half more weight; lifting it by a tenth of the viewport puts it where it
       looks centred. The padding is what does the lifting, so the block still
       falls back to a true centre if the text grows enough to fill the space. */
    :global(html.mobile) .auth-welcome .auth-page-body {
        padding-bottom: 10vh;
    }

    /* Centred and large: on the one screen with no content to compete with,
       the logo is the thing that says whose app this is. */
    :global(html.mobile) .auth-mark {
        display: block;
        width: 112px;
        height: auto;
        margin: 0 auto 1.75rem;
    }

    :global(html.mobile) .auth-welcome h2 {
        font-size: 1.75rem;
    }

    :global(html.mobile) .auth-inline-link {
        padding: 0;
        color: var(--accent);
        font: inherit;
        font-weight: 600;
        background: none;
        border: 0;
        border-radius: var(--radius-xs);
        cursor: pointer;
    }

    :global(html.mobile) .auth-inline-link:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    /* Fine print follows the inline error, so an error sits right under
       the field it belongs to. */
    :global(html.mobile) .auth-privacy {
        order: 1;
        margin: 0 0 1rem;
        font-size: 0.8125rem;
        color: var(--color-text-muted);
    }

    :global(html.mobile) .auth-privacy :global(svg) {
        display: none;
    }

    :global(html.mobile) .auth-fields {
        gap: 1rem;
        margin-bottom: 1.25rem;
    }

    :global(html.mobile) .auth-field > label {
        margin-bottom: 0.375rem;
        color: var(--color-text-soft);
        font-size: 0.8125rem;
        font-weight: 600;
        letter-spacing: normal;
        text-transform: none;
    }

    :global(html.mobile) .auth-field:not(.auth-fields .auth-field) {
        margin-bottom: 1.25rem;
    }

    :global(html.mobile) .phone-entry .auth-field {
        margin-bottom: 0;
    }

    :global(html.mobile) .auth-error:empty {
        display: block;
    }

    :global(html.mobile) .auth-error {
        margin: -0.25rem 0 1rem;
        font-size: 0.875rem;
        line-height: 1.45;
    }

    :global(html.mobile) .auth-caption {
        margin: -0.75rem 0 1rem;
        padding: 0;
        background: none;
        border: 0;
        font-size: 0.875rem;
    }

    :global(html.mobile) .auth-submit {
        margin-top: 0;
    }

    :global(html.mobile) .auth-form button:disabled,
    :global(html.mobile) .auth-form input:disabled {
        cursor: default;
    }

    :global(html.mobile) .phone-hint {
        margin-top: 0;
    }

    /* The state matrix wants the loading state inside the button. The label
       already reads "Sending code", so the ring is decoration for sighted
       users and stays out of the accessibility tree. */
    :global(html.mobile) .auth-form[aria-busy='true'] .auth-submit::before {
        display: inline-block;
        width: 16px;
        height: 16px;
        margin-right: 8px;
        vertical-align: -3px;
        content: '';
        border: 2px solid currentColor;
        border-top-color: transparent;
        border-radius: 50%;
        animation: auth-submit-spin 720ms linear infinite;
    }

    @keyframes auth-submit-spin {
        to { transform: rotate(360deg); }
    }
</style>
