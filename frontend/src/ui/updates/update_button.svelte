<script lang="ts">
    import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import { updateState } from './update_store';
    import { openUpdatesUI } from '../../modules/updates';
    import { t_store } from '../../i18n/i18n';
    const pending = $derived(['available', 'ready'].includes($updateState.phase));
    const busy = $derived(['checking', 'downloading'].includes($updateState.phase));
</script>

<button type="button" class="update-button" class:pending class:busy aria-label={$t_store('settings_updates.check_button')}
    title={$t_store($updateState.phase === 'ready' ? 'misc.ready_title' : 'settings_updates.check_button')}
    onclick={() => void openUpdatesUI()}>
    <ArrowsClockwiseIcon size={16} aria-hidden="true" />
    {#if pending}<span class="dot" aria-hidden="true"></span>{/if}
</button>

<style>
    .update-button { position: relative; width: 32px; height: 32px; display: inline-grid; place-items: center; padding: 0; flex: none;
        border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--text-muted); background: transparent; cursor: pointer; }
    .update-button:hover, .pending { color: var(--color-accent); background: var(--overlay-accent-1); }
    .dot { position: absolute; top: 3px; right: 3px; width: 6px; height: 6px; border-radius: 50%; background: var(--color-accent); }
    .busy :global(svg) { animation: spin 1.5s linear infinite; }
    @keyframes spin { to { transform: rotate(360deg); } }
    @media (prefers-reduced-motion: reduce) { .busy :global(svg) { animation: none; } }
</style>
