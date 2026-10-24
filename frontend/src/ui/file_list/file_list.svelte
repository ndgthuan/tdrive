<script lang="ts">
    import { onMount } from 'svelte';
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import DownloadIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import EllipsisIcon from 'phosphor-svelte/lib/DotsThreeIcon';
    import FolderInputIcon from 'phosphor-svelte/lib/FolderOpenIcon';
    import ExternalLinkIcon from 'phosphor-svelte/lib/ArrowSquareOutIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import LoaderCircleIcon from 'phosphor-svelte/lib/CircleNotchIcon';
    import LockKeyholeIcon from 'phosphor-svelte/lib/LockKeyIcon';
    import PlayIcon from 'phosphor-svelte/lib/PlayIcon';
    import RotateCcwIcon from 'phosphor-svelte/lib/ArrowCounterClockwiseIcon';
    import Trash2Icon from 'phosphor-svelte/lib/TrashIcon';
    import { isMobilePlatform } from '../../api';
    import { t_store } from '../../i18n/i18n';
    import FileState from './file_state.svelte';
    import FileThumbnail from './file_thumbnail.svelte';
    import { minuteTick } from '../file_list/clock';
    import { fileTypeFamily, fileTypeIcon } from './file_type';
    import { rowMetaLine, splitRowLabel } from './row_meta';
    import { rowOffset, rowWindowFor, type RowMetrics } from './row_window';
    import ItemStatus from '../mobile/item_status.svelte';
    import { explainedFileIds, itemStateFor, transfersByFile } from '../mobile/item_state_store';
    import { busyRowIds } from './busy_rows';
    import { itemStateDescriptor } from '../mobile/item_state';
    import { activeTab } from '../mobile/mobile_shell_store';
    import { fileListView, sortedFileListRows } from './file_list_store';
    import { fileSortState } from './file_sort_store';
    import { activeFileRowKey, selectedFileRowKeys } from './row_state_store';
    import { selectionModeOn } from '../selection/selection_bar_store';
    import { new_folder_draft } from './new_folder_draft_store';
    import { submitFolder } from '../../modules/modals/folder';
    import {
        beginFileThumbnailRender,
        rearmFileThumbnailLocked,
        setFileThumbnailRoot,
        teardownFileThumbnails,
    } from './file_thumbnail_controller';
    import type { FileListAction, FileListFileRow, FileListRow, FileListView, FolderListRow } from '../file_list/types';

    type InteractiveRow = FolderListRow | FileListFileRow;
    // One of the three places in a desktop row's Actions column: an action, the
    // More button, or an empty place that keeps the icons in line.
    type DesktopSlot = FileListAction | 'more' | null;

    // The desktop Actions column always has three places, so Open, Download and
    // More line up from row to row even where a file has no Open. The trash has
    // no menu, so its two icons are the whole column, centred under the label.
    function desktop_action_slots(row: InteractiveRow): DesktopSlot[] {
        if (row.actionsInline) return row.actions.slice(0, 2);
        const open = row.actions.find((action) => action.kind === 'open' || action.kind === 'play') ?? null;
        const download = row.actions.find((action) => action.kind === 'download') ?? null;
        return [open, download, 'more'];
    }

    // The inline folder row names a new folder inside the list: the field starts
    // with the name selected, Enter creates, Escape and leaving the field cancel.
    function focus_select(node: HTMLInputElement): void {
        node.focus();
        node.select();
    }

    function on_new_folder_input(event: Event): void {
        new_folder_draft.set((event.currentTarget as HTMLInputElement).value);
    }

    function cancel_new_folder(): void {
        new_folder_draft.set(null);
    }

    function on_new_folder_keydown(event: KeyboardEvent): void {
        // Keys typed into the name belong to the field, not to the list's arrows
        // or to the selection's Escape.
        event.stopPropagation();
        if (event.key === 'Escape') {
            event.preventDefault();
            cancel_new_folder();
            return;
        }
        if (event.key !== 'Enter') return;
        event.preventDefault();
        const name = (event.currentTarget as HTMLInputElement).value.trim();
        cancel_new_folder();
        if (name) void submitFolder(name);
    }

    // The phone row is a two-line list item rather than a table row; the
    // desktop grid below is untouched.
    const mobile = isMobilePlatform();

    // A row carries three attributes, and each one has to earn its place: a
    // long folder is thousands of rows, and anything written here is written
    // that many times.
    //
    // `data-row-key` is how a handler holding nothing but an element gets the
    // row back (see row_lookup.ts). `data-type` is what the delegated
    // listeners and the touch binders match on to tell a folder from a file
    // from a folder still being created. `data-name` is there for the
    // end-to-end tests, which assert on sort order and have no other way to
    // read a row's name out of the two different layouts below.
    //
    // Everything else about a row -- its id, size, source, uploader, whether
    // it can be renamed -- lives in the store and is looked up from there.
    // Serialising those as well gave every row a second, untyped copy of
    // itself that was free to disagree with the first.
    function dataType(row: FileListRow) {
        return row.kind === 'pending-folder' ? 'pending-folder' : row.kind;
    }

    function onActionClick(event: MouseEvent, row: FileListRow, action: FileListAction) {
        if (!action.onClick) return;
        event.stopPropagation();
        action.onClick(event, row);
    }

    function onRowClick(event: MouseEvent, row: InteractiveRow) {
        row.onClick?.(event, row);
    }

    function onRowDoubleClick(event: MouseEvent, row: InteractiveRow) {
        row.onDoubleClick?.(event, row);
    }


    // Keyboard commands are owned by the single delegated listener on
    // #file-list. This handler keeps that bubbling contract explicit to Svelte.
    function onGridRowKeydown(_event: KeyboardEvent): void {}

    // Seeds the spacers for the frames before measureRow reports a real row.
    // A wrong seed is visible: the list scrolls to a stale offset and jumps
    // once the measurement lands. Desktop is the 40px chip plus the row's 12px
    // padding either side plus its 2px gap; the phone row is its own
    // min-height, which is taller than the 44px chip it holds.
    const ESTIMATED_ROW_HEIGHT = mobile ? 68 : 66;
    const WINDOW_OVERSCAN = 8;
    let scrollTop = $state(0);
    let viewportHeight = $state(0);
    let rowHeight = $state(ESTIMATED_ROW_HEIGHT);
    // A row that has to explain itself is taller. Measured on its own so one of
    // them cannot be taken for the standard height, which would move every
    // spacer in the list by the difference.
    let explainRowHeight = $state(ESTIMATED_ROW_HEIGHT);
    let list: HTMLElement | null = null;

    const visibleRows = $derived(sortedFileListRows($fileListView, $fileSortState));
    const thumbnailChannelId = $derived(
        visibleRows.find((row): row is FileListFileRow => row.kind === 'file' && row.thumbnail !== undefined)
            ?.thumbnail?.channelId ?? 0,
    );
    $effect(() => beginFileThumbnailRender(thumbnailChannelId));
    // The rows whose second line changes virtual height.
    //
    // The transfer map is rewritten on every progress event, so what this
    // reads decides what a busy upload costs. It names the files, not the
    // rows: the explained ids are bounded by the capped notification history
    // while a folder is not bounded at all, and an unchanged answer is
    // republished by identity so nothing below re-runs (see explainedFileIds).
    //
    // A plain Set, not a SvelteSet: it is built whole and never mutated after,
    // so per-entry reactive sources would be overhead paid once per file for a
    // signal nothing subscribes to.
    const NO_EXPLAINED_IDS: ReadonlySet<string> = new Set<string>();
    const NO_TALL_INDICES: readonly number[] = [];
    const explainedIds = $derived.by(() => {
        // Read for its identity rather than its rows: the view is what changes
        // when the drive does, and a transfer key only names a row in the
        // drive it was scoped to.
        const scope = $fileListView;
        // Desktop draws no explaining line, and returning first is what keeps
        // it from subscribing to the transfer map at all.
        if (!mobile) return NO_EXPLAINED_IDS;
        return explainedFileIds($transfersByFile, scope);
    });
    const rowMetrics = $derived.by((): RowMetrics => ({
        rowHeight,
        tallRowHeight: explainRowHeight,
        // Over the whole folder, because a spacer's height depends on every
        // tall row above it and not only on the ones on screen. It runs when
        // the explained set changes -- a transfer failing -- not when one
        // progresses, which is the difference between once and sixty times a
        // second.
        tallIndices: explainedIds.size === 0
            ? NO_TALL_INDICES
            : visibleRows.reduce<number[]>((indices, row, index) => {
                if (row.kind !== 'pending-folder' && explainedIds.has(row.id)) indices.push(index);
                return indices;
            }, []),
    }));
    const rowWindow = $derived.by(() => {
        if (visibleRows.length <= 64) {
            return { before: 0, rows: visibleRows, start: 0, after: 0, end: visibleRows.length };
        }
        const window = rowWindowFor(visibleRows.length, scrollTop, viewportHeight, WINDOW_OVERSCAN, rowMetrics);
        return { ...window, rows: visibleRows.slice(window.start, window.end) };
    });
    // Selection mode on the phone: once anything is selected every row shows
    // its check so a tap reads as toggling rather than opening.
    const selecting = $derived(mobile && $selectedFileRowKeys.size > 0);

    // Which rows sit at the ends of the card. Marked by the list rather than
    // left to a CSS sibling selector, which can only see the rendered window:
    // between the spacers, a row in the middle of a long folder looks like a
    // first or last child and rounded its corners while scrolling.
    function cardEdges(rowIndex: number): string {
        const index = rowWindow.start + rowIndex;
        return `${index === 0 ? ' is-card-top' : ''}${index === visibleRows.length - 1 ? ' is-card-bottom' : ''}`;
    }

    function updateViewport(): void {
        if (!list) return;
        scrollTop = list.scrollTop;
        viewportHeight = list.clientHeight;
    }

    // Keep the keyboard in the list when its focused row scrolls out of the
    // virtual window and unmounts. Without this, focus falls to <body>, keydown
    // stops reaching the list's delegated handler, and the arrows go dead. The
    // container (tabindex="-1") catches it; the delegated handler resolves the
    // active row from its stored key, so the arrows pick up where they left off.
    function retainKeyboardFocus(): void {
        if (!list) return;
        const active = document.activeElement;
        if (!(active instanceof HTMLElement) || !active.classList.contains('drive-row') || !list.contains(active)) return;
        const key = active.dataset.rowKey ?? '';
        const index = key ? visibleRows.findIndex((row) => row.kind !== 'pending-folder' && row.selectionKey === key) : -1;
        if (index < 0) return;
        const slack = rowHeight * WINDOW_OVERSCAN;
        if (rowOffset(index + 1, rowMetrics) < scrollTop - slack || rowOffset(index, rowMetrics) > scrollTop + viewportHeight + slack) {
            list.focus({ preventScroll: true });
        }
    }

    function measureRow(element: HTMLElement): { destroy: () => void } {
        const update = () => {
            const style = getComputedStyle(element);
            const footprint = Math.ceil(
                element.getBoundingClientRect().height
                + Number.parseFloat(style.marginTop || '0')
                + Number.parseFloat(style.marginBottom || '0'),
            );
            if (footprint <= 0) return;
            if (element.classList.contains('needs-explanation')) explainRowHeight = footprint;
            else rowHeight = footprint;
        };
        update();
        const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(update);
        observer?.observe(element);
        return { destroy: () => observer?.disconnect() };
    }

    function revealRow(event: Event): void {
        const key = (event as CustomEvent<{ key: string }>).detail?.key;
        const index = visibleRows.findIndex((row) => row.kind !== 'pending-folder' && row.selectionKey === key);
        if (!list || index === -1) return;
        const top = rowOffset(index, rowMetrics);
        const bottom = rowOffset(index + 1, rowMetrics);
        if (top < list.scrollTop || bottom > list.scrollTop + list.clientHeight) {
            list.scrollTop = Math.max(0, top - Math.max(0, (list.clientHeight - (bottom - top)) / 2));
        }
        updateViewport();
    }

    function applyListSemantics(view: FileListView): void {
        if (!list) return;
        // A grid or a list may contain only rows or items. The loading, empty
        // and error messages are neither, and their own role=status/alert is
        // invalid as a direct child of one, so while a message shows the
        // container drops to a plain labelled group.
        if (view.kind === 'state') {
            list.setAttribute('role', 'group');
            list.removeAttribute('aria-colcount');
            list.removeAttribute('aria-rowcount');
            list.removeAttribute('aria-multiselectable');
            return;
        }
        if (mobile) {
            // The shell starts as a desktop grid so it can render before the
            // mobile portal mounts. A phone row is one two-line item, not four
            // cells, and leaving these attributes in place makes a screen reader
            // invent a header and announce a table that is not on screen.
            list.setAttribute('role', 'list');
            list.removeAttribute('aria-colcount');
            list.removeAttribute('aria-rowcount');
            list.removeAttribute('aria-multiselectable');
            return;
        }
        // aria-rowcount is written by renderFileListRows alongside the rows.
        list.setAttribute('role', 'grid');
        list.setAttribute('aria-colcount', '4');
        list.setAttribute('aria-multiselectable', 'true');
    }

    onMount(() => {
        list = document.getElementById('file-list');
        if (!list) return;
        setFileThumbnailRoot(list);
        // subscribe fires immediately with the current view, which also seeds the
        // initial semantics.
        const unsubscribeListSemantics = fileListView.subscribe(applyListSemantics);
        const onScroll = () => {
            updateViewport();
            retainKeyboardFocus();
        };
        const resizeObserver = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(updateViewport);
        list.addEventListener('scroll', onScroll, { passive: true });
        resizeObserver?.observe(list);
        window.addEventListener('tdrive:reveal-file-row', revealRow);
        window.addEventListener('tdrive:unlocked', rearmFileThumbnailLocked);
        updateViewport();
        return () => {
            list?.removeEventListener('scroll', onScroll);
            resizeObserver?.disconnect();
            window.removeEventListener('tdrive:reveal-file-row', revealRow);
            window.removeEventListener('tdrive:unlocked', rearmFileThumbnailLocked);
            unsubscribeListSemantics();
            teardownFileThumbnails();
            list = null;
        };
    });
</script>

{#snippet phoneLabel(name: string, split: boolean)}
    {@const label = split ? splitRowLabel(name) : { head: name, tail: '' }}
    <span class="row-label">
        {#if label.tail}
            <span class="row-label-head">{label.head}</span><span class="row-label-tail">{label.tail}</span>
        {:else}
            <span class="row-label-head">{name}</span>
        {/if}
    </span>
{/snippet}

<!-- An action's glyph, in one place: the phone draws the same set at its own
     size, and two copies of this list would be two chances to disagree. -->
{#snippet actionGlyph(action: FileListAction, size: number)}
    {#if action.kind === 'open'}
        <ExternalLinkIcon {size} aria-hidden="true" />
    {:else if action.kind === 'play'}
        <PlayIcon {size} aria-hidden="true" />
    {:else if action.kind === 'restore'}
        <RotateCcwIcon {size} aria-hidden="true" />
    {:else if action.kind === 'purge'}
        <Trash2Icon {size} aria-hidden="true" />
    {:else}
        <DownloadIcon {size} aria-hidden="true" />
    {/if}
{/snippet}

<!-- Keep shared content inside each platform's existing semantic wrapper. -->
{#snippet fileIcon(row: FileListFileRow)}
    {#if row.thumbnail}
        <FileThumbnail ext={row.ext} identity={row.thumbnail} />
    {:else}
        {@const family = fileTypeFamily(row.ext)}
        {@const TypeIcon = fileTypeIcon(family)}
        <span class="file-type-icon" data-family={family} aria-hidden="true">
            <TypeIcon size={20} aria-hidden="true" />
        </span>
    {/if}
{/snippet}

{#snippet inlineActions(row: InteractiveRow, size: number)}
    {#each row.actions as action (action.kind)}
        <button
            class={`action-icon ${action.className}`}
            type="button"
            tabindex="-1"
            title={action.title}
            aria-label={action.label}
            onclick={(event) => onActionClick(event, row, action)}
        >
            {@render actionGlyph(action, size)}
            {#if action.text && !mobile}<span class="action-text">{action.text}</span>{/if}
        </button>
    {/each}
{/snippet}

{#if !mobile && $new_folder_draft !== null}
    <!-- The new folder row is the first row of the list, in the normal flow, so
         the rows below move down to make room for it and never sit under it. -->
    <div class="file-row drive-row new-folder-row" data-type="new-folder" role="row">
        <div class="row-name" role="gridcell" aria-colindex="1">
            <span class="folder-chip" aria-hidden="true">
                <FolderIcon size={20} weight="fill" aria-hidden="true" />
            </span>
            <div class="new-folder-field">
                <input
                    class="new-folder-input"
                    type="text"
                    aria-label={$t_store('files.new_folder_name_label')}
                    data-testid="new-folder-input"
                    autocomplete="off"
                    spellcheck="false"
                    value={$new_folder_draft ?? ''}
                    use:focus_select
                    oninput={on_new_folder_input}
                    onkeydown={on_new_folder_keydown}
                    onblur={cancel_new_folder}
                />
                <span class="new-folder-hint">{$t_store('files.new_folder_hint')}</span>
            </div>
        </div>
    </div>
{/if}

{#if $fileListView.kind === 'state'}
    <FileState
        kind={$fileListView.stateKind}
        title={$fileListView.title}
        body={$fileListView.body ?? ''}
        actions={$fileListView.actions ?? []}
    />
{:else if mobile}
    <div aria-hidden="true" style:height={`${rowWindow.before}px`}></div>
    {#each rowWindow.rows as row, rowIndex (row.key)}
        {#if row.kind === 'pending-folder'}
            <div
                class={`file-row drive-row folder-row pending-folder${cardEdges(rowIndex)}`}
                use:measureRow
                data-type="pending-folder"
                data-temp-id={row.tempId}
                role="listitem"
                aria-posinset={rowWindow.start + rowIndex + 1}
                aria-setsize={visibleRows.length}
                title={$t_store('files.creating')}
            >
                <div class="row-swipe-content">
                    <div class="row-name" title={row.name}>
                        <span class="folder-chip" aria-hidden="true">
                            <FolderIcon size={20} weight="fill" aria-hidden="true" />
                        </span>
                        <span class="row-text">
                            {@render phoneLabel(row.name, false)}
                            <span class="row-sub"><span class="row-sub-text">{$t_store('files.creating')}</span><span class="pending-indicator" aria-hidden="true"><LoaderCircleIcon size={12} aria-hidden="true" weight="bold" /></span></span>
                        </span>
                    </div>
                    <div class="row-actions"></div>
                </div>
            </div>
        {:else}
            {@const selected = $selectedFileRowKeys.has(row.selectionKey)}
            {@const meta = rowMetaLine(row, $minuteTick)}
            {@const state = itemStateFor($transfersByFile, row.id)}
            {@const stateInfo = itemStateDescriptor(state)}
            <!-- The focusable item is intentional: the delegated list handler
                 supplies its keyboard contract while the native list role
                 keeps the phone's one-item-per-row announcement. -->
            <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
            <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
            <div
                class={`file-row drive-row${row.kind === 'folder' ? ' folder-row' : ''}${selected ? ' is-selected' : ''}${$activeFileRowKey === row.selectionKey ? ' is-keyboard-active' : ''}${stateInfo.needsExplanation ? ' needs-explanation' : ''}${$busyRowIds.has(row.id) ? ' is-busy' : ''}${cardEdges(rowIndex)}`}
                aria-busy={$busyRowIds.has(row.id) ? 'true' : undefined}
                use:measureRow
                data-type={dataType(row)}
                data-row-key={row.selectionKey}
                data-name={row.name}
                role="listitem"
                aria-posinset={rowWindow.start + rowIndex + 1}
                aria-setsize={visibleRows.length}
                aria-label={selected ? $t_store('files.selected_named', { label: row.ariaLabel }) : row.ariaLabel}
                tabindex={$activeFileRowKey === row.selectionKey ? 0 : -1}
                onclick={(event) => onRowClick(event, row)}
                ondblclick={(event) => onRowDoubleClick(event, row)}
                onkeydown={onGridRowKeydown}
            >
                <div class="row-swipe-content">
                    <div class="row-name" title={row.name}>
                        {#if selecting}
                            <span class="row-check" aria-hidden="true">
                                <CheckIcon size={14} aria-hidden="true" weight="bold" />
                            </span>
                        {/if}
                        {#if row.kind === 'folder'}
                            <span class="folder-chip" aria-hidden="true">
                                <FolderIcon size={20} weight="fill" aria-hidden="true" />
                            </span>
                        {:else}
                            {@render fileIcon(row)}
                        {/if}
                        <span class="row-text">
                            {@render phoneLabel(row.name, row.kind === 'file')}
                            {#if meta || (row.kind === 'file' && (row.encrypted || row.uploaderChip?.firstName))}
                                <span class="row-sub">
                                    {#if meta}
                                        <span class="row-sub-text">{meta}</span>
                                    {/if}
                                    {#if row.kind === 'file' && row.encrypted}
                                        <span class="file-lock-badge" title={$t_store('files.encrypted')} aria-label={$t_store('files.encrypted')}>
                                            <LockKeyholeIcon size={12} aria-hidden="true" weight="bold" />
                                        </span>
                                    {/if}
                                    {#if row.kind === 'file' && row.uploaderChip?.firstName}
                                        <span class="uploader-chip">
                                            <span class="uploader-initials" aria-hidden="true">{row.uploaderChip.initials}</span>
                                            {row.uploaderChip.firstName}
                                        </span>
                                    {/if}
                                </span>
                            {/if}
                            {#if mobile && stateInfo.needsExplanation}
                                <span class="row-explain" data-tone={stateInfo.tone}>{stateInfo.detail}</span>
                            {/if}
                        </span>
                    </div>
                    {#if mobile}
                        <div class="row-status">
                            <ItemStatus state={state} onOpenQueue={() => activeTab.set('transfers')} />
                        </div>
                    {/if}
                    <div class={`row-actions${row.actionsInline ? ' is-inline' : ''}`}>
                        {#if row.actionsInline}
                            <!-- Few, important, and with no menu behind them, so the
                                 phone shows them the way the desktop does rather
                                 than hiding them in a sheet built for a live item. -->
                            {@render inlineActions(row, 20)}
                        {:else}
                            <button
                                class="action-icon row-more"
                                type="button"
                                aria-label={$t_store('files.more_actions_for', { name: row.name })}
                                aria-haspopup="menu"
                            >
                                <EllipsisIcon size={20} aria-hidden="true" />
                            </button>
                        {/if}
                    </div>
                </div>
                {#if mobile && !row.actionsInline}
                    <!-- Revealed by a trailing swipe. Delete is available only
                         where the item may be deleted, and still opens the
                         existing Trash confirmation rather than acting on swipe.
                         Hidden from assistive tech because it duplicates what
                         the always-visible overflow button already offers; a
                         screen reader should not meet the same action twice. -->
                    <div class="row-swipe-actions" aria-hidden="true">
                        <button class="row-swipe-btn" type="button" tabindex="-1" data-swipe-action="move">
                            <FolderInputIcon size={20} aria-hidden="true" />
                            <span>{$t_store('files.swipe_move')}</span>
                        </button>
                        {#if row.kind === 'folder' || row.canDelete}
                            <button class="row-swipe-btn is-delete" type="button" tabindex="-1" data-swipe-action="delete">
                                <Trash2Icon size={20} aria-hidden="true" />
                                <span>{$t_store('files.swipe_delete')}</span>
                            </button>
                        {/if}
                    </div>
                {/if}
            </div>
        {/if}
    {/each}
    <div aria-hidden="true" style:height={`${rowWindow.after}px`}></div>
{:else}
    <div aria-hidden="true" style:height={`${rowWindow.before}px`}></div>
    {#each rowWindow.rows as row, rowIndex (row.key)}
        {#if row.kind === 'pending-folder'}
            <div
                class="file-row drive-row folder-row pending-folder"
                use:measureRow
                data-type="pending-folder"
                data-temp-id={row.tempId}
                role="row"
                aria-rowindex={rowWindow.start + rowIndex + 2}
                title={$t_store('files.creating')}
            >
                <div class="row-name" role="gridcell" aria-colindex="1" title={row.name}>
                    <span class="folder-chip" aria-hidden="true">
                        <FolderIcon size={20} weight="fill" aria-hidden="true" />
                    </span>
                    <span class="row-label">{row.name}</span>
                    <span class="pending-indicator" aria-hidden="true"><LoaderCircleIcon size={12} aria-hidden="true" weight="bold" /></span>
                </div>
                <div class="row-meta" role="gridcell" aria-colindex="2">{$t_store('files.creating')}</div>
                <div class="row-meta" role="gridcell" aria-colindex="3"><span aria-hidden="true">…</span></div>
                <div class="row-actions" role="gridcell" aria-colindex="4"></div>
            </div>
        {:else}
            <div
                class={`file-row drive-row${row.kind === 'folder' ? ' folder-row' : ''}${$selectedFileRowKeys.has(row.selectionKey) ? ' is-selected' : ''}${$activeFileRowKey === row.selectionKey ? ' is-keyboard-active' : ''}${$busyRowIds.has(row.id) ? ' is-busy' : ''}`}
                aria-busy={$busyRowIds.has(row.id) ? 'true' : undefined}
                use:measureRow
                data-type={dataType(row)}
                data-row-key={row.selectionKey}
                data-name={row.name}
                role="row"
                aria-rowindex={rowWindow.start + rowIndex + 2}
                aria-selected={$selectedFileRowKeys.has(row.selectionKey) ? 'true' : 'false'}
                aria-label={row.ariaLabel}
                tabindex={$activeFileRowKey === row.selectionKey ? 0 : -1}
                onclick={(event) => onRowClick(event, row)}
                ondblclick={(event) => onRowDoubleClick(event, row)}
                onkeydown={onGridRowKeydown}
            >
                <div class="row-name" role="gridcell" aria-colindex="1" draggable="true" title={row.name}>
                    <!-- Ctrl+click without the open. The delegated list handler
                         does the toggling; this button is only the affordance.
                         It is out of the tab order because the row already takes
                         Space and the arrows for selection. -->
                    {#if $selectionModeOn}
                        <button
                            class="row-select"
                            type="button"
                            role="checkbox"
                            tabindex="-1"
                            aria-checked={$selectedFileRowKeys.has(row.selectionKey) ? 'true' : 'false'}
                            aria-label={$t_store('files.select_named', { name: row.name })}
                            data-testid="row-select"
                            ondblclick={(event) => event.stopPropagation()}
                        >
                            {#if $selectedFileRowKeys.has(row.selectionKey)}
                                <CheckIcon size={12} weight="bold" aria-hidden="true" />
                            {/if}
                        </button>
                    {/if}
                    {#if row.kind === 'folder'}
                        <span class="folder-chip" aria-hidden="true">
                            <FolderIcon size={20} weight="fill" aria-hidden="true" />
                        </span>
                        <span class="row-label">{row.name}</span>
                    {:else}
                        {@render fileIcon(row)}
                        {#if row.encrypted}
                            <span class="file-lock-badge" title={$t_store('files.encrypted')} aria-label={$t_store('files.encrypted')}>
                                <LockKeyholeIcon size={12} aria-hidden="true" weight="bold" />
                            </span>
                        {/if}
                        <!-- The glyph says document or video; only the name says mkv or mp4. -->
                        <span class="row-label">{row.name}</span>
                        {#if row.uploaderChip}
                            <span class="uploader-chip">{row.uploaderChip.label}</span>
                        {/if}
                    {/if}
                </div>
                <div class={`row-meta${row.metaUrgent ? ' is-urgent' : ''}`} role="gridcell" aria-colindex="2">{row.metaLabel}</div>
                <div class={`row-meta ${row.kind === 'folder' ? 'folder-size' : ''}`} role="gridcell" aria-colindex="3">{row.sizeLabel}</div>
                <div class={`row-actions${row.actionsInline ? ' is-inline' : ''}`} role="gridcell" aria-colindex="4">
                    {#each desktop_action_slots(row) as slot, slotIndex (slotIndex)}
                        {#if slot === 'more'}
                            <!-- Opens the same menu as a right click, under the button,
                                 so the row's full set of actions is discoverable
                                 without one. -->
                            <button
                                class="action-icon row-more"
                                type="button"
                                tabindex="-1"
                                title={$t_store('files.more')}
                                aria-label={$t_store('files.more_actions_for', { name: row.name })}
                                aria-haspopup="menu"
                            >
                                <EllipsisIcon size={16} aria-hidden="true" />
                            </button>
                        {:else if slot}
                            <button
                                class={`action-icon ${slot.className}`}
                                type="button"
                                tabindex="-1"
                                title={slot.title}
                                aria-label={slot.label}
                                onclick={(event) => onActionClick(event, row, slot)}
                            >
                                {@render actionGlyph(slot, 16)}
                            </button>
                        {:else}
                            <span class="action-slot" aria-hidden="true"></span>
                        {/if}
                    {/each}
                </div>
            </div>
        {/if}
    {/each}
    <div aria-hidden="true" style:height={`${rowWindow.after}px`}></div>
{/if}
