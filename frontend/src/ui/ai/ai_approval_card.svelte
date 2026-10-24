<script lang="ts">
    import FileIcon from 'phosphor-svelte/lib/FileIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import { confirm_label_key, resolve_approval } from './ai_actions';
    import type { ApprovalRequest } from './ai_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        request: ApprovalRequest;
    }

    let { request }: Props = $props();

    const MAX_ITEMS = 6;
    const shown = $derived(request.items.slice(0, MAX_ITEMS));
    const hidden_count = $derived(Math.max(0, request.items.length - MAX_ITEMS));
    const is_delete = $derived(/delete|trash|remove/i.test(request.tool));
    // Emptying the trash destroys items for good; the card says so in the user's language.
    const is_permanent = $derived(/empty_trash/i.test(request.tool));
    const can_answer = $derived(request.state === 'pending');
    const outcome = $derived(
        request.state === 'approved' ? $t_store('ai.approval_approved')
            : request.state === 'denied' ? $t_store('ai.approval_denied')
                : request.state === 'expired' ? $t_store('ai.approval_expired')
                    : '',
    );
</script>

{#if request.state === 'approved' || request.state === 'denied' || request.state === 'expired'}
    <!-- Once answered, the card collapses to the outcome row. -->
    <div class="ai-approval-outcome" role="status">
        <span class="ai-approval-outcome-title">{request.summary}</span>
        <span class="ai-approval-outcome-state">{outcome}</span>
    </div>
{:else}
    <section class="ai-approval" aria-label={$t_store('ai.approval_label')}>
        <p class="ai-approval-title">{request.summary}</p>
        {#if is_permanent}
            <p class="ai-approval-permanent" role="note">{$t_store('ai.approval_permanent')}</p>
        {/if}
        <ul class="ai-approval-items">
            {#each shown as item (item.id)}
                <li class="ai-approval-item">
                    <span class="ai-approval-item-icon" aria-hidden="true">
                        {#if item.kind === 'folder'}
                            <FolderIcon size={14} />
                        {:else}
                            <FileIcon size={14} />
                        {/if}
                    </span>
                    <span class="ai-approval-item-name">{item.name}</span>
                </li>
            {/each}
            {#if hidden_count > 0}
                <li class="ai-approval-more">{$t_store('ai.approval_more', { count: hidden_count })}</li>
            {/if}
        </ul>
        <div class="ai-approval-actions">
            <button
                type="button"
                class="ai-approval-btn is-neutral"
                disabled={!can_answer}
                onclick={() => resolve_approval(request.request_id, false)}
            >
                {$t_store('ai.approval_cancel')}
            </button>
            <button
                type="button"
                class="ai-approval-btn"
                class:is-danger={is_delete}
                class:is-accent={!is_delete}
                disabled={!can_answer}
                onclick={() => resolve_approval(request.request_id, true)}
            >
                {$t_store(confirm_label_key(request.tool))}
            </button>
        </div>
    </section>
{/if}

<style>
    .ai-approval {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        padding: var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-2);
    }

    .ai-approval-title {
        margin: 0;
        font-weight: var(--weight-semibold);
        line-height: var(--leading-snug);
    }

    .ai-approval-permanent {
        margin: 0;
        color: var(--color-danger);
        font-size: var(--type-sm);
        line-height: var(--leading-snug);
    }

    .ai-approval-items {
        margin: 0;
        padding: 0;
        list-style: none;
        display: flex;
        flex-direction: column;
        gap: var(--space-1);
        font-size: var(--type-sm);
    }

    .ai-approval-item {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-width: 0;
    }

    .ai-approval-item-icon {
        display: inline-flex;
        flex: none;
        color: var(--text-muted);
    }

    .ai-approval-item-name {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .ai-approval-more {
        color: var(--text-muted);
        padding-left: calc(14px + var(--space-2));
    }

    .ai-approval-actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--space-2);
    }

    .ai-approval-btn {
        height: 30px;
        padding: 0 var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        background: var(--color-surface-2);
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-sm);
        font-weight: var(--weight-medium);
        cursor: pointer;
        white-space: nowrap;
    }

    .ai-approval-btn.is-accent {
        border-color: var(--color-accent);
        color: var(--color-accent);
    }

    .ai-approval-btn.is-danger {
        border-color: var(--color-danger);
        color: var(--color-danger);
    }

    .ai-approval-btn:disabled {
        opacity: 0.5;
        cursor: default;
    }

    .ai-approval-btn:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    .ai-approval-outcome {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
        min-height: 26px;
        padding: var(--space-1) var(--space-2);
        border-radius: var(--radius-sm);
        background: var(--color-surface-2);
        font-size: var(--type-sm);
    }

    .ai-approval-outcome-title {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .ai-approval-outcome-state {
        flex: none;
        color: var(--text-muted);
        white-space: nowrap;
    }
</style>
