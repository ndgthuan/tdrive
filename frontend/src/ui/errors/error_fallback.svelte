<script lang="ts">
    import CloudAlertIcon from 'phosphor-svelte/lib/CloudWarningIcon';
    import RefreshCwIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import RotateCcwIcon from 'phosphor-svelte/lib/ArrowCounterClockwiseIcon';
    import { onMount } from 'svelte';
    import { formatAppErrorDiagnostic, type AppError } from '../../modules/errors';
    import Button from '../button.svelte';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        error: AppError;
        onRecover: () => void;
        onReload: () => void;
    }

    let { error, onRecover, onReload }: Props = $props();
    let heading: HTMLHeadingElement;
    const diagnostic = $derived(formatAppErrorDiagnostic(error));

    onMount(() => heading.focus());
</script>

<main class="error-recovery" role="alert" aria-labelledby="error-recovery-title">
    <section class="recovery-panel">
        <div class="recovery-signal" aria-hidden="true"></div>

        <div class="recovery-icon" aria-hidden="true">
            <CloudAlertIcon size={28} />
        </div>

        <div class="recovery-content">
            <h1 id="error-recovery-title" tabindex="-1" bind:this={heading}>{error.title}</h1>
            <p class="recovery-message">{error.message}</p>
            <p class="recovery-assurance">
                {$t_store('misc.recovery_assurance')}
            </p>

            <div class="recovery-actions">
                <Button onclick={onRecover}>
                    <RotateCcwIcon size={16} aria-hidden="true" />
                    {$t_store('misc.restore_screen')}
                </Button>
                <Button variant="secondary" onclick={onReload}>
                    <RefreshCwIcon size={16} aria-hidden="true" />
                    {$t_store('misc.reload_tdrive')}
                </Button>
            </div>

            <details class="recovery-details">
                <summary>{$t_store('misc.technical_details')}</summary>
                <pre>{diagnostic}</pre>
            </details>
        </div>
    </section>
</main>

<style>
    .error-recovery {
        min-height: 100vh;
        min-height: 100dvh;
        display: grid;
        place-items: center;
        padding: clamp(var(--space-5), 6vw, 64px);
        color: var(--text-main);
        background: var(--color-canvas);
    }

    .recovery-panel {
        width: min(680px, 100%);
        display: grid;
        grid-template-columns: 6px 64px minmax(0, 1fr);
        align-items: start;
        gap: var(--space-5);
        padding: clamp(var(--space-5), 5vw, 44px);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        background: var(--color-surface-0);
        box-shadow: var(--shadow-md);
    }

    .recovery-signal {
        align-self: stretch;
        min-height: 168px;
        border-radius: var(--radius-pill);
        background: var(--danger);
    }

    .recovery-icon {
        width: 56px;
        height: 56px;
        display: grid;
        place-items: center;
        border: 1px solid var(--overlay-danger-strong);
        border-radius: var(--radius-lg);
        color: var(--danger);
        background: var(--overlay-danger-soft);
    }

    .recovery-content {
        min-width: 0;
    }

    h1 {
        color: var(--color-text);
        font-size: clamp(1.45rem, 4vw, 1.9rem);
        font-weight: var(--weight-strong);
        line-height: 1.15;
        letter-spacing: -0.025em;
        outline: none;
    }

    h1:focus-visible {
        border-radius: var(--radius-xs);
        box-shadow: var(--focus-ring);
    }

    .recovery-message {
        max-width: 56ch;
        margin-top: var(--space-3);
        color: var(--text-main);
        font-size: var(--type-md);
        line-height: 1.55;
    }

    .recovery-assurance {
        max-width: 62ch;
        margin-top: var(--space-3);
        color: var(--text-muted);
        font-size: var(--type-sm);
        line-height: 1.55;
    }

    .recovery-actions {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-3);
        margin-top: var(--space-6);
    }

    .recovery-details {
        margin-top: var(--space-5);
        color: var(--text-muted);
        font-size: var(--type-xs);
    }

    .recovery-details summary {
        width: fit-content;
        padding: var(--space-1) 0;
        color: var(--text-muted);
        cursor: pointer;
    }

    .recovery-details summary:focus-visible {
        outline: none;
        border-radius: var(--radius-xs);
        box-shadow: var(--focus-ring);
    }

    .recovery-details pre {
        max-height: 180px;
        overflow: auto;
        margin-top: var(--space-2);
        padding: var(--space-3);
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        color: var(--text-muted);
        background: var(--color-surface-1);
        font: 0.72rem/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
        overflow-wrap: anywhere;
        white-space: pre-wrap;
        user-select: text;
    }

    @media (max-width: 560px) {
        .recovery-panel {
            grid-template-columns: 5px minmax(0, 1fr);
            gap: var(--space-4);
        }

        .recovery-icon {
            display: none;
        }

        .recovery-actions {
            align-items: stretch;
            flex-direction: column;
        }
    }

</style>
