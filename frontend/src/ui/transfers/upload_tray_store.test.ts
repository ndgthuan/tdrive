import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

type Handler = (...args: unknown[]) => void;

const runtime = vi.hoisted(() => ({
    handlers: new Map<string, Handler>(),
    cancelUpload: vi.fn(async () => undefined),
    cancelUploadById: vi.fn(async () => undefined),
    retryUploadToDriveFs: vi.fn(async () => ({ result: { ok: true } })),
    refreshFiles: vi.fn(),
}));

vi.mock('../../api', () => ({
    onRuntimeEvent: (name: string, handler: Handler) => {
        runtime.handlers.set(name, handler);
        return () => runtime.handlers.delete(name);
    },
    cancelUpload: runtime.cancelUpload,
    cancelUploadById: runtime.cancelUploadById,
    retryUploadToDriveFs: runtime.retryUploadToDriveFs,
}));

vi.mock('../../modules/app_actions', () => ({
    appActions: () => ({ refreshFiles: runtime.refreshFiles }),
}));

vi.mock('../../modules/errors', () => ({
    humanizeBackendError: (error: unknown) => {
        if (typeof error === 'string') return error;
        if (error && typeof error === 'object' && 'message' in error) return String((error as { message: unknown }).message);
        return 'Upload failed';
    },
}));

import {
    abandon_folder_upload,
    activate_upload_tray,
    begin_folder_upload,
    cancel_tray_item,
    dismiss_finished,
    expect_upload_batch,
    reset_upload_tray,
    retry_tray_item,
    set_tray_hovered,
    settle_upload_batch,
    TRAY_FADE_MS,
    tray_collapsed,
    upload_tray,
} from './upload_tray_store';

const emit = (name: string, ...args: unknown[]) => {
    const handler = runtime.handlers.get(name);
    if (!handler) throw new Error(`no subscriber for ${name}`);
    handler(...args);
};

function snapshot() {
    return get(upload_tray);
}

function register(names: string[]) {
    expect_upload_batch(names.map((name) => ({
        path: `/src/${name}`,
        name,
        parent_id: 'root',
        encrypt: false,
        drive_id: 3,
    })));
}

let teardown: () => void;

beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000_000);
    runtime.handlers.clear();
    vi.clearAllMocks();
    reset_upload_tray();
    teardown = activate_upload_tray();
});

afterEach(() => {
    teardown();
    reset_upload_tray();
    vi.useRealTimers();
});

describe('upload tray store', () => {
    it('shows the rows as soon as a batch is registered, before any event', () => {
        expect(snapshot().summary.phase).toBe('empty');
        register(['a.txt', 'b.txt']);
        expect(snapshot().summary).toMatchObject({ phase: 'uploading', count: 2 });
        expect(snapshot().items.map((item) => item.state)).toEqual(['queued', 'queued']);
    });

    it('follows the runtime events from start to completion and refreshes the file list', () => {
        register(['a.txt']);
        emit('upload_start', 0, 'a.txt', 400, 'root');
        expect(snapshot().items[0]).toMatchObject({ state: 'uploading', size: 400 });
        vi.advanceTimersByTime(1000);
        emit('upload_progress', 0, 50);
        expect(snapshot().items[0]).toMatchObject({ percent: 50, sent: 200 });
        expect(snapshot().items[0].speed).toBeGreaterThan(0);
        emit('upload_complete', 0, 'a.txt');
        expect(snapshot().items[0].state).toBe('done');
        vi.advanceTimersByTime(300);
        expect(runtime.refreshFiles).toHaveBeenCalledTimes(1);
    });

    it('folds to a complete card and fades it after the fade delay', () => {
        register(['a.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        emit('upload_complete', 0, 'a.txt');
        expect(get(tray_collapsed)).toBe(true);
        expect(snapshot().summary.phase).toBe('complete');
        vi.advanceTimersByTime(TRAY_FADE_MS - 1);
        expect(snapshot().items).toHaveLength(1);
        vi.advanceTimersByTime(1);
        expect(snapshot().items).toHaveLength(0);
    });

    it('keeps a finished card on screen while the pointer is over it', () => {
        register(['a.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        emit('upload_complete', 0, 'a.txt');
        set_tray_hovered(true);
        vi.advanceTimersByTime(TRAY_FADE_MS * 3);
        expect(snapshot().items).toHaveLength(1);
        set_tray_hovered(false);
        vi.advanceTimersByTime(TRAY_FADE_MS);
        expect(snapshot().items).toHaveLength(0);
    });

    it('never fades a card with a failure, and offers a retry for the failed file', () => {
        register(['a.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        emit('upload_error', 0, 'a.txt', 'Network dropped');
        expect(snapshot().summary.phase).toBe('failed');
        // Stays open so the Retry is in reach.
        expect(get(tray_collapsed)).toBe(false);
        vi.advanceTimersByTime(TRAY_FADE_MS * 5);
        expect(snapshot().items).toHaveLength(1);
        expect(snapshot().items[0]).toMatchObject({ state: 'failed', error: 'Network dropped' });
    });

    it('retries a failed file through the same call, reusing its row', () => {
        register(['a.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        emit('upload_error', 0, 'a.txt', 'Network dropped');
        const key = snapshot().items[0].key;
        retry_tray_item(key);
        expect(runtime.retryUploadToDriveFs).toHaveBeenCalledWith(['/src/a.txt'], ['root'], false, 3);
        expect(snapshot().items).toHaveLength(1);
        expect(snapshot().items[0]).toMatchObject({ key, state: 'queued' });
        emit('upload_start', 0, 'a.txt', 10, 'root');
        expect(snapshot().items[0]).toMatchObject({ key, state: 'uploading' });
    });

    it('marks the row failed when the retry call itself is refused', async () => {
        register(['a.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        emit('upload_error', 0, 'a.txt', 'first');
        runtime.retryUploadToDriveFs.mockRejectedValueOnce(new Error('offline'));
        retry_tray_item(snapshot().items[0].key);
        await vi.waitFor(() => expect(snapshot().items[0]).toMatchObject({ state: 'failed', error: 'offline' }));
    });

    it('cancels an in-flight file on the backend and marks it cancelled at once', () => {
        register(['a.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        cancel_tray_item(snapshot().items[0].key);
        expect(runtime.cancelUploadById).toHaveBeenCalledWith(0);
        expect(snapshot().items[0].state).toBe('canceled');
        // The backend's failure for the cancelled file does not turn it into a failure.
        emit('upload_error', 0, 'a.txt', 'canceled');
        expect(snapshot().items[0].state).toBe('canceled');
    });

    it('stops a queued file as soon as the backend starts it after it was cancelled', () => {
        register(['a.txt', 'b.txt']);
        cancel_tray_item(snapshot().items[1].key);
        emit('upload_start', 1, 'b.txt', 10, 'root');
        expect(runtime.cancelUploadById).toHaveBeenCalledWith(1);
    });

    it('shows a folder import as one row and counts its files', () => {
        begin_folder_upload('Holiday');
        expect(snapshot().items[0]).toMatchObject({ kind: 'folder', name: 'Holiday', state: 'queued' });
        emit('import_start');
        emit('import_uploading', { files: 5, folders: 2 });
        emit('import_upload_progress', { total: 5, done: 2, failed: 0, progress: 40, bytes: 200 });
        expect(snapshot().items[0]).toMatchObject({ state: 'uploading', files_total: 5, files_done: 2, percent: 40 });
        // Per-file events during an import are the aggregate's business, not rows of their own.
        emit('upload_start', 0, 'x.txt', 10, '');
        expect(snapshot().items).toHaveLength(1);
        emit('import_complete', { status: 'done', uploaded: 5, failed: 0 });
        expect(snapshot().items[0]).toMatchObject({ state: 'done', files_done: 5 });
    });

    it('settles a folder row as failed when the import never starts', () => {
        begin_folder_upload('Broken');
        abandon_folder_upload('Could not read the folder');
        expect(snapshot().items[0]).toMatchObject({ state: 'failed', error: 'Could not read the folder' });
    });

    it('settles rows the backend never reported when its batch call returns', () => {
        register(['a.txt', 'b.txt', 'c.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        settle_upload_batch({ failures: [{ index: 1, message: 'Receipt lost' }], canceled: false, threw: false });
        const states = snapshot().items.map((item) => item.state);
        expect(states).toEqual(['done', 'failed', 'done']);
        expect(snapshot().items[1].error).toBe('Receipt lost');
        vi.advanceTimersByTime(300);
        expect(runtime.refreshFiles).toHaveBeenCalled();
    });

    it('closes out a cancelled batch as cancelled', () => {
        register(['a.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        settle_upload_batch({ failures: [], canceled: true, threw: false });
        expect(snapshot().items[0].state).toBe('canceled');
    });

    it('lets the close control clear the finished rows, and nothing in flight', () => {
        register(['a.txt', 'b.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        emit('upload_complete', 0, 'a.txt');
        dismiss_finished();
        expect(snapshot().items.map((item) => item.name)).toEqual(['b.txt']);
    });

    it('unfolds the card again when new work starts after it finished', () => {
        register(['a.txt']);
        emit('upload_start', 0, 'a.txt', 10, '');
        emit('upload_complete', 0, 'a.txt');
        expect(get(tray_collapsed)).toBe(true);
        register(['b.txt']);
        expect(get(tray_collapsed)).toBe(false);
    });

    it('shows a finished upload it never saw start, and ignores progress for it', () => {
        emit('upload_progress', 42, 10);
        emit('upload_complete', 42, 'ghost.txt');
        expect(snapshot().items).toHaveLength(1);
        expect(snapshot().items[0]).toMatchObject({ state: 'done', name: 'ghost.txt' });
    });
});
