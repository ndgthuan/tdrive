<script lang="ts">
    import ArrowClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import ImageIcon from 'phosphor-svelte/lib/ImageSquareIcon';
    import WarningCircleIcon from 'phosphor-svelte/lib/WarningCircleIcon';
    import { apply_card, open_preview, retry_plan, undo_card, plan_file_count } from './ai_organize';
    import type { PlanCard } from './ai_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        id: string;
        card: PlanCard;
    }

    let { id, card }: Props = $props();

    const planned = $derived(card.plan ? plan_file_count(card.plan) : 0);
    const folders = $derived(card.plan?.folders ?? []);
    const progress_percent = $derived(card.total > 0 ? Math.round((card.analyzed / card.total) * 100) : 0);
    const busy = $derived(card.status === 'planning' || card.status === 'applying');
</script>

<section class="ai-plan" aria-label={$t_store('ai_organize.plan_label')}>
    {#if card.status === 'planning'}
        <div class="ai-plan-head">
            <span class="ai-plan-title">
                {card.total > 0
                    ? $t_store('ai_organize.analyzing', { analyzed: card.analyzed, total: card.total })
                    : $t_store('ai_organize.analyzing_start')}
            </span>
            {#if card.total > 0}
                <span class="ai-plan-count">{card.analyzed}/{card.total}</span>
            {/if}
        </div>
        <div class="ai-progress" class:is-indeterminate={card.total === 0}>
            <i style={card.total > 0 ? `width: ${progress_percent}%` : ''}></i>
        </div>
    {:else if card.status === 'failed'}
        <div class="ai-plan-error" role="alert">
            <WarningCircleIcon size={16} aria-hidden="true" />
            <span class="ai-plan-error-text">
                {#if card.error === 'no_model'}
                    {$t_store('ai.error_no_model')}
                {:else if card.error === 'invalid_key'}
                    {$t_store('ai.error_invalid_key')}
                {:else if card.error === 'rate_limit'}
                    {$t_store('ai.error_rate_limit')}
                {:else}
                    {$t_store('ai_organize.plan_failed')}
                {/if}
            </span>
            <button type="button" class="ai-plan-link" onclick={() => retry_plan(id)}>
                <ArrowClockwiseIcon size={14} aria-hidden="true" />
                {$t_store('ai.retry')}
            </button>
        </div>
    {:else if card.status === 'undone'}
        <p class="ai-plan-note">{$t_store('ai_organize.undone')}</p>
    {:else if card.status === 'applied'}
        <div class="ai-plan-head">
            <span class="ai-plan-title">{$t_store('ai_organize.moved_done', { count: card.moved })}</span>
        </div>
        <div class="ai-plan-actions">
            <button type="button" class="ai-btn" onclick={() => undo_card(id)}>{$t_store('ai_organize.undo')}</button>
        </div>
    {:else if card.plan && folders.length === 0}
        <p class="ai-plan-note">{$t_store('ai_organize.nothing_to_move')}</p>
    {:else if card.plan}
        <div class="ai-plan-head">
            <span class="ai-plan-title">{$t_store('ai_organize.analyzed_all', { count: card.plan.total })}</span>
        </div>
        <div class="ai-progress"><i style="width: 100%"></i></div>

        <div class="ai-cap">{$t_store('ai_organize.proposed_folders')}</div>
        <ul class="ai-folders">
            {#each folders as folder (folder.name)}
                <li class="ai-folder">
                    <span class="ai-folder-tile" aria-hidden="true"><FolderIcon size={16} weight="fill" /></span>
                    <span class="ai-folder-text">
                        <span class="ai-folder-name">
                            {folder.name}
                            {#if folder.is_new}
                                <span class="ai-tag">{$t_store('ai_organize.new_badge')}</span>
                            {/if}
                        </span>
                        <span class="ai-folder-count">{$t_store('ai_organize.files_count', { count: folder.files?.length ?? 0 })}</span>
                    </span>
                    <span class="ai-thumbs" aria-hidden="true">
                        <span class="ai-thumb"><ImageIcon size={12} /></span>
                        <span class="ai-thumb"><ImageIcon size={12} /></span>
                        <span class="ai-thumb"><ImageIcon size={12} /></span>
                    </span>
                </li>
            {/each}
        </ul>

        {#if card.plan.skipped > 0}
            <p class="ai-plan-note">{$t_store('ai_organize.skipped', { count: card.plan.skipped })}</p>
        {/if}

        <div class="ai-plan-actions">
            <button type="button" class="ai-btn" disabled={busy} onclick={() => open_preview(id)}>
                {$t_store('ai_organize.preview_changes')}
            </button>
            <button type="button" class="ai-btn is-accent" disabled={busy || planned === 0} onclick={() => apply_card(id, [])}>
                {$t_store('ai_organize.apply')}
            </button>
        </div>
        <p class="ai-plan-note">{$t_store('ai_organize.safety_note')}</p>
    {:else}
        <p class="ai-plan-note">{$t_store('ai_organize.applying')}</p>
    {/if}
</section>

<style>
    .ai-plan {
        display: flex;
        flex-direction: column;
        gap: var(--space-2-5);
        padding: var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-2);
    }

    .ai-plan-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
    }

    .ai-plan-title {
        font-weight: var(--weight-semibold);
        line-height: var(--leading-snug);
    }

    .ai-plan-count,
    .ai-plan-note,
    .ai-folder-count {
        font-size: var(--type-xs);
        color: var(--text-muted);
        line-height: 16px;
    }

    .ai-plan-note {
        margin: 0;
    }

    .ai-progress {
        height: 4px;
        border-radius: 2px;
        background: var(--color-border-soft);
        overflow: hidden;
    }

    .ai-progress i {
        display: block;
        height: 4px;
        background: var(--color-accent);
        transition: width var(--motion-med) var(--ease-standard);
    }

    .ai-progress.is-indeterminate i {
        width: 35%;
        animation: ai-slide 1.2s infinite ease-in-out;
    }

    @keyframes ai-slide {
        from { transform: translateX(-100%); }
        to { transform: translateX(300%); }
    }

    @media (prefers-reduced-motion: reduce) {
        .ai-progress.is-indeterminate i {
            animation: none;
        }
    }

    .ai-cap {
        font-size: var(--type-xs);
        letter-spacing: 0.06em;
        text-transform: uppercase;
        color: var(--text-muted);
        font-weight: var(--weight-semibold);
    }

    .ai-folders {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-1);
    }

    .ai-folder {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 40px;
    }

    .ai-folder-tile {
        width: 28px;
        height: 28px;
        flex: none;
        display: grid;
        place-items: center;
        border-radius: var(--radius-sm);
        background: color-mix(in srgb, var(--color-accent) 14%, transparent);
        color: var(--color-accent);
    }

    .ai-folder-text {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
    }

    .ai-folder-name {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-weight: var(--weight-medium);
    }

    .ai-tag {
        display: inline-flex;
        align-items: center;
        height: 18px;
        margin-left: 6px;
        padding: 0 6px;
        border-radius: var(--radius-sm);
        background: color-mix(in srgb, var(--color-accent) 22%, transparent);
        color: var(--text-main);
        font-size: var(--type-2xs);
        font-weight: var(--weight-semibold);
        vertical-align: middle;
    }

    .ai-thumbs {
        display: flex;
        gap: 4px;
    }

    .ai-thumb {
        width: 22px;
        height: 22px;
        display: grid;
        place-items: center;
        border-radius: 4px;
        background: var(--color-surface-3);
        color: var(--text-muted);
    }

    .ai-plan-actions {
        display: flex;
        gap: var(--space-2);
        margin-top: var(--space-1);
    }

    .ai-btn {
        height: 30px;
        padding: 0 var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        background: var(--color-surface-1);
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-sm);
        font-weight: var(--weight-medium);
        cursor: pointer;
        white-space: nowrap;
    }

    .ai-btn.is-accent {
        background: var(--color-accent);
        border-color: var(--color-accent);
        color: var(--color-on-accent);
        font-weight: var(--weight-semibold);
    }

    .ai-btn:disabled {
        opacity: 0.5;
        cursor: default;
    }

    .ai-btn:focus-visible,
    .ai-plan-link:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    .ai-plan-error {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        font-size: var(--type-sm);
        color: var(--text-muted);
    }

    .ai-plan-error :global(svg:first-child) {
        color: var(--color-danger);
        flex: none;
    }

    .ai-plan-error-text {
        flex: 1;
        min-width: 0;
    }

    .ai-plan-link {
        height: 24px;
        padding: 0 var(--space-2);
        display: inline-flex;
        align-items: center;
        gap: var(--space-1);
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--color-accent);
        font: inherit;
        font-weight: var(--weight-medium);
        cursor: pointer;
        white-space: nowrap;
    }
</style>
