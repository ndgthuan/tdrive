<script lang="ts">
    import SparkleIcon from 'phosphor-svelte/lib/SparkleIcon';
    import { panel_open } from './ai_store';
    import { t_store } from '../../i18n/i18n';

    function toggle(): void {
        panel_open.update((open) => !open);
    }
</script>

<button
    type="button"
    class="ai-toggle"
    class:is-on={$panel_open}
    aria-label={$t_store('ai.button_label')}
    aria-pressed={$panel_open ? 'true' : 'false'}
    aria-controls="ai-panel"
    title={$t_store('ai.button_title')}
    onclick={toggle}
>
    <SparkleIcon size={16} aria-hidden="true" />
</button>

<style>
    .ai-toggle {
        width: 32px;
        height: 32px;
        display: inline-grid;
        place-items: center;
        flex: none;
        padding: 0;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
    }

    .ai-toggle:hover {
        background: var(--color-surface-2);
        color: var(--text-main);
    }

    .ai-toggle.is-on {
        background: color-mix(in srgb, var(--color-accent) 18%, transparent);
        border-color: transparent;
        color: var(--color-accent);
    }

    .ai-toggle:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring, 0 0 0 2px var(--color-accent));
    }
</style>
