<script lang="ts">
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import ImageIcon from 'phosphor-svelte/lib/ImageSquareIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import ArrowRightIcon from 'phosphor-svelte/lib/ArrowRightIcon';
    import ModalShell from '../modals/modal_shell.svelte';
    import { apply_card, close_preview, plan_file_count, set_file_excluded } from './ai_organize';
    import { organize_preview } from './ai_store';
    import { drive_root_name, sidebar_drives } from '../chrome/drive_root_name';
    import { sidebarState } from '../sidebar/sidebar_store';
    import { t_store } from '../../i18n/i18n';

    const preview = $derived($organize_preview);
    // The plan names the drive it was made for, which need not be the open one.
    const plan_drive = $derived(
        drive_root_name(sidebar_drives($sidebarState).find((drive) => drive.id === preview?.plan.scope.channel_id) ?? null, $t_store),
    );
    const total = $derived(preview ? plan_file_count(preview.plan) : 0);
    const excluded_count = $derived(preview ? preview.excluded.length : 0);
    const to_apply = $derived(total - excluded_count);
    const new_folders = $derived(preview ? (preview.plan.folders ?? []).filter((folder) => folder.is_new).length : 0);

    function is_excluded(file_id: number): boolean {
        return preview ? preview.excluded.includes(file_id) : false;
    }

    function toggle_file(file_id: number, checked: boolean): void {
        set_file_excluded(file_id, !checked);
    }

    function apply_now(): void {
        if (!preview || to_apply <= 0) return;
        const card_id = preview.card_id;
        const excluded = [...preview.excluded];
        close_preview();
        void apply_card(card_id, excluded);
    }
</script>

<ModalShell
    hostId="ai-organize-modal"
    open={preview !== null}
    titleId="ai-organize-title"
    title={$t_store('ai_organize.preview_title')}
    subtitle={preview ? $t_store('ai_organize.preview_subtitle', { instruction: preview.instruction }) : ''}
    cardClass="ai-organize-card"
    actionsClass="ai-organize-actions"
    presentation="dialog"
    onClose={close_preview}
>
    {#snippet header()}
        <div class="ai-dialog-head">
            <div class="ai-dialog-heading">
                <h3 id="ai-organize-title" class="ai-dialog-title">{$t_store('ai_organize.preview_title')}</h3>
                <p class="ai-dialog-subtitle">{preview ? $t_store('ai_organize.preview_subtitle', { instruction: preview.instruction }) : ''}</p>
            </div>
            <button type="button" class="ai-dialog-close" aria-label={$t_store('ai_organize.cancel')} title={$t_store('ai_organize.cancel')} onclick={close_preview}>
                <XIcon size={16} aria-hidden="true" />
            </button>
        </div>
    {/snippet}

        {#if preview}
            <div class="ai-summary" role="status">
                <span class="ai-summary-strong">{$t_store('ai_organize.summary_photos', { count: total })}</span>
                <span class="ai-dot" aria-hidden="true">·</span>
                <span>{$t_store('ai_organize.summary_new_folders', { count: new_folders })}</span>
                <span class="ai-dot" aria-hidden="true">·</span>
                <span class="ai-safe">{$t_store('ai_organize.summary_deletes', { count: 0 })}</span>
                <span class="ai-spacer"></span>
                {#if excluded_count > 0}
                    <span class="ai-summary-muted">{$t_store('ai_organize.summary_excluded', { count: excluded_count })}</span>
                {/if}
            </div>

            <div class="ai-groups">
                {#each preview.plan.folders ?? [] as folder (folder.name)}
                    <section class="ai-group" aria-label={folder.name}>
                        <header class="ai-group-head">
                            <span class="ai-group-tile" aria-hidden="true"><FolderIcon size={20} weight="fill" /></span>
                            <span class="ai-group-name">{folder.name}</span>
                            <span class="ai-group-count">{$t_store('ai_organize.files_count', { count: folder.files?.length ?? 0 })}</span>
                            {#if folder.is_new}
                                <span class="ai-tag">{$t_store('ai_organize.new_badge')}</span>
                            {/if}
                            <span class="ai-spacer"></span>
                            <span class="ai-group-path">{plan_drive} / {folder.name}</span>
                        </header>
                        {#each folder.files ?? [] as file (file.file_id)}
                            {@const off = is_excluded(file.file_id)}
                            <div class="ai-row" class:is-off={off}>
                                <input
                                    type="checkbox"
                                    class="ai-check"
                                    checked={!off}
                                    aria-label={$t_store('ai_organize.include_file', { name: file.name })}
                                    onchange={(event) => toggle_file(file.file_id, event.currentTarget.checked)}
                                />
                                <span class="ai-row-tile" aria-hidden="true"><ImageIcon size={16} /></span>
                                <span class="ai-row-main">
                                    <span class="ai-row-name">{file.name}</span>
                                    <span class="ai-row-path">
                                        <span class="ai-row-from">{file.from_path}</span>
                                        <ArrowRightIcon size={12} aria-hidden="true" />
                                        <span class="ai-row-to">{off ? $t_store('ai_organize.stays_put') : folder.name}</span>
                                    </span>
                                </span>
                            </div>
                        {/each}
                    </section>
                {/each}
            </div>
        {/if}

    {#snippet actions()}
        <span class="ai-footnote">{$t_store('ai_organize.safety_note_long')}</span>
        <button type="button" class="ai-btn" onclick={close_preview}>{$t_store('ai_organize.cancel')}</button>
        <button type="button" class="ai-btn is-accent" disabled={to_apply <= 0} onclick={apply_now}>
            {$t_store('ai_organize.apply_count', { count: to_apply })}
        </button>
    {/snippet}
</ModalShell>

<style>
    :global(.ai-organize-card) {
        width: min(960px, calc(100vw - 48px));
        max-width: 960px;
        height: min(700px, calc(100vh - 48px));
        display: flex;
        flex-direction: column;
        gap: 0;
        padding: 0;
        overflow: hidden;
    }

    :global(.ai-organize-actions) {
        flex: none;
        display: flex;
        align-items: center;
        gap: var(--space-2);
        margin: 0;
        padding: var(--space-3) var(--space-6);
        border-top: 1px solid var(--color-border-soft);
    }

    .ai-dialog-head {
        flex: none;
        display: flex;
        align-items: flex-start;
        gap: var(--space-3);
        padding: var(--space-5) var(--space-5) var(--space-3) var(--space-6);
    }

    .ai-dialog-heading {
        flex: 1;
        min-width: 0;
    }

    .ai-dialog-title {
        margin: 0;
        font-size: var(--type-xl);
        font-weight: var(--weight-semibold);
        letter-spacing: -0.01em;
        line-height: var(--leading-tight);
        color: var(--color-text);
    }

    .ai-dialog-subtitle {
        margin: 2px 0 0;
        font-size: var(--type-sm);
        line-height: 18px;
        color: var(--text-muted);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .ai-dialog-close {
        width: 28px;
        height: 28px;
        flex: none;
        display: grid;
        place-items: center;
        padding: 0;
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
    }

    .ai-dialog-close:hover {
        background: var(--color-surface-3);
        color: var(--text-main);
    }

    .ai-dialog-close:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    .ai-summary {
        flex: none;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-2-5) var(--space-6);
        background: var(--color-surface-1);
        border-top: 1px solid var(--color-border-soft);
        border-bottom: 1px solid var(--color-border-soft);
        font-size: var(--type-base);
        white-space: nowrap;
        overflow: hidden;
    }

    .ai-summary-strong {
        font-weight: var(--weight-semibold);
    }

    .ai-summary-muted,
    .ai-dot {
        color: var(--text-muted);
    }

    .ai-safe {
        color: var(--success);
        font-weight: var(--weight-medium);
    }

    .ai-spacer {
        flex: 1;
        min-width: 0;
    }

    .ai-groups {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
    }

    .ai-group-head {
        height: 40px;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 0 var(--space-4);
        background: var(--color-surface-0);
        border-bottom: 1px solid var(--color-border-soft);
        white-space: nowrap;
    }

    /* A bare folder glyph, no tile: same calm folder colour as the file list. */
    .ai-group-tile {
        width: 26px;
        height: 26px;
        display: grid;
        place-items: center;
        color: var(--folder-glyph);
        flex: none;
    }

    .ai-group-tile :global(svg) { width: 20px; height: 20px; }

    .ai-group-name {
        font-size: var(--type-base);
        font-weight: var(--weight-semibold);
    }

    .ai-group-count,
    .ai-group-path {
        font-size: var(--type-xs);
        color: var(--text-muted);
    }

    .ai-tag {
        display: inline-flex;
        align-items: center;
        height: 20px;
        padding: 0 6px;
        border-radius: var(--radius-sm);
        background: color-mix(in srgb, var(--color-accent) 22%, transparent);
        font-size: var(--type-2xs);
        font-weight: var(--weight-semibold);
    }

    .ai-row {
        min-height: 44px;
        display: grid;
        grid-template-columns: 16px 32px minmax(0, 1fr);
        align-items: center;
        gap: var(--space-3);
        padding: 0 var(--space-4) 0 var(--space-6);
        border-bottom: 1px solid var(--color-border-soft);
    }

    .ai-row.is-off {
        opacity: 0.55;
    }

    .ai-check {
        width: 16px;
        height: 16px;
        margin: 0;
        accent-color: var(--color-accent);
        cursor: pointer;
    }

    .ai-row-tile {
        width: 32px;
        height: 32px;
        display: grid;
        place-items: center;
        border-radius: var(--radius-sm);
        background: var(--color-surface-2);
        color: var(--text-muted);
    }

    .ai-row-main {
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }

    .ai-row-name {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-size: var(--type-base);
        font-weight: var(--weight-medium);
    }

    .ai-row-path {
        display: flex;
        align-items: center;
        gap: 6px;
        min-width: 0;
        font-size: var(--type-xs);
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
    }

    .ai-row-from,
    .ai-row-to {
        overflow: hidden;
        text-overflow: ellipsis;
        min-width: 0;
    }

    .ai-row-to {
        color: var(--text-main);
    }

    .ai-footnote {
        flex: 1;
        min-width: 0;
        font-size: var(--type-sm);
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .ai-btn {
        height: 32px;
        padding: 0 var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        background: var(--color-surface-2);
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-base);
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
    .ai-check:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }
</style>
