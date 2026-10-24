<script lang="ts">
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import PlusIcon from 'phosphor-svelte/lib/PlusIcon';
    import RotateCwIcon from 'phosphor-svelte/lib/ArrowClockwiseIcon';
    import { isMobilePlatform } from '../../api';
    import type { PersonalDriveCandidate } from '../../types';
    import type { DriveScanProgress, PersonalDrivePhase } from './personal_drive_store';
    import { format_date, format_number, t_store } from '../../i18n/i18n';

    interface Props {
        phase: PersonalDrivePhase;
        candidates: PersonalDriveCandidate[];
        error: string;
        detail?: string;
        createRetry?: boolean;
        scan?: DriveScanProgress | null;
        waitSeconds?: number;
        onSelect: (channelID: string) => void;
        onCreate: () => void;
        onRetry: () => void | Promise<void>;
    }

    let {
        phase,
        candidates,
        error,
        detail = '',
        createRetry = false,
        scan = null,
        waitSeconds = 0,
        onSelect,
        onCreate,
        onRetry,
    }: Props = $props();

    // Phones move the primary and create actions into the bottom bar; the
    // desktop card keeps them in reading order under the list. With no
    // channel to pick, creating is the only way forward and takes the
    // primary look there.
    const mobile = isMobilePlatform();
    const createIsPrimary = $derived(mobile && candidates.length === 0);

    let selectedID = $state('');
    let confirmingCreate = $state(false);
    let retrying = $state(false);

    const busy = $derived(phase === 'loading' || phase === 'recovering');
    const recovering = $derived(phase === 'recovering');

    // Counting walks the channel backwards to size the job, so no total is
    // known yet; only the applying pass can fill a determinate bar.
    const percent = $derived.by(() => {
        if (!scan || scan.phase !== 'applying' || scan.messages_total <= 0) return null;
        const ratio = scan.messages_done / scan.messages_total;
        return Math.max(0, Math.min(100, Math.round(ratio * 100)));
    });

    let waitRemaining = $state(0);

    // Telegram's pauses are the only stage long enough to look like a hang,
    // so count them down locally rather than showing a frozen number.
    $effect(() => {
        if (waitSeconds <= 0) {
            waitRemaining = 0;
            return;
        }
        let left = waitSeconds;
        waitRemaining = left;
        const timer = setInterval(() => {
            left -= 1;
            waitRemaining = Math.max(0, left);
            if (left <= 0) clearInterval(timer);
        }, 1000);
        return () => clearInterval(timer);
    });

    // Keys plus params rather than text, so the label re-renders on a language change.
    const scanLabel = $derived.by((): { key: string; params?: Record<string, string | number> } => {
        if (waitRemaining > 0) return { key: 'auth.drive_scan_wait', params: { seconds: waitRemaining } };
        if (!scan) return { key: 'auth.drive_scan_reading' };
        if (scan.phase === 'counting') {
            return { key: 'auth.drive_scan_counting', params: { done: formatCount(scan.messages_done) } };
        }
        return {
            key: 'auth.drive_scan_rebuilding',
            params: { done: formatCount(scan.messages_done), total: formatCount(scan.messages_total) },
        };
    });

    $effect(() => {
        if (selectedID && !candidates.some((candidate) => candidate.id === selectedID)) {
            selectedID = '';
        }
        if (phase !== 'ready' || createRetry) confirmingCreate = false;
    });

    function formatCount(value: number): string {
        return format_number(value);
    }

    function formatCreated(timestamp: number): string {
        if (!Number.isFinite(timestamp) || timestamp <= 0) return '';
        return format_date(new Date(timestamp * 1000), 'medium');
    }

    function submitSelection(): void {
        if (!selectedID || busy) return;
        onSelect(selectedID);
    }

    function onChoiceKeydown(event: KeyboardEvent): void {
        if (event.key !== 'Enter') return;
        event.preventDefault();
        submitSelection();
    }

    // Discovery only changes phase once the backend answers, so the button
    // guards itself against a second tap until then.
    async function retry(): Promise<void> {
        if (retrying) return;
        retrying = true;
        try {
            await onRetry();
        } finally {
            retrying = false;
        }
    }
</script>

{#snippet continueButton()}
    <button
        class="primary-btn drive-primary"
        class:is-busy={recovering}
        data-drive-continue
        type="button"
        disabled={!selectedID || busy}
        onclick={submitSelection}
    >
        {recovering ? $t_store('auth.drive_recovering') : mobile ? $t_store('auth.drive_use') : $t_store('auth.continue')}
    </button>
{/snippet}

{#snippet retryButton()}
    <button
        class="primary-btn drive-primary"
        class:is-busy={retrying}
        data-drive-retry
        type="button"
        disabled={retrying}
        onclick={() => void retry()}
    >
        <RotateCwIcon size={16} aria-hidden="true" />
        {retrying ? $t_store('auth.drive_retrying') : $t_store('auth.drive_retry')}
    </button>
{/snippet}

{#snippet createActions()}
    {#if createRetry}
        <div class="drive-secondary drive-secondary-stack">
            <button
                class="drive-ghost"
                class:is-primary={createIsPrimary}
                data-drive-create-retry
                type="button"
                disabled={busy}
                onclick={onCreate}
            >
                <RotateCwIcon size={16} aria-hidden="true" />
                {$t_store('auth.drive_retry_setup')}
            </button>
            <span class="drive-hint">{$t_store('auth.drive_retry_hint')}</span>
        </div>
    {:else if confirmingCreate}
        <div class="drive-confirm" role="group" aria-label={$t_store('auth.drive_confirm_aria')}>
            <div>
                <strong>{$t_store('auth.drive_confirm_question')}</strong>
                <span>{$t_store('auth.drive_confirm_detail')}</span>
            </div>
            <div class="drive-confirm-actions">
                <button class="drive-ghost" type="button" disabled={busy} onclick={() => { confirmingCreate = false; }}>
                    {$t_store('auth.cancel')}
                </button>
                <button class="drive-confirm-create" data-drive-create-confirm type="button" disabled={busy} onclick={onCreate}>
                    {$t_store('auth.drive_create')}
                </button>
            </div>
        </div>
    {:else}
        <button
            class="drive-ghost drive-secondary"
            class:is-primary={createIsPrimary}
            data-drive-create-request
            type="button"
            disabled={busy}
            onclick={() => { confirmingCreate = true; }}
        >
            <PlusIcon size={16} aria-hidden="true" />
            {$t_store('auth.drive_create_new')}
        </button>
    {/if}
{/snippet}

<section class="auth-box drive-setup" aria-labelledby="drive-setup-title">
    <div class="auth-page-body">
        <h2 id="drive-setup-title">{$t_store('auth.drive_setup_title')}</h2>
        <p>{$t_store('auth.drive_setup_intro')}</p>

        {#if phase === 'loading'}
            <div class="drive-panel" role="status" aria-live="polite">
                <span class="drive-spinner" aria-hidden="true"></span>
                <strong>{$t_store('auth.drive_loading_title')}</strong>
                <span>{$t_store('auth.drive_loading_detail')}</span>
            </div>
        {:else if phase === 'discovery-error'}
            <div class="drive-alert" role="alert">
                <strong>{error}</strong>
                {#if detail}<span>{detail}</span>{/if}
            </div>
            {#if !mobile}
                {@render retryButton()}
            {/if}
        {:else}
            {#if error}
                <div class="drive-alert" role="alert">
                    <strong>{error}</strong>
                    {#if detail}<span>{detail}</span>{/if}
                </div>
            {/if}

            {#if candidates.length > 0}
                <fieldset class="drive-list" disabled={busy}>
                    <legend class="sr-only">{$t_store('auth.drive_channels_legend')}</legend>
                    {#each candidates as candidate (candidate.id)}
                        <label class="drive-choice" class:selected={selectedID === candidate.id}>
                            <input
                                type="radio"
                                name="personal-drive"
                                value={candidate.id}
                                bind:group={selectedID}
                                disabled={busy}
                                onkeydown={onChoiceKeydown}
                            />
                            <span class="drive-choice-copy">
                                <span class="drive-choice-title">
                                    <span class="drive-title-text" title={candidate.title || $t_store('auth.drive_untitled')}>
                                        {candidate.title || $t_store('auth.drive_untitled')}
                                    </span>
                                    {#if candidate.recommended}
                                        <span class="drive-badge">{$t_store('auth.drive_recommended')}</span>
                                    {/if}
                                </span>
                                <span class="drive-choice-meta">
                                    <span class:in-use={candidate.hasActivity}>
                                        {candidate.hasActivity ? $t_store('auth.drive_in_use') : $t_store('auth.drive_empty')}
                                    </span>
                                    {#if formatCreated(candidate.createdAt)}
                                        <span>{$t_store('auth.drive_created', { date: formatCreated(candidate.createdAt) })}</span>
                                    {/if}
                                    <span>{$t_store('auth.drive_id', { id: candidate.id })}</span>
                                </span>
                            </span>
                            <span class="drive-check" aria-hidden="true">
                                <CheckIcon size={13} weight="bold" />
                            </span>
                        </label>
                    {/each}
                </fieldset>

                {#if !mobile}
                    {@render continueButton()}
                {/if}
            {:else}
                <div class="drive-panel">
                    <strong>{$t_store('auth.drive_no_channels_title')}</strong>
                    <span>{$t_store('auth.drive_no_channels_detail')}</span>
                </div>
            {/if}

            {#if recovering}
                <div class="drive-scan" data-drive-scan>
                    <div
                        class="drive-scan-track"
                        role="progressbar"
                        aria-label={$t_store('auth.drive_progress_label')}
                        aria-valuemin={0}
                        aria-valuemax={100}
                        aria-valuenow={percent ?? undefined}
                    >
                        <span
                            class="drive-scan-fill"
                            class:indeterminate={percent === null}
                            style={percent === null ? undefined : `width: ${percent}%`}
                        ></span>
                    </div>
                    <span class="drive-hint" role="status" aria-live="polite">{$t_store(scanLabel.key, scanLabel.params)}</span>
                </div>
            {/if}

            {#if !mobile}
                {@render createActions()}
            {/if}
        {/if}
    </div>

    {#if mobile && phase !== 'loading'}
        <div class="auth-actions">
            {#if phase === 'discovery-error'}
                {@render retryButton()}
            {:else}
                {#if candidates.length > 0}
                    {@render continueButton()}
                {/if}
                {@render createActions()}
            {/if}
        </div>
    {/if}
</section>

<style>
    /* Extends the shared .auth-box card: same surface, border and radius as
       the login screens, sized to the mockup's 480px card for the channel list. */
    .drive-setup {
        width: min(480px, calc(100vw - 32px));
        max-height: calc(100vh - 48px);
        padding: var(--space-8);
        overflow: auto;
    }

    .drive-list {
        display: grid;
        max-height: min(320px, 40vh);
        padding: 0;
        margin: 0 0 var(--space-4) 0;
        overflow: auto;
        text-align: left;
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
    }

    .drive-choice {
        position: relative;
        display: grid;
        grid-template-columns: minmax(0, 1fr) 16px;
        gap: var(--space-3);
        align-items: center;
        padding: var(--space-2-5) var(--space-3);
        cursor: pointer;
        background: var(--color-surface-1);
        transition: background var(--motion-fast) var(--ease-standard);
    }
    .drive-choice + .drive-choice { border-top: 1px solid var(--color-border-soft); }
    .drive-choice:hover { background: var(--color-surface-2); }
    .drive-choice.selected { background: var(--overlay-accent-1); }
    .drive-choice:has(input:focus-visible) { box-shadow: inset 0 0 0 2px var(--color-accent); }
    .drive-choice input {
        position: absolute;
        width: 1px;
        height: 1px;
        margin: -1px;
        opacity: 0;
        pointer-events: none;
    }

    .drive-choice-copy { display: grid; gap: var(--space-0-5); min-width: 0; }
    .drive-choice-title {
        display: flex;
        gap: var(--space-2);
        align-items: center;
        min-width: 0;
        color: var(--color-text);
        font-size: var(--type-base);
        font-weight: 500;
    }
    .drive-title-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .drive-badge {
        flex: 0 0 auto;
        padding: 1px var(--space-1-5);
        color: var(--accent);
        font-size: var(--type-2xs);
        font-weight: 500;
        background: var(--overlay-accent-1);
        border-radius: var(--radius-pill);
    }
    .drive-choice-meta { display: flex; flex-wrap: wrap; color: var(--color-text-subtle); font-size: var(--type-xs); }
    .drive-choice-meta > span + span::before { margin: 0 var(--space-1-5); content: '·'; }
    .drive-choice-meta .in-use { color: var(--color-success); font-weight: 500; }

    .drive-check {
        display: grid;
        width: 16px;
        height: 16px;
        color: transparent;
        border: 1px solid var(--color-border);
        border-radius: 50%;
        place-items: center;
        transition:
            background var(--motion-fast) var(--ease-standard),
            border-color var(--motion-fast) var(--ease-standard);
    }
    .selected .drive-check {
        color: var(--color-on-accent);
        background: var(--accent);
        border-color: var(--accent);
    }

    .drive-primary { display: inline-flex; gap: var(--space-2); align-items: center; justify-content: center; }
    .drive-primary:disabled { opacity: 0.5; cursor: default; transform: none; }
    .drive-primary:disabled:hover { background: var(--accent); }

    .drive-hint {
        display: block;
        margin-top: var(--space-2);
        color: var(--color-text-subtle);
        font-size: var(--type-xs);
        line-height: 1.45;
    }

    .drive-scan { display: grid; gap: var(--space-2); margin-top: var(--space-4); }
    .drive-scan .drive-hint { margin-top: 0; }

    .drive-scan-track {
        height: 4px;
        overflow: hidden;
        background: var(--color-surface-2);
        border-radius: var(--radius-pill);
    }
    .drive-scan-fill {
        display: block;
        width: 0;
        height: 100%;
        background: var(--accent);
        border-radius: inherit;
        transition: width var(--motion-med) var(--ease-standard);
    }
    .drive-scan-fill.indeterminate {
        width: 38%;
        animation: drive-scan-slide 1.25s var(--ease-standard) infinite;
    }

    .drive-panel {
        display: grid;
        gap: var(--space-1-5);
        justify-items: center;
        padding: var(--space-6) var(--space-5);
        margin-bottom: var(--space-4);
        color: var(--color-text-muted);
        font-size: var(--type-sm);
        background: var(--color-surface-2);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
    }
    .drive-panel strong { color: var(--color-text); font-size: var(--type-base); font-weight: 500; }

    .drive-alert {
        display: grid;
        gap: var(--space-1);
        padding: var(--space-3) var(--space-4);
        margin-bottom: var(--space-4);
        color: var(--color-danger);
        font-size: var(--type-sm);
        text-align: left;
        background: color-mix(in srgb, var(--color-danger) 8%, transparent);
        border: 1px solid color-mix(in srgb, var(--color-danger) 22%, transparent);
        border-radius: var(--radius-sm);
    }
    .drive-alert span { color: var(--color-text-muted); font-size: var(--type-xs); overflow-wrap: anywhere; }

    .drive-spinner {
        width: 16px;
        height: 16px;
        border: 2px solid var(--color-border-soft);
        border-top-color: var(--accent);
        border-radius: 50%;
        animation: drive-spin 800ms linear infinite;
    }

    .drive-ghost {
        display: inline-flex;
        gap: var(--space-1-5);
        align-items: center;
        justify-content: center;
        min-height: 32px;
        padding: 0 var(--space-3);
        color: var(--color-text-soft);
        font: inherit;
        font-size: var(--type-sm);
        font-weight: 500;
        background: transparent;
        border: 0;
        border-radius: var(--radius-sm);
        cursor: pointer;
        transition:
            color var(--motion-fast) var(--ease-standard),
            background var(--motion-fast) var(--ease-standard);
    }
    .drive-ghost:not(:disabled):hover { color: var(--color-text); background: var(--color-surface-3); }
    /* Neutral secondary: a bordered 32px control, full width, under the list. */
    :global(html:not(.mobile)) .drive-ghost.drive-secondary {
        width: 100%;
        background: var(--color-surface-2);
        border: 1px solid var(--color-border-soft);
        color: var(--color-text);
    }
    :global(html:not(.mobile)) .drive-ghost.drive-secondary:not(:disabled):hover {
        background: var(--color-surface-3);
    }
    .drive-secondary { margin-top: var(--space-3); }
    .drive-secondary-stack { display: grid; gap: var(--space-0-5); justify-items: center; }
    .drive-secondary-stack .drive-hint { margin-top: 0; }

    .drive-confirm {
        display: flex;
        gap: var(--space-3-5);
        align-items: center;
        justify-content: space-between;
        padding: var(--space-3) var(--space-4);
        margin-top: var(--space-3);
        text-align: left;
        background: var(--color-surface-2);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
    }
    .drive-confirm > div:first-child { display: grid; gap: var(--space-0-5); }
    .drive-confirm strong { color: var(--color-text); font-size: var(--type-sm); font-weight: 500; }
    .drive-confirm span { color: var(--color-text-muted); font-size: var(--type-xs); }
    .drive-confirm-actions { display: flex; flex: 0 0 auto; gap: var(--space-1-5); }
    .drive-confirm-create {
        min-height: 32px;
        padding: 0 var(--space-3);
        color: var(--color-text);
        font: inherit;
        font-size: var(--type-sm);
        font-weight: 500;
        background: var(--color-surface-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        cursor: pointer;
    }

    button:disabled { opacity: 0.5; cursor: default; }
    button:focus-visible { outline: none; box-shadow: var(--focus-ring); }

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

    @keyframes drive-spin { to { transform: rotate(360deg); } }
    @keyframes drive-scan-slide {
        from { transform: translateX(-100%); }
        to { transform: translateX(300%); }
    }

    @media (max-width: 520px) {
        .drive-setup { padding: var(--space-6) var(--space-5); }
        .drive-confirm { flex-direction: column; align-items: stretch; }
        .drive-confirm-actions button { flex: 1; }
    }

    @media (prefers-reduced-motion: reduce) {
        .drive-spinner { animation-duration: 1.5s; }
        .drive-scan-fill { transition: none; }
        .drive-scan-fill.indeterminate { width: 100%; animation: none; }
        .drive-choice, .drive-check, .drive-ghost { transition: none; }
    }

    /* Phone page: the list scrolls with the body and the actions sit in the
       bottom bar (auth.css owns the page frame; this tunes the picker). */
    :global(html.mobile) .drive-setup {
        width: 100%;
        max-height: none;
        padding: 0;
        overflow-y: auto;
    }
    :global(html.mobile) .auth-page-body > p {
        max-width: none;
        margin: 0 0 1.5rem;
    }
    :global(html.mobile) .drive-list {
        max-height: none;
        margin: 0;
        overflow: hidden;
        border-radius: var(--radius-lg);
    }
    :global(html.mobile) .drive-choice {
        min-height: 56px;
        padding: 12px 16px;
        background: var(--color-surface-1);
    }
    :global(html.mobile) .drive-choice-title { font-size: 0.9375rem; font-weight: 600; }
    :global(html.mobile) .drive-choice-meta { font-size: 0.8125rem; }
    :global(html.mobile) .drive-badge { font-size: 0.75rem; font-weight: 600; }
    :global(html.mobile) .drive-check { width: 22px; height: 22px; }
    :global(html.mobile) .drive-panel {
        padding: 32px 20px;
        margin: 0;
        font-size: 0.875rem;
        border-radius: var(--radius-lg);
        background: var(--color-surface-1);
    }
    :global(html.mobile) .drive-panel strong { font-size: 1rem; font-weight: 600; }
    :global(html.mobile) .drive-alert {
        font-size: 0.9375rem;
        border-radius: var(--radius-lg);
    }
    :global(html.mobile) .drive-alert span { font-size: 0.8125rem; }
    :global(html.mobile) .drive-scan { margin-top: 16px; }
    :global(html.mobile) .drive-hint { font-size: 0.8125rem; line-height: 1.5; margin-top: 10px; }
    :global(html.mobile) .drive-primary {
        min-height: 50px;
        font-size: 1rem;
        font-weight: 600;
        border-radius: var(--radius-lg);
    }
    :global(html.mobile) .drive-primary:hover { transform: none; }
    :global(html.mobile) .drive-primary.is-busy::before {
        width: 16px;
        height: 16px;
        content: '';
        border: 2px solid currentColor;
        border-top-color: transparent;
        border-radius: 50%;
        animation: drive-spin 720ms linear infinite;
    }
    :global(html.mobile) .drive-primary.is-busy > :global(svg) { display: none; }
    :global(html.mobile) .drive-ghost {
        min-height: 44px;
        font-size: 0.9375rem;
        font-weight: 600;
    }
    :global(html.mobile) .drive-secondary {
        width: 100%;
        margin-top: 4px;
    }
    :global(html.mobile) .drive-ghost.is-primary {
        width: 100%;
        min-height: 50px;
        margin-top: 0;
        color: var(--color-on-accent);
        font-size: 1rem;
        font-weight: var(--weight-strong);
        background: var(--accent);
        border-radius: var(--radius-lg);
    }
    :global(html.mobile) .drive-ghost.is-primary:not(:disabled):hover {
        color: var(--color-on-accent);
        background: var(--accent);
    }
    :global(html.mobile) .drive-secondary-stack { gap: 0; }
    :global(html.mobile) .drive-confirm {
        flex-direction: column;
        align-items: stretch;
        margin-top: 4px;
        border-radius: var(--radius-lg);
    }
    :global(html.mobile) .drive-confirm strong { font-size: 0.9375rem; font-weight: 600; }
    :global(html.mobile) .drive-confirm span { font-size: 0.8125rem; }
    :global(html.mobile) .drive-confirm-actions button {
        flex: 1;
        min-height: 44px;
    }
    :global(html.mobile) .drive-confirm-create {
        font-size: 0.9375rem;
        font-weight: 600;
        color: var(--color-on-accent);
        background: var(--accent);
        border: 0;
        border-radius: var(--radius-md);
    }
</style>
