<script lang="ts">
    import FolderInputIcon from 'phosphor-svelte/lib/FolderOpenIcon';
    import DownloadIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import Trash2Icon from 'phosphor-svelte/lib/TrashIcon';
    import { t_store } from '../../i18n/i18n';
    import { selectionBarState } from './selection_bar_store';

    interface SelectionBarProps {
        onDownload: () => void;
        onMove: () => void;
        onDelete: () => void;
        /** Done on the desktop, Clear on the phone: both leave selection mode. */
        onClear: () => void;
    }

    let { onDownload, onMove, onDelete, onClear }: SelectionBarProps = $props();

    const label = $derived($t_store('selection.count', { count: $selectionBarState.count }));
</script>

<!-- Desktop: a floating bar of text actions, Download, Move, Delete and Done.
     The count sits in the list header there, so the bar leaves it out.
     Mobile: the host becomes a bottom action bar; the count and Done move to
     the top region owned by the shell (which reads selectionBarState.count and
     calls clearSelection), so the count and Clear are hidden here and the bulk
     actions show as labelled icons. -->
<div id="selection-count" class="selection-count" aria-atomic="true">{label}</div>
<div class="selection-actions">
    <button id="selection-download" class="selection-btn" type="button" onclick={onDownload}>
        <span class="selection-btn-icon" aria-hidden="true">
            <DownloadIcon size={22} />
        </span>
        <span class="selection-btn-label">{$t_store('selection.download')}</span>
    </button>
    <button id="selection-move" class="selection-btn" type="button" onclick={onMove}>
        <span class="selection-btn-icon" aria-hidden="true">
            <FolderInputIcon size={22} />
        </span>
        <span class="selection-btn-label">{$t_store('selection.move')}</span>
    </button>
    <button id="selection-delete" class="selection-btn danger" type="button" onclick={onDelete}>
        <span class="selection-btn-icon" aria-hidden="true">
            <Trash2Icon size={22} />
        </span>
        <span class="selection-btn-label">{$t_store('selection.delete')}</span>
    </button>
    <span class="selection-spacer" aria-hidden="true"></span>
    <button id="selection-clear" class="selection-btn done" type="button" onclick={onClear}>{$t_store('selection.done')}</button>
</div>

<style>
    /* The action glyphs are mobile-only; desktop keeps text buttons. */
    .selection-btn-icon { display: none; }
    .selection-spacer { display: none; }

    /* Desktop: one floating bar at the bottom centre of the content pane (its
       containing block is .main-content, so the sidebar is not counted), 8px
       corners, one shadow and no blur. Its buttons are 32px. */
    :global(html:not(.mobile) .selection-bar) {
        inset: auto auto 24px 50%;
        transform: translateX(-50%);
        z-index: var(--z-selection);
        width: max-content;
        max-width: calc(100% - 48px);
        height: auto;
        min-height: 48px;
        padding: var(--space-2);
        gap: var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-2);
        box-shadow: var(--shadow-md);
        backdrop-filter: none;
        -webkit-backdrop-filter: none;
    }
    :global(html:not(.mobile) .selection-count) { display: none; }
    :global(html:not(.mobile) .selection-actions) {
        display: flex;
        align-items: center;
        gap: var(--space-2);
    }
    :global(html:not(.mobile) .selection-spacer) {
        display: block;
        width: var(--space-4);
    }
    :global(html:not(.mobile) .selection-btn) {
        height: var(--control-md);
        padding: 0 var(--space-3);
        border-radius: var(--radius-sm);
        border: 1px solid transparent;
        background: transparent;
        color: var(--color-text-muted);
        font-size: var(--type-sm);
        font-weight: var(--weight-medium);
        white-space: nowrap;
    }
    :global(html:not(.mobile) .selection-btn:hover) { background: var(--color-surface-3); color: var(--color-text); }
    :global(html:not(.mobile) .selection-btn.danger) { color: var(--color-danger); }
    :global(html:not(.mobile) .selection-btn.danger:hover) { background: var(--color-surface-3); color: var(--color-danger); }
    :global(html:not(.mobile) .selection-btn.done) {
        color: var(--color-text);
        border-color: var(--color-border-soft);
        background: var(--color-surface-1);
    }
    :global(html:not(.mobile) .selection-btn.done:hover) { background: var(--color-surface-3); }

    :global(html.mobile .selection-bar) {
        position: fixed;
        inset: auto 0 0 0;
        height: auto;
        padding: var(--space-2) var(--space-2) calc(var(--space-2) + var(--inset-bottom));
        justify-content: space-around;
        align-items: stretch;
        border-top: 1px solid var(--color-border-soft);
        border-bottom: none;
        background: var(--color-surface-0);
        -webkit-backdrop-filter: none;
        backdrop-filter: none;
    }

    /* The shell renders the count and Done in the top region on a phone. */
    :global(html.mobile) .selection-count { display: none; }
    :global(html.mobile) .selection-actions {
        flex: 1;
        justify-content: space-around;
        gap: var(--space-1);
    }
    :global(html.mobile) .selection-btn.done { display: none; }

    :global(html.mobile) .selection-btn {
        flex-direction: column;
        gap: 3px;
        min-width: 64px;
        min-height: 48px;
        height: auto;
        padding: var(--space-1) var(--space-3);
        border: none;
        background: transparent;
        border-radius: var(--radius-md);
        font-size: var(--type-xs);
        font-weight: 600;
    }
    :global(html.mobile) .selection-btn:active { background: var(--overlay-neutral-2); transform: none; }
    :global(html.mobile) .selection-btn.danger { border: none; color: var(--color-danger); }
    :global(html.mobile) .selection-btn.danger:active { background: var(--overlay-danger-1); }
    :global(html.mobile) .selection-btn-icon {
        display: flex;
        align-items: center;
        justify-content: center;
    }
</style>
