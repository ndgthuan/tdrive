<script lang="ts">
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import LoaderCircleIcon from 'phosphor-svelte/lib/CircleNotchIcon';
    import CircleAlertIcon from 'phosphor-svelte/lib/WarningCircleIcon';
    import type { RingState } from './mobile_shell_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        status: RingState;
        /** Opens the transfer queue. Absent where there is nothing to open. */
        onOpenQueue?: () => void;
    }

    let { status, onOpenQueue }: Props = $props();

    // The header only earns a status mark while something needs attention.
    // An idle hollow ring competes with the drive switcher yet says nothing the
    // user can act on, so it is intentionally absent at rest.
    //
    // The label is the state in words, because a 16px glyph is not a sentence:
    // it is what a screen reader reads and what a returning user needs when the
    // shape alone is ambiguous.
    const label = $derived(
        status === 'active' ? $t_store('mobile.sync_working')
        : status === 'attention' ? $t_store('mobile.sync_attention')
        : status === 'failed' ? $t_store('mobile.sync_failed')
        : $t_store('mobile.sync_up_to_date'),
    );

    const Glyph = $derived(
        status === 'active' ? LoaderCircleIcon
        : status === 'attention' || status === 'failed' ? CircleAlertIcon
        : CheckIcon,
    );

    // Only the states the queue can explain take a tap. A mark that opens an
    // unrelated screen is worse than one that does nothing.
    const tappable = $derived(Boolean(onOpenQueue) && status !== 'idle');
</script>

{#if status !== 'idle' && tappable}
    <button
        class="sync-ring is-tappable"
        data-state={status}
        type="button"
        aria-label={$t_store('mobile.open_transfers_label', { label })}
        onclick={() => onOpenQueue?.()}
    >
        <Glyph size={14} aria-hidden="true" weight="bold" />
    </button>
{:else if status !== 'idle'}
    <span class="sync-ring" data-state={status} role="img" aria-label={label}>
        <Glyph size={14} aria-hidden="true" weight="bold" />
    </span>
{/if}

<style>
    .sync-ring {
        display: inline-grid;
        place-items: center;
        width: 16px;
        height: 16px;
        padding: 0;
        border: 0;
        background: none;
        color: var(--color-text-muted);
    }

    .sync-ring[data-state='idle'] { opacity: 0.5; }
    /* Accent marks work in progress, matching the row badge's rule exactly, so
       the two surfaces speak one colour language. */
    .sync-ring[data-state='active'] { color: var(--color-accent); }
    .sync-ring[data-state='attention'],
    .sync-ring[data-state='failed'] { color: var(--color-danger); }

    .sync-ring.is-tappable {
        /* Grows to a real target without moving the mark or the title beside
           it: the margins give back exactly the 28px the box gained. It grows
           away from the leading edge rather than around it, so the target never
           lands on the drive name's last few pixels and steals its tap. */
        width: 44px;
        height: 44px;
        margin: -14px -20px -14px -8px;
        cursor: pointer;
    }

    .sync-ring.is-tappable:focus-visible {
        outline: 2px solid var(--color-accent);
        outline-offset: -12px;
        border-radius: var(--radius-md);
    }

    .sync-ring[data-state='active'] :global(svg) {
        transform-origin: center;
        animation: sync-ring-spin 1.2s linear infinite;
    }

    @keyframes sync-ring-spin {
        to { transform: rotate(360deg); }
    }

    @media (prefers-reduced-motion: reduce) {
        /* Loops stop under reduced motion; the loader stays as a static mark,
           and the colour still says that work is happening. */
        .sync-ring[data-state='active'] :global(svg) { animation: none; }
    }
</style>
