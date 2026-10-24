<script lang="ts">
    import ArrowCounterClockwiseIcon from 'phosphor-svelte/lib/ArrowCounterClockwiseIcon';
    import ArrowsLeftRightIcon from 'phosphor-svelte/lib/ArrowsLeftRightIcon';
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import FolderPlusIcon from 'phosphor-svelte/lib/FolderPlusIcon';
    import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlassIcon';
    import PencilSimpleIcon from 'phosphor-svelte/lib/PencilSimpleIcon';
    import TrashIcon from 'phosphor-svelte/lib/TrashIcon';
    import { icon_for_tool, undo_row } from './ai_actions';
    import type { ActionRow } from './ai_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        rows: ActionRow[];
    }

    let { rows }: Props = $props();
</script>

<ol class="ai-timeline" aria-label={$t_store('ai.action_timeline_label')}>
    {#each rows as row (row.id)}
        <li class="ai-action" class:is-error={!row.ok}>
            <span class="ai-action-icon" aria-hidden="true">
                {#if icon_for_tool(row.tool) === 'rename'}
                    <PencilSimpleIcon size={14} />
                {:else if icon_for_tool(row.tool) === 'move'}
                    <ArrowsLeftRightIcon size={14} />
                {:else if icon_for_tool(row.tool) === 'create'}
                    <FolderPlusIcon size={14} />
                {:else if icon_for_tool(row.tool) === 'restore'}
                    <ArrowCounterClockwiseIcon size={14} />
                {:else if icon_for_tool(row.tool) === 'delete'}
                    <TrashIcon size={14} />
                {:else}
                    <MagnifyingGlassIcon size={14} />
                {/if}
            </span>
            <span class="ai-action-text">
                <span class="ai-action-summary">{row.summary}</span>
                {#if !row.ok}
                    <span class="ai-action-error" role="alert">{row.error || $t_store('ai.action_failed')}</span>
                {/if}
            </span>
            {#if row.ok && !row.undone}
                <span class="ai-action-status" aria-label={$t_store('ai.action_done')}>
                    <CheckIcon size={12} aria-hidden="true" />
                </span>
            {/if}
            {#if row.undo_token && !row.undone}
                <button
                    type="button"
                    class="ai-action-undo"
                    disabled={row.undoing}
                    onclick={() => undo_row(row.id)}
                >
                    {$t_store('ai.action_undo')}
                </button>
            {:else if row.undone}
                <span class="ai-action-undone">{$t_store('ai.action_undone')}</span>
            {/if}
        </li>
    {/each}
</ol>

<style>
    .ai-timeline {
        margin: 0;
        padding: 0;
        list-style: none;
        display: flex;
        flex-direction: column;
        gap: var(--space-1);
        font-size: var(--type-sm);
    }

    .ai-action {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-height: 26px;
        padding: var(--space-1) var(--space-2);
        border-radius: var(--radius-sm);
        background: var(--color-surface-2);
        line-height: var(--leading-snug);
    }

    .ai-action.is-error {
        background: color-mix(in srgb, var(--color-danger) 8%, transparent);
    }

    .ai-action-icon {
        display: inline-flex;
        flex: none;
        color: var(--text-muted);
    }

    .ai-action-text {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
    }

    .ai-action-summary {
        word-break: break-word;
    }

    .ai-action-error {
        color: var(--color-danger);
        word-break: break-word;
    }

    .ai-action-status {
        display: inline-flex;
        flex: none;
        color: var(--color-success, var(--text-muted));
    }

    .ai-action-undo {
        height: 22px;
        padding: 0 var(--space-2);
        flex: none;
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--color-accent);
        font: inherit;
        font-weight: var(--weight-medium);
        cursor: pointer;
        white-space: nowrap;
    }

    .ai-action-undo:disabled {
        opacity: 0.5;
        cursor: default;
    }

    .ai-action-undo:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    .ai-action-undone {
        flex: none;
        color: var(--text-muted);
        white-space: nowrap;
    }
</style>
