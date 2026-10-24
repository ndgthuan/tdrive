// Selection handling module for TDrive frontend

import { get } from 'svelte/store';
import { state } from '../state';
import { isMobilePlatform } from '../api';
import { getInteractiveFileListRows } from '../ui/file_list/file_list_store';
import { openDeleteModal } from './modals/delete';
import { openMoveModal } from './modals/move';
import { enqueueDownload, enqueueFolderDownload } from './transfers';
import { notify } from './notifications';
import { t } from '../i18n/i18n';
import { selectionModeOn, setSelectionCount, setSelectionModeActive } from '../ui/selection/selection_bar_store';
import { setSelectedFileRowKeys } from '../ui/file_list/row_state_store';
import { fileListRowForElement } from '../ui/file_list/row_lookup';
import { setActiveFileRowKey } from '../ui/file_list/row_state_store';
import { rangeSelectionKeys } from '../ui/file_list/keyboard';
import type { FileCommandItem, FileListFileRow, FolderListRow } from '../ui/file_list/types';

const SELECTABLE_ROW_SELECTOR = '.drive-row[data-type="folder"], .drive-row[data-type="file"]';
const MAX_BULK_DOWNLOAD_ITEMS = 500;
let selectionAnchorKey = '';

type LogicalFileListRow = FolderListRow | FileListFileRow;

/**
 * A row element's identity, and the only thing selection reads off the markup.
 * Every other field it needs belongs to the row itself and is fetched from the
 * store, so the element and the row can never drift apart.
 */
export function getRowKey(row: HTMLElement): string {
    return row.dataset.rowKey ?? '';
}

function syncSelectedRowKeys(): void {
    setSelectedFileRowKeys(state.selectedItems.keys());
}

export function isRowSelected(row: HTMLElement): boolean {
    const key = getRowKey(row);
    return Boolean(key && state.selectedItems.has(key));
}

/**
 * The selection entry for a rendered row, keyed back to the row the list drew
 * it from. Null when the element no longer belongs to the published list, in
 * which case there is nothing to select: the caller leaves the selection alone
 * rather than adding an entry built from a row the reader cannot see.
 */
function selectionItemForElement(element: HTMLElement): { key: string; item: FileCommandItem } | null {
    const row = fileListRowForElement(element);
    if (!row) return null;
    return { key: row.selectionKey, item: logicalRowToSelectionItem(row, element) };
}

function logicalRowToSelectionItem(row: LogicalFileListRow, element?: HTMLElement): FileCommandItem {
    const withElement = element ? { row: element } : {};
    if (row.kind === 'folder') {
        return {
            type: 'folder',
            id: row.id,
            name: row.name,
            channelId: row.channelId,
            parentId: row.parentId,
            canDelete: true,
            canRename: true,
            ...withElement,
        };
    }
    if (row.source === 'tg') {
        return {
            type: 'file',
            id: Number(row.id),
            name: row.name,
            channelId: row.channelId,
            size: row.size,
            source: 'tg',
            parentId: row.parentId,
            uploaderID: row.uploaderID,
            canDelete: row.canDelete,
            canRename: row.canRename,
            ...withElement,
        };
    }
    return {
        type: 'file',
        id: Number(row.id),
        name: row.name,
        channelId: row.channelId,
        size: row.size,
        source: 'fs',
        parentId: row.parentId,
        uploaderID: row.uploaderID,
        canDelete: row.canDelete,
        canRename: row.canRename,
        ...withElement,
    };
}

function renderedRowsByKey(list: HTMLElement): Map<string, HTMLElement> {
    return new Map(Array.from(list.querySelectorAll<HTMLElement>(SELECTABLE_ROW_SELECTOR))
        .map((row) => [getRowKey(row), row] as const)
        .filter(([key]) => Boolean(key)));
}
// Whether the desktop list is in selection mode. A plain click selects one row
// without it; Ctrl/Shift click, Ctrl+A, a press and hold, and the Select button
// enter it, and Done, Clear, Escape or emptying the selection leave it.
export function is_selection_mode_on(): boolean {
    return get(selectionModeOn);
}

export function updateSelectionBar(): void {
    syncSelectedRowKeys();
    const count = state.selectedItems.size;
    setSelectionCount(count);
    if (!state.selectionBarEl) return;
    // The phone keeps its own rule: the bar shows whenever anything is selected.
    const visible = isMobilePlatform() ? count > 0 : is_selection_mode_on() && count > 0;
    state.selectionBarEl.style.display = visible ? 'flex' : 'none';
}

export function clearSelection({ keepAnchor = false }: { keepAnchor?: boolean } = {}): void {
    state.selectedItems.clear();
    setSelectionModeActive(false);
    if (!keepAnchor) {
        selectionAnchorKey = '';
        state.selectionAnchorIndex = -1;
    }
    updateSelectionBar();
}

export function startSelectionMode(): void {
    setSelectionModeActive(true);
    updateSelectionBar();
}

export function selectRow(row: HTMLElement, rowIndex: number): void {
    const selected = selectionItemForElement(row);
    if (!selected) return;
    state.selectedItems.set(selected.key, selected.item);
    selectionAnchorKey = selected.key;
    state.selectionAnchorIndex = rowIndex;
    updateSelectionBar();
}

export function deselectRow(row: HTMLElement): void {
    const key = getRowKey(row);
    if (!key) return;
    state.selectedItems.delete(key);
    // Unticking the last row ends the mode, as Clear does.
    if (state.selectedItems.size === 0) setSelectionModeActive(false);
    updateSelectionBar();
}

// A keyed file-list update can replace row nodes. When the grid is windowed,
// logicalRows keeps offscreen selections and the range anchor intact.
export function reconcileSelection(list: HTMLElement, logicalRows?: readonly LogicalFileListRow[]): void {
    const renderedRows = Array.from(list.querySelectorAll<HTMLElement>(SELECTABLE_ROW_SELECTOR));
    const renderedByKey = renderedRowsByKey(list);
    const previous = state.selectedItems;
    const next = new Map<string, FileCommandItem>();

    if (logicalRows) {
        for (const row of logicalRows) {
            if (!previous.has(row.selectionKey)) continue;
            next.set(row.selectionKey, logicalRowToSelectionItem(row, renderedByKey.get(row.selectionKey)));
        }
    } else {
        for (const row of renderedRows) {
            const selected = selectionItemForElement(row);
            if (!selected || !previous.has(selected.key)) continue;
            next.set(selected.key, selected.item);
        }
    }

    previous.clear();
    for (const [key, item] of next) previous.set(key, item);

    const anchorIndex = logicalRows
        ? logicalRows.findIndex((row) => row.selectionKey === selectionAnchorKey)
        : renderedRows.findIndex((row) => getRowKey(row) === selectionAnchorKey);
    if (anchorIndex === -1) {
        selectionAnchorKey = '';
        state.selectionAnchorIndex = -1;
    } else {
        state.selectionAnchorIndex = anchorIndex;
    }
    updateSelectionBar();
}

export function handleRowSelection(
    row: HTMLElement,
    event: MouseEvent | KeyboardEvent,
    logicalRows?: readonly LogicalFileListRow[],
): void {
    if ('button' in event && event.button === 2) return;

    const list = document.getElementById('file-list');
    const renderedRows = list ? Array.from(list.querySelectorAll<HTMLElement>(SELECTABLE_ROW_SELECTOR)) : [];
    const key = getRowKey(row);
    const index = logicalRows
        ? logicalRows.findIndex((candidate) => candidate.selectionKey === key)
        : renderedRows.indexOf(row);
    if (index === -1) return;

    // In selection mode a plain click toggles the row, as Ctrl+click does.
    const isToggle = event.metaKey || event.ctrlKey || is_selection_mode_on();
    const anchorIndex = selectionAnchorKey
        ? logicalRows
            ? logicalRows.findIndex((candidate) => candidate.selectionKey === selectionAnchorKey)
            : renderedRows.findIndex((candidate) => getRowKey(candidate) === selectionAnchorKey)
        : state.selectionAnchorIndex;
    const isRange = event.shiftKey && anchorIndex >= 0;

    if (isRange) {
        const start = Math.min(anchorIndex, index);
        const end = Math.max(anchorIndex, index);
        if (!isToggle) clearSelection({ keepAnchor: true });
        startSelectionMode();
        const renderedByKey = list ? renderedRowsByKey(list) : new Map<string, HTMLElement>();

        for (let cursor = start; cursor <= end; cursor += 1) {
            if (logicalRows) {
                const rangeRow = logicalRows[cursor];
                if (!rangeRow || state.selectedItems.has(rangeRow.selectionKey)) continue;
                state.selectedItems.set(
                    rangeRow.selectionKey,
                    logicalRowToSelectionItem(rangeRow, renderedByKey.get(rangeRow.selectionKey)),
                );
                continue;
            }
            const rangeRow = renderedRows[cursor];
            if (!rangeRow) continue;
            const selected = selectionItemForElement(rangeRow);
            if (!selected || state.selectedItems.has(selected.key)) continue;
            state.selectedItems.set(selected.key, selected.item);
        }
        updateSelectionBar();
        return;
    }

    if (isToggle) {
        if (isRowSelected(row)) {
            deselectRow(row);
        } else {
            selectRow(row, index);
            startSelectionMode();
        }
        return;
    }

    clearSelection({ keepAnchor: true });
    selectRow(row, index);
}

// A press and hold on a row: one selected row, and the mode on. Anything selected
// before is dropped, because the hold is the start of a fresh selection.
export function enter_selection_with_row(row: HTMLElement): void {
    const index = getInteractiveFileListRows().findIndex((candidate) => candidate.selectionKey === getRowKey(row));
    clearSelection();
    if (index === -1) return;
    selectRow(row, index);
    // The held row also becomes the keyboard's row, so its ring is the one shown.
    setActiveFileRowKey(getRowKey(row));
    startSelectionMode();
}

// Ctrl/Cmd+click semantics for one row: add it when it is not selected, drop it
// when it is. The row's position in the published list becomes its anchor, so a
// later Shift+click ranges from it. Used by the checkbox and the phone tap.
export function toggle_row_selection(row: HTMLElement): void {
    if (isRowSelected(row)) {
        deselectRow(row);
        return;
    }
    const index = getInteractiveFileListRows().findIndex((candidate) => candidate.selectionKey === getRowKey(row));
    selectRow(row, index);
    startSelectionMode();
}

// The header checkbox: selects every row when some are unselected, and clears
// the selection when every row already is.
export function toggle_select_all(logicalRows: readonly LogicalFileListRow[] = getInteractiveFileListRows()): void {
    if (logicalRows.length === 0) return;
    const allSelected = logicalRows.every((row) => state.selectedItems.has(row.selectionKey));
    if (allSelected) clearSelection();
    else selectAllRows(logicalRows);
}

// Rebuilds the selection from a set of logical rows, attaching the rendered
// element where the windowed list currently shows one. Shared by select-all and
// range extension, which both replace the selection wholesale.
function setSelectionFromRows(rows: readonly LogicalFileListRow[]): void {
    const list = document.getElementById('file-list');
    const renderedByKey = list ? renderedRowsByKey(list) : new Map<string, HTMLElement>();
    state.selectedItems.clear();
    for (const row of rows) {
        state.selectedItems.set(row.selectionKey, logicalRowToSelectionItem(row, renderedByKey.get(row.selectionKey)));
    }
    updateSelectionBar();
}

// Selects every row of the current folder, keeping the anchor where it is so a
// following Shift+Arrow still extends from the same point. The rows come from
// the list's model, not the DOM, so rows the windowed list has not drawn are
// selected too.
export function selectAllRows(logicalRows: readonly LogicalFileListRow[] = getInteractiveFileListRows()): void {
    if (logicalRows.length === 0) return;
    const anchorIndex = selectionAnchorKey
        ? logicalRows.findIndex((row) => row.selectionKey === selectionAnchorKey)
        : -1;
    setSelectionFromRows(logicalRows);
    startSelectionMode();
    if (anchorIndex < 0) {
        selectionAnchorKey = logicalRows[0].selectionKey;
        state.selectionAnchorIndex = 0;
    } else {
        state.selectionAnchorIndex = anchorIndex;
    }
}

// Extends the selection from the current anchor to `targetIndex`, the model a
// Shift+Arrow shares with a Shift+Click. The range is rebuilt each time so
// pulling back toward the anchor deselects the tail. Falls back to anchoring on
// the target when nothing is anchored yet.
export function extendSelectionToIndex(
    logicalRows: readonly LogicalFileListRow[],
    targetIndex: number,
    fallbackAnchorIndex: number = targetIndex,
): void {
    if (targetIndex < 0 || targetIndex >= logicalRows.length) return;
    // With a selection in progress the anchor is its start; with none, the key
    // or index left over from an earlier selection means nothing, so the range
    // starts from the caller's current row instead.
    let anchorIndex = state.selectedItems.size > 0 && selectionAnchorKey
        ? logicalRows.findIndex((row) => row.selectionKey === selectionAnchorKey)
        : -1;
    if (anchorIndex < 0 || anchorIndex >= logicalRows.length) {
        anchorIndex = Math.min(logicalRows.length - 1, Math.max(0, fallbackAnchorIndex));
        selectionAnchorKey = logicalRows[anchorIndex].selectionKey;
        state.selectionAnchorIndex = anchorIndex;
    }
    const keys = new Set(rangeSelectionKeys(logicalRows, anchorIndex, targetIndex));
    setSelectionFromRows(logicalRows.filter((row) => keys.has(row.selectionKey)));
    startSelectionMode();
}

export function ensureRowSelectedForContextMenu(row: HTMLElement): void {
    const list = document.getElementById('file-list');
    const rows = list ? Array.from(list.querySelectorAll<HTMLElement>(SELECTABLE_ROW_SELECTOR)) : [];
    const index = rows.indexOf(row);
    if (index === -1) return;

    if (!isRowSelected(row) || state.selectedItems.size === 0) {
        clearSelection({ keepAnchor: true });
        selectRow(row, index);
        return;
    }
    selectionAnchorKey = getRowKey(row);
    state.selectionAnchorIndex = index;
}

export function getSelectionPayload(): FileCommandItem[] {
    // The stored items already carry the folder/telegram/filesystem shape that
    // logicalRowToSelectionItem built; the payload only has to drop the live row
    // element, which modals and the backend have no use for. Rebuilding the three
    // branches here was a second copy of that mapping, free to drift from it.
    return Array.from(state.selectedItems.values(), ({ row: _row, ...payload }) => payload);
}

export function openSelectedItemsDelete(): void {
    if (state.selectedItems.size === 0) return;
    openDeleteModal({ type: 'bulk', items: getSelectionPayload(), parentId: state.currentFolderId });
}

export function openSelectedItemsMove(): void {
    if (state.selectedItems.size === 0) return;
    openMoveModal({ type: 'bulk', items: getSelectionPayload(), parentId: state.currentFolderId });
}

export function openSelectedItemsDownload(): void {
    const items = getSelectionPayload();
    if (items.length === 0) return;
    if (items.length > MAX_BULK_DOWNLOAD_ITEMS) {
        notify({
            level: 'warning',
            title: t('selection.too_many_title'),
            body: t('selection.too_many_body', { count: MAX_BULK_DOWNLOAD_ITEMS }),
        });
        return;
    }
    const downloads = items.map((item) => ({
        item,
        channelId: Number.isSafeInteger(item.channelId) && Number(item.channelId) > 0
            ? Number(item.channelId)
            : null,
    }));
    if (downloads.some(({ channelId }) => channelId === null)) {
        notify({
            level: 'error',
            title: t('selection.unavailable_title'),
            body: t('selection.unavailable_body'),
        });
        return;
    }
    for (const { item, channelId } of downloads) {
        if (item.type === 'folder') {
            enqueueFolderDownload(item.id, item.name, 0, channelId);
        } else {
            enqueueDownload(item.id, item.name, item.size ?? 0, channelId);
        }
    }
    clearSelection();
}

export function activateSelectionBar(): () => void {
    const selectionBar = document.getElementById('selection-bar');
    if (!selectionBar) return () => {};

    state.selectionBarEl = selectionBar;
    const list = document.getElementById('file-list');
    const onListClick = (event: MouseEvent) => {
        if ((event.target as HTMLElement).closest('.drive-row')) return;
        clearSelection();
    };
    const onKeydown = (event: KeyboardEvent) => {
        // A menu or dialog that closed on this same Escape has already claimed
        // it (both call preventDefault), so clearing the selection as well would
        // make one press do two things. Only an Escape nothing else wanted
        // clears the selection.
        if (event.key === 'Escape' && !event.defaultPrevented) clearSelection();
    };

    list?.addEventListener('click', onListClick);
    window.addEventListener('keydown', onKeydown);
    updateSelectionBar();

    return () => {
        list?.removeEventListener('click', onListClick);
        window.removeEventListener('keydown', onKeydown);
        clearSelection();
        if (state.selectionBarEl === selectionBar) state.selectionBarEl = null;
    };
}
