// The desktop list's keyboard and drag contract. Every one of these paths only
// ever holds an element, so each is a chance for the row it acts on to differ
// from the row the reader is looking at.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import { get } from 'svelte/store';

const api = vi.hoisted(() => ({
    isMobilePlatform: vi.fn(() => false),
    isIOSPlatform: () => false,
    isAndroidPlatform: () => false,
    onRuntimeEvent: () => () => {},
    playHaptic: vi.fn(),
}));
const dragDrop = vi.hoisted(() => ({
    beginRowDrag: vi.fn(),
    endRowDrag: vi.fn(),
    canDropOnFolder: vi.fn(() => true),
    setDropHighlight: vi.fn(),
    performDropMove: vi.fn(),
}));
const modals = vi.hoisted(() => ({
    openDeleteModal: vi.fn(),
    openRenameModal: vi.fn(),
    openMoveModal: vi.fn(),
    openNewFolderModal: vi.fn(),
    submitFolder: vi.fn(() => Promise.resolve()),
}));

const actions = vi.hoisted(() => ({
    playVideo: vi.fn(), openFile: vi.fn(), triggerRefresh: vi.fn(), refreshFiles: vi.fn(),
    navigateToFolder: vi.fn(), navigateBack: vi.fn(), enqueueDownload: vi.fn(), enqueueFolderDownload: vi.fn(),
    preview: vi.fn(), notify: vi.fn(),
}));

vi.mock('../api', () => api);
vi.mock('./drag_drop', () => dragDrop);
vi.mock('./modals/delete', () => ({ openDeleteModal: modals.openDeleteModal }));
vi.mock('./modals/rename', () => ({ openRenameModal: modals.openRenameModal }));
vi.mock('./modals/move', () => ({ openMoveModal: modals.openMoveModal }));
vi.mock('./modals/folder', () => ({ openNewFolderModal: modals.openNewFolderModal, submitFolder: modals.submitFolder }));
vi.mock('./app_actions', () => ({ appActions: () => actions }));
vi.mock('./navigation', () => ({ navigateToFolder: actions.navigateToFolder, navigateBack: actions.navigateBack }));
vi.mock('./notifications', () => ({ notify: actions.notify }));
vi.mock('./transfers', () => ({
    chooseFilesForCurrentFolder: vi.fn(),
    enqueueDownload: actions.enqueueDownload,
    enqueueFolderDownload: actions.enqueueFolderDownload,
}));
vi.mock('./connectivity', () => ({ isOffline: () => false }));
vi.mock('./context_menu', () => ({ showRowContextMenu: vi.fn() }));
vi.mock('./modals/preview', () => ({ activatePreviewModal: vi.fn(), openPreviewList: actions.preview }));
vi.mock('./gallery', () => ({ renderGallery: vi.fn(), setPhotosMode: vi.fn() }));
vi.mock('./uploaders', () => ({ ensureUserNames: vi.fn(), uploaderChipLabel: () => null }));
vi.mock('./drive_data', () => ({ calculateVisibleFolderStats: vi.fn() }));
vi.mock('./folder_index', () => ({ refreshFolderIndex: vi.fn(() => Promise.resolve({ children: new Map() })), collectDescendants: vi.fn(() => []) }));

import FileList from '../ui/file_list/file_list.svelte';
import { activateFileList, buildFileRow, buildFolderRow, renderFileListRows, renderFileState, start_inline_folder_create } from './file_list';
import { clearSelection, startSelectionMode } from './selection';
import { selectionModeOn, setSelectionModeActive } from '../ui/selection/selection_bar_store';
import { state } from '../state';
import { refreshFolderIndex, collectDescendants } from './folder_index';
import type { FileListFileRow } from '../ui/file_list/types';
import { setSelectedFileRowKeys } from '../ui/file_list/row_state_store';

let list: HTMLElement;
let app: Record<string, unknown> | null = null;
let deactivate = () => {};

function row(key: string): HTMLElement {
    const node = list.querySelector<HTMLElement>(`.drive-row[data-row-key="${key}"]`);
    if (!node) throw new Error(`missing row ${key}`);
    return node;
}

function press(target: Element, key: string): void {
    target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
}

function pointer(type: string, target: EventTarget, init: PointerEventInit = {}): void {
    target.dispatchEvent(new PointerEvent(type, {
        bubbles: true,
        cancelable: true,
        button: 0,
        isPrimary: true,
        pointerId: 7,
        pointerType: 'mouse',
        ...init,
    }));
}

function drag(target: Element, type: string): void {
    target.dispatchEvent(new Event(type, { bubbles: true, cancelable: true }));
}

function publish(...overrides: Partial<FileListFileRow>[]): void {
    renderFileListRows(list, [
        buildFolderRow({ id: 'design', name: 'Design' }, ''),
        buildFileRow({ id: 41, name: 'plan.pdf', size: 2_000_000, date: 1_700_000_000 }, '', overrides[0] ?? {}),
    ]);
    flushSync();
}

beforeEach(() => {
    vi.clearAllMocks();
    dragDrop.canDropOnFolder.mockReturnValue(true);
    state.activeChannel = { id: 1, title: 'Drive', kind: 'personal' };
    state.currentFolderId = '';
    state.virtualView = null;
    state.searchQuery = '';
    state.dragOverEl = null;
    state.selectedItems.clear();
    setSelectedFileRowKeys([]);
    state.dragState = null;
    list = document.createElement('div');
    list.id = 'file-list';
    document.body.append(list);
    app = mount(FileList, { target: list });
    deactivate = activateFileList();
    publish();
});

afterEach(async () => {
    deactivate();
    vi.useRealTimers();
    state.selectedItems.clear();
    setSelectionModeActive(false);
    state.dragState = null;
    if (app) await unmount(app);
    app = null;
    list.remove();
});

describe('acting on a row from the keyboard', () => {
    it('deletes with the size and source the list published for the row', () => {
        press(row('file:41'), 'Delete');

        expect(modals.openDeleteModal).toHaveBeenCalledWith({
            type: 'file',
            id: 41,
            name: 'plan.pdf',
            size: 2_000_000,
            parentId: '',
            source: 'fs',
            canDelete: true,
            channelId: 1,
        });
    });

    it('refuses to delete a row the published row says cannot be deleted, and says why', () => {
        publish({ canDelete: false });

        press(row('file:41'), 'Delete');

        expect(modals.openDeleteModal).not.toHaveBeenCalled();
        expect(actions.notify).toHaveBeenCalledWith(expect.objectContaining({ title: expect.stringContaining('delete') }));
    });

    it('refuses to rename a row the published row says cannot be renamed, and says why', () => {
        publish({ canRename: false });

        press(row('file:41'), 'F2');

        expect(modals.openRenameModal).not.toHaveBeenCalled();
        expect(actions.notify).toHaveBeenCalledWith(expect.objectContaining({ title: expect.stringContaining('rename') }));
    });

    it('renames a folder by its id rather than by a number parsed out of it', () => {
        press(row('folder:design'), 'F2');

        expect(modals.openRenameModal).toHaveBeenCalledWith({
            type: 'folder',
            id: 'design',
            name: 'Design',
            parentId: '',
        });
    });

    it('does nothing for a row the list does not publish', () => {
        // Left over from an earlier render: still in the markup, no longer a
        // row anybody can act on.
        const stale = document.createElement('div');
        stale.className = 'drive-row';
        stale.dataset.type = 'file';
        stale.dataset.rowKey = 'file:999';
        list.append(stale);

        press(stale, 'Delete');
        press(stale, 'F2');

        expect(modals.openDeleteModal).not.toHaveBeenCalled();
        expect(modals.openRenameModal).not.toHaveBeenCalled();
    });
});

describe('dragging a row', () => {
    it('drags the item the published row describes', () => {
        drag(row('file:41').querySelector('.row-name')!, 'dragstart');

        expect(dragDrop.beginRowDrag).toHaveBeenCalledWith(
            row('file:41'),
            [expect.objectContaining({ type: 'file', id: 41, name: 'plan.pdf', size: 2_000_000, source: 'fs', parentId: '' })],
            '',
            new Set(),
        );
    });

    it('offers a folder as a drop target by the id the list currently holds', () => {
        state.dragState = { items: [], parentId: '', blocked: new Set(), row: row('file:41') };

        drag(row('folder:design'), 'dragover');

        expect(dragDrop.canDropOnFolder).toHaveBeenCalledWith('design');
        expect(dragDrop.setDropHighlight).toHaveBeenCalledWith(row('folder:design'), true);
    });

    it('will not drop onto a file row, whatever the pointer is over', () => {
        state.dragState = { items: [], parentId: '', blocked: new Set(), row: row('file:41') };

        drag(row('file:41'), 'dragover');

        expect(dragDrop.setDropHighlight).not.toHaveBeenCalled();
    });

    it('moves into the folder the drop landed on', async () => {
        state.dragState = { items: [], parentId: '', blocked: new Set(), row: row('file:41') };

        drag(row('folder:design'), 'drop');
        await Promise.resolve();

        expect(dragDrop.performDropMove).toHaveBeenCalledWith('design');
    });
});


describe('desktop activation and lifecycle', () => {
    it.each([
        ['movie.mp4', 'playVideo'],
        ['notes.txt', 'openFile'],
        ['bundle.zip', 'enqueueDownload'],
    ] as const)('opens %s from Enter with the captured drive identity', (name, action) => {
        renderFileListRows(list, [buildFileRow({ id: 41, name, size: 25 }, '')]);
        flushSync();
        state.activeChannel = { id: 2, title: 'Other', kind: 'personal' };
        press(row('file:41'), 'Enter');
        if (action === 'enqueueDownload') {
            expect(actions.enqueueDownload).toHaveBeenCalledWith(41, name, 25, 1);
        } else {
            expect(actions[action]).toHaveBeenCalledWith(expect.objectContaining({ id: 41, name, channelId: 1 }));
        }
    });

    it('opens an image preview from Enter', async () => {
        renderFileListRows(list, [buildFileRow({ id: 7, name: 'photo.jpg', size: 9 }, '')]);
        flushSync();
        press(row('file:7'), 'Enter');
        await vi.waitFor(() => expect(actions.preview).toHaveBeenCalledTimes(1));
        expect(actions.enqueueDownload).not.toHaveBeenCalled();
    });

    it('opens folders and downloads them through distinct affordances', () => {
        press(row('folder:design'), 'Enter');
        row('folder:design').querySelector<HTMLButtonElement>('button.download-folder')?.click();
        expect(actions.navigateToFolder).toHaveBeenCalledWith('design', 'Design');
        expect(actions.enqueueFolderDownload).toHaveBeenCalledWith('design', 'Design', 0, 1);
        expect(state.selectedItems.size).toBe(0);
    });

    it('does not open a folder when its Download action is double-clicked', () => {
        const download = row('folder:design').querySelector<HTMLButtonElement>('button.download-folder')!;
        download.click();
        download.dispatchEvent(new MouseEvent('dblclick', { bubbles: true, cancelable: true }));

        expect(actions.enqueueFolderDownload).toHaveBeenCalledTimes(1);
        expect(actions.navigateToFolder).not.toHaveBeenCalled();
    });

    it.each([
        ['movie.mp4', 'play-video', 'playVideo'],
        ['notes.txt', 'open-file', 'openFile'],
        ['bundle.zip', 'download', 'enqueueDownload'],
    ] as const)('runs the %s button without selecting its row', (name, className, action) => {
        renderFileListRows(list, [buildFileRow({ id: 41, name, size: 25 }, '')]);
        flushSync();
        row('file:41').querySelector<HTMLButtonElement>(`button.${className}`)?.click();
        expect(actions[action]).toHaveBeenCalledTimes(1);
        expect(state.selectedItems.size).toBe(0);
    });

    it.each([
        ['movie.mp4', 'playVideo'], ['notes.txt', 'openFile'],
    ] as const)('opens %s by double-clicking anywhere on the row', (name, action) => {
        renderFileListRows(list, [buildFileRow({ id: 41, name }, '')]);
        flushSync();
        row('file:41').querySelector('.row-meta')?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
        expect(actions[action]).toHaveBeenCalledTimes(1);
        expect(modals.openRenameModal).not.toHaveBeenCalled();
    });

    it('does not play a video again when its Play action receives a double-click', () => {
        renderFileListRows(list, [buildFileRow({ id: 41, name: 'movie.mp4' }, '')]);
        flushSync();
        const play = row('file:41').querySelector<HTMLButtonElement>('button.play-video')!;
        play.click();
        play.dispatchEvent(new MouseEvent('dblclick', { bubbles: true, cancelable: true }));

        expect(actions.playVideo).toHaveBeenCalledTimes(1);
    });

    it('previews an image on double click and from its row Open action', async () => {
        renderFileListRows(list, [buildFileRow({ id: 41, name: 'photo.jpg', size: 25 }, '')]);
        flushSync();
        expect(row('file:41').querySelector('button.open-image')).not.toBeNull();
        row('file:41').dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
        await vi.waitFor(() => expect(actions.preview).toHaveBeenCalledTimes(1));
        row('file:41').querySelector<HTMLButtonElement>('button.open-image')?.click();
        await vi.waitFor(() => expect(actions.preview).toHaveBeenCalledTimes(2));
    });

    it('downloads a plain file on double click and never renames it', () => {
        renderFileListRows(list, [buildFileRow({ id: 41, name: 'bundle.zip', size: 25 }, '')]);
        flushSync();
        row('file:41').querySelector('.row-name')?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
        expect(actions.enqueueDownload).toHaveBeenCalledWith(41, 'bundle.zip', 25, 1);
        expect(modals.openRenameModal).not.toHaveBeenCalled();
    });

    it('leaves removed listeners inactive and reactivation invokes each action once', () => {
        deactivate();
        press(row('folder:design'), 'Enter');
        drag(row('folder:design'), 'dragend');
        expect(actions.navigateToFolder).not.toHaveBeenCalled();
        expect(dragDrop.endRowDrag).not.toHaveBeenCalled();
        deactivate = activateFileList();
        press(row('folder:design'), 'Enter');
        expect(actions.navigateToFolder).toHaveBeenCalledTimes(1);
    });

    it('ignores live-item keyboard and click actions in the trash', () => {
        state.virtualView = 'trash';
        press(row('folder:design'), 'Enter');
        press(row('folder:design'), 'F2');
        press(row('folder:design'), 'Delete');
        row('file:41').click();
        drag(row('folder:design').querySelector('.row-name')!, 'dragstart');
        expect(actions.navigateToFolder).not.toHaveBeenCalled();
        expect(modals.openRenameModal).not.toHaveBeenCalled();
        expect(modals.openDeleteModal).not.toHaveBeenCalled();
        expect(dragDrop.beginRowDrag).not.toHaveBeenCalled();
        expect(state.selectedItems.size).toBe(0);
    });

    it('delegates search activation to the result callback', () => {
        state.searchQuery = 'plan';
        const activate = vi.fn();
        publish({ onDoubleClick: activate });
        press(row('file:41'), 'Enter');
        expect(activate).toHaveBeenCalledTimes(1);
        expect(actions.openFile).not.toHaveBeenCalled();
    });
});

describe('desktop keyboard focus', () => {
    it('moves between logical rows and returns from row actions', async () => {
        const folder = row('folder:design');
        const file = row('file:41');
        press(folder, 'ArrowDown');
        await vi.waitFor(() => expect(document.activeElement).toBe(file));
        press(file, 'Home');
        await vi.waitFor(() => expect(document.activeElement).toBe(folder));
        press(folder, 'End');
        await vi.waitFor(() => expect(document.activeElement).toBe(file));
        press(file, 'ArrowUp');
        await vi.waitFor(() => expect(document.activeElement).toBe(folder));
        press(folder, 'ArrowRight');
        const button = folder.querySelector('button:not(.row-select)')!;
        expect(document.activeElement).toBe(button);
        press(button, 'Escape');
        await vi.waitFor(() => expect(document.activeElement).toBe(folder));
    });

    it('selects with Space while leaving text editors and action buttons alone', () => {
        press(row('file:41'), ' ');
        expect(state.selectedItems.has('file:41')).toBe(true);
        const input = document.createElement('input');
        row('file:41').append(input);
        press(input, 'Delete');
        press(row('file:41').querySelector('button:not(.row-select)')!, 'Delete');
        expect(modals.openDeleteModal).not.toHaveBeenCalled();
    });

    it('opens the context menu at the row for keyboard users', () => {
        const listener = vi.fn();
        row('file:41').addEventListener('contextmenu', listener);
        press(row('file:41'), 'ContextMenu');
        row('file:41').dispatchEvent(new KeyboardEvent('keydown', { key: 'F10', shiftKey: true, bubbles: true }));
        expect(listener).toHaveBeenCalledTimes(2);
    });
});

describe('list semantics', () => {
    it('is a grid for rows and a labelled group for a state message', () => {
        expect(list.getAttribute('role')).toBe('grid');
        expect(list.getAttribute('aria-colcount')).toBe('4');
        renderFileState(list, 'loading', 'Loading files');
        flushSync();
        expect(list.getAttribute('role')).toBe('group');
        expect(list.hasAttribute('aria-colcount')).toBe(false);
    });

    it('announces a file row with its size and date, keeping the File: prefix', () => {
        renderFileListRows(list, [buildFileRow({ id: 7, name: 'report.pdf', size: 2048, date: 1_700_000_000 }, '')]);
        flushSync();
        const label = row('file:7').getAttribute('aria-label') ?? '';
        expect(label.startsWith('File: report.pdf')).toBe(true);
        expect(label).toContain('2 KB');
        expect(label.split(',').length).toBeGreaterThanOrEqual(3);
    });
});

describe('desktop keyboard shortcuts', () => {
    it('selects the whole folder with the select-all accelerator', () => {
        press(row('folder:design'), 'a'); // no modifier: type-ahead, not select-all
        expect(state.selectedItems.size).toBe(0);
        row('folder:design').dispatchEvent(new KeyboardEvent('keydown', { key: 'a', ctrlKey: true, bubbles: true, cancelable: true }));
        expect(state.selectedItems.size).toBe(2);
    });

    it('does not select the folder from the trash', () => {
        state.virtualView = 'trash';
        row('folder:design').dispatchEvent(new KeyboardEvent('keydown', { key: 'a', ctrlKey: true, bubbles: true, cancelable: true }));
        expect(state.selectedItems.size).toBe(0);
    });

    it('toggles a row from its checkbox like Ctrl+click, and never opens it', () => {
        startSelectionMode();
        flushSync();
        const check = row('file:41').querySelector<HTMLElement>('[data-testid="row-select"]')!;
        expect(check.getAttribute('role')).toBe('checkbox');
        expect(check.getAttribute('aria-label')).toBe('Select plan.pdf');
        expect(check.getAttribute('aria-checked')).toBe('false');

        check.dispatchEvent(new MouseEvent('click', { bubbles: true }));
        flushSync();
        expect(state.selectedItems.has('file:41')).toBe(true);
        expect(check.getAttribute('aria-checked')).toBe('true');

        // A double click on the box is a toggle, not an open.
        check.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
        flushSync();
        expect(actions.enqueueDownload).not.toHaveBeenCalled();
        expect(actions.preview).not.toHaveBeenCalled();

        // Unticking the last row ends the mode, so the box goes with it.
        check.dispatchEvent(new MouseEvent('click', { bubbles: true }));
        flushSync();
        expect(state.selectedItems.has('file:41')).toBe(false);
        expect(list.querySelector('[data-testid="row-select"]')).toBeNull();
    });

    it('keeps the other rows selected when a checkbox toggles one', () => {
        startSelectionMode();
        flushSync();
        row('file:41').querySelector<HTMLElement>('[data-testid="row-select"]')!.dispatchEvent(new MouseEvent('click', { bubbles: true }));
        row('folder:design').querySelector<HTMLElement>('[data-testid="row-select"]')!.dispatchEvent(new MouseEvent('click', { bubbles: true }));
        flushSync();
        expect(state.selectedItems.size).toBe(2);
    });

    it('extends the selection with Shift and an arrow', () => {
        row('folder:design').dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', shiftKey: true, bubbles: true, cancelable: true }));
        expect(state.selectedItems.has('folder:design')).toBe(true);
        expect(state.selectedItems.has('file:41')).toBe(true);
    });

    it('jumps to the next row whose name starts with the typed character', async () => {
        press(row('folder:design'), 'p');
        await vi.waitFor(() => expect(document.activeElement).toBe(row('file:41')));
    });

    it('goes up to the parent folder on the platform accelerator', () => {
        // metaKey and altKey both set so the test holds whichever this platform uses.
        row('file:41').dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', metaKey: true, altKey: true, bubbles: true, cancelable: true }));
        expect(actions.navigateBack).toHaveBeenCalledTimes(1);
    });
});

describe('desktop drag target safety', () => {
    it('does not commit a drop rejected by the folder permission check', () => {
        state.dragState = { items: [], parentId: '', blocked: new Set(), row: row('file:41') };
        dragDrop.canDropOnFolder.mockReturnValue(false);
        drag(row('folder:design'), 'dragover');
        drag(row('folder:design'), 'drop');
        expect(dragDrop.setDropHighlight).toHaveBeenCalledWith(row('folder:design'), false);
        expect(dragDrop.performDropMove).not.toHaveBeenCalled();
    });

    it('clears drop highlighting only when leaving the folder itself', () => {
        const folder = row('folder:design');
        state.dragOverEl = folder;
        folder.classList.add('drop-target');
        folder.dispatchEvent(new MouseEvent('dragleave', { bubbles: true, relatedTarget: folder.querySelector('.row-name') }));
        expect(folder.classList.contains('drop-target')).toBe(true);
        folder.dispatchEvent(new MouseEvent('dragleave', { bubbles: true, relatedTarget: list }));
        expect(folder.classList.contains('drop-target')).toBe(false);
        expect(state.dragOverEl).toBeNull();
    });
});


it('drags a multi-selection and blocks selected folders and descendants', async () => {
    row('folder:design').click();
    row('file:41').dispatchEvent(new MouseEvent('click', { bubbles: true, ctrlKey: true }));
    expect(state.selectedItems.size).toBe(2);
    dragDrop.beginRowDrag.mockImplementationOnce((element, items, parentId, blocked) => {
        state.dragState = { row: element, items, parentId, blocked };
    });
    vi.mocked(collectDescendants).mockReturnValueOnce(new Set(['child']));
    drag(row('folder:design').querySelector('.row-name')!, 'dragstart');
    await vi.waitFor(() => expect(state.dragState?.blocked.has('child')).toBe(true));
    expect(state.dragState?.blocked.has('design')).toBe(true);
    expect(state.dragState?.items.map((item) => item.id)).toEqual(['design', 41]);
});

it('does not apply late folder descendants after a drag has ended', async () => {
    let resolve!: (index: Awaited<ReturnType<typeof refreshFolderIndex>>) => void;
    vi.mocked(refreshFolderIndex).mockReturnValueOnce(new Promise((done) => { resolve = done; }));
    dragDrop.beginRowDrag.mockImplementationOnce((element, items, parentId, blocked) => {
        state.dragState = { row: element, items, parentId, blocked };
    });
    drag(row('folder:design').querySelector('.row-name')!, 'dragstart');
    state.dragState = null;
    resolve({ folders: [], byId: new Map(), children: new Map() });
    await Promise.resolve();
    expect(state.dragState).toBeNull();
    expect(collectDescendants).not.toHaveBeenCalled();
});

it('deletes a folder and renames a Telegram file with their original identifiers', () => {
    press(row('folder:design'), 'Backspace');
    expect(modals.openDeleteModal).toHaveBeenCalledWith({ type: 'folder', id: 'design', name: 'Design', parentId: '', channelId: 1 });
    renderFileListRows(list, [buildFileRow({ id: 99, name: 'raw.zip', source: 'tg', size: 300 }, 'origin')]);
    flushSync();
    press(row('file:99'), 'F2');
    expect(modals.openRenameModal).toHaveBeenCalledWith({ type: 'file', id: 99, name: 'raw.zip', size: 300, source: 'tg', parentId: 'origin' });
});

it('never renames a read-only archive on double click', () => {
    renderFileListRows(list, [buildFileRow({ id: 41, name: 'archive.zip', size: 10, canRename: false }, '')]);
    flushSync();
    row('file:41').querySelector('.row-name')?.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
    expect(modals.openRenameModal).not.toHaveBeenCalled();
    expect(actions.enqueueDownload).toHaveBeenCalledTimes(1);
});

it('supports keyboard navigation from the list container without a focused row', async () => {
    press(list, 'End');
    await vi.waitFor(() => expect(document.activeElement).toBe(row('file:41')));
    press(list, 'Home');
    await vi.waitFor(() => expect(document.activeElement).toBe(row('folder:design')));
});

it('does not consume unrelated keys', () => {
    publish({ actions: [] });
    for (const key of ['F10', 'Tab']) {
        const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true });
        row('file:41').dispatchEvent(event);
        expect(event.defaultPrevented).toBe(false);
    }
});

it('reaches the More button with ArrowRight even with no quick actions', () => {
    publish({ actions: [] });
    press(row('file:41'), 'ArrowRight');
    expect(document.activeElement).toBe(row('file:41').querySelector('button.row-more'));
});

it('opens the row menu from the three-dot button', () => {
    const listener = vi.fn();
    row('file:41').addEventListener('contextmenu', listener);
    row('file:41').querySelector<HTMLButtonElement>('button.row-more')?.click();
    expect(listener).toHaveBeenCalledTimes(1);
});

it('uses native drag feedback and clears highlight when a drop is committed', () => {
    const source = row('file:41');
    const folder = row('folder:design');
    const dataTransfer = { effectAllowed: '', dropEffect: '', types: ['text/plain'], setData: vi.fn() };
    const start = new Event('dragstart', { bubbles: true, cancelable: true });
    Object.defineProperty(start, 'dataTransfer', { value: dataTransfer });
    source.querySelector('.row-name')!.dispatchEvent(start);
    expect(dataTransfer.effectAllowed).toBe('move');
    expect(dataTransfer.setData).toHaveBeenCalledWith('text/plain', 'tdrive-move');
    state.dragState = { items: [], parentId: '', blocked: new Set(), row: source };
    for (const allowed of [false, true]) {
        dragDrop.canDropOnFolder.mockReturnValue(allowed);
        const over = new Event('dragover', { bubbles: true, cancelable: true });
        Object.defineProperty(over, 'dataTransfer', { value: dataTransfer });
        folder.dispatchEvent(over);
        expect(dataTransfer.dropEffect).toBe(allowed ? 'move' : 'none');
        expect(over.defaultPrevented).toBe(allowed);
    }
    folder.classList.add('drop-target');
    state.dragOverEl = folder;
    drag(folder, 'drop');
    expect(state.dragOverEl).toBeNull();
    expect(folder.classList.contains('drop-target')).toBe(false);
    expect(dragDrop.performDropMove).toHaveBeenCalledWith('design');
});

it('does not handle external drag events as internal moves', () => {
    drag(row('folder:design'), 'dragover');
    drag(row('folder:design'), 'drop');
    expect(dragDrop.canDropOnFolder).not.toHaveBeenCalled();
    expect(dragDrop.performDropMove).not.toHaveBeenCalled();
});

it('does not treat a file or the list background as a folder drop target', () => {
    state.dragState = { items: [], parentId: '', blocked: new Set(), row: row('file:41') };
    drag(row('file:41'), 'drop');
    drag(list, 'drop');
    drag(list, 'dragleave');
    drag(list, 'dragstart');
    expect(dragDrop.performDropMove).not.toHaveBeenCalled();
    expect(dragDrop.beginRowDrag).not.toHaveBeenCalled();
});

describe('selection mode on the desktop list', () => {
    it('a plain click selects only the row it lands on and does not enter selection mode', () => {
        row('file:41').dispatchEvent(new MouseEvent('click', { bubbles: true }));
        row('folder:design').dispatchEvent(new MouseEvent('click', { bubbles: true }));
        flushSync();
        expect(state.selectedItems.size).toBe(1);
        expect(state.selectedItems.has('folder:design')).toBe(true);
        expect(get(selectionModeOn)).toBe(false);
        expect(list.querySelector('[data-testid="row-select"]')).toBeNull();
    });

    it('a press and hold for 400 ms enters selection mode with that row selected', () => {
        vi.useFakeTimers();
        pointer('pointerdown', row('file:41'), { clientX: 10, clientY: 10 });
        vi.advanceTimersByTime(399);
        expect(state.selectedItems.size).toBe(0);
        vi.advanceTimersByTime(1);
        flushSync();
        expect(state.selectedItems.has('file:41')).toBe(true);
        expect(get(selectionModeOn)).toBe(true);
        expect(row('file:41').querySelector('[data-testid="row-select"]')).not.toBeNull();
        expect(row('folder:design').querySelector('[data-testid="row-select"]')).not.toBeNull();
    });

    it('a press released before 400 ms is a plain click and leaves the mode off', () => {
        vi.useFakeTimers();
        pointer('pointerdown', row('file:41'), { clientX: 10, clientY: 10 });
        vi.advanceTimersByTime(200);
        pointer('pointerup', row('file:41'), { clientX: 10, clientY: 10 });
        vi.advanceTimersByTime(500);
        expect(get(selectionModeOn)).toBe(false);
        expect(state.selectedItems.size).toBe(0);
    });

    it('moving more than 6 px before the hold completes cancels it', () => {
        vi.useFakeTimers();
        pointer('pointerdown', row('file:41'), { clientX: 10, clientY: 10 });
        pointer('pointermove', row('file:41'), { clientX: 10, clientY: 17 });
        vi.advanceTimersByTime(500);
        expect(get(selectionModeOn)).toBe(false);
        expect(state.selectedItems.size).toBe(0);
    });

    it('the click that ends a completed hold neither opens the row nor clears the selection', () => {
        vi.useFakeTimers();
        pointer('pointerdown', row('file:41'), { clientX: 10, clientY: 10 });
        vi.advanceTimersByTime(400);
        pointer('pointerup', row('file:41'), { clientX: 10, clientY: 10 });
        row('file:41').dispatchEvent(new MouseEvent('click', { bubbles: true }));
        flushSync();
        expect(actions.preview).not.toHaveBeenCalled();
        expect(actions.enqueueDownload).not.toHaveBeenCalled();
        expect(state.selectedItems.has('file:41')).toBe(true);
        expect(get(selectionModeOn)).toBe(true);
    });

    it('a plain click in selection mode toggles the row', () => {
        startSelectionMode();
        row('file:41').dispatchEvent(new MouseEvent('click', { bubbles: true }));
        row('folder:design').dispatchEvent(new MouseEvent('click', { bubbles: true }));
        flushSync();
        expect(state.selectedItems.size).toBe(2);
        row('file:41').dispatchEvent(new MouseEvent('click', { bubbles: true }));
        row('folder:design').dispatchEvent(new MouseEvent('click', { bubbles: true }));
        flushSync();
        // Unticking the last row ends the mode.
        expect(state.selectedItems.size).toBe(0);
        expect(get(selectionModeOn)).toBe(false);
    });

    it('Ctrl+A enters selection mode', () => {
        row('folder:design').dispatchEvent(new KeyboardEvent('keydown', { key: 'a', ctrlKey: true, bubbles: true, cancelable: true }));
        flushSync();
        expect(get(selectionModeOn)).toBe(true);
    });

    it('Clear leaves the mode and empties the selection', () => {
        startSelectionMode();
        state.selectedItems.set('file:41', { type: 'file', id: 41, name: 'plan.pdf', channelId: 1, size: 1, source: 'fs', parentId: '', canDelete: true, canRename: true } as never);
        clearSelection();
        flushSync();
        expect(get(selectionModeOn)).toBe(false);
        expect(state.selectedItems.size).toBe(0);
    });
});

describe('desktop actions column and new folder row', () => {
    it('gives every row three places: Open or none, Download or none, and More', () => {
        const slots = row('file:41').querySelectorAll(':scope > .row-actions > *');
        expect(slots.length).toBe(3);
        expect(row('file:41').querySelector('button.row-more')).not.toBeNull();
        expect(row('folder:design').querySelector('button.open-folder')).not.toBeNull();
        expect(row('folder:design').querySelectorAll(':scope > .row-actions > *').length).toBe(3);
    });

    it('shows no checkbox in normal browsing', () => {
        expect(list.querySelector('[data-testid="row-select"]')).toBeNull();
    });

    it('places the new folder row first in the list, with the name selected and no rows overlapping it', () => {
        start_inline_folder_create();
        flushSync();
        const input = list.querySelector<HTMLInputElement>('[data-testid="new-folder-input"]')!;
        expect(input.value).toBe('New folder');
        expect(input.selectionStart).toBe(0);
        expect(input.selectionEnd).toBe('New folder'.length);
        const draft = input.closest<HTMLElement>('.drive-row')!;
        expect(list.querySelector('.drive-row')).toBe(draft);
        expect(draft.compareDocumentPosition(row('folder:design')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
        expect(draft.textContent).toContain('Enter to create · Esc to cancel');
    });

    it('Enter creates the folder through the shared flow and closes the row', () => {
        start_inline_folder_create();
        flushSync();
        const input = list.querySelector<HTMLInputElement>('[data-testid="new-folder-input"]')!;
        input.value = 'anime';
        input.dispatchEvent(new Event('input', { bubbles: true }));
        input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }));
        flushSync();
        expect(modals.submitFolder).toHaveBeenCalledWith('anime');
        expect(list.querySelector('[data-testid="new-folder-input"]')).toBeNull();
    });

    it('Escape cancels without creating anything', () => {
        start_inline_folder_create();
        flushSync();
        const input = list.querySelector<HTMLInputElement>('[data-testid="new-folder-input"]')!;
        input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
        flushSync();
        expect(modals.submitFolder).not.toHaveBeenCalled();
        expect(list.querySelector('[data-testid="new-folder-input"]')).toBeNull();
        expect(modals.openNewFolderModal).not.toHaveBeenCalled();
    });
});
