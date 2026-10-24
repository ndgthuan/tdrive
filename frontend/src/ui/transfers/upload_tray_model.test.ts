// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { describe, expect, it } from 'vitest';
import {
    begin_group,
    cancel_item,
    clear_finished,
    empty_model,
    eta_seconds,
    extension_of,
    finish_upload,
    format_clock,
    group_complete,
    group_progress,
    group_rows,
    group_started,
    group_total,
    next_speed,
    progress_upload,
    register_batch,
    requeue,
    resumable_changed,
    resumable_progress,
    start_upload,
    summarize,
    MAX_TRAY_ITEMS,
    type RegisterEntry,
    type UploadTrayModel,
} from './upload_tray_model';

const entry = (name: string, overrides: Partial<RegisterEntry> = {}): RegisterEntry => ({
    path: `/tmp/${name}`,
    name,
    parent_id: 'root',
    encrypt: false,
    drive_id: 7,
    ...overrides,
});

/** Registers two files and starts the first, the state most tests begin from. */
function two_file_batch(now = 1_000): UploadTrayModel {
    let model = register_batch(empty_model(), [entry('a.txt'), entry('b.pdf')], now);
    model = start_upload(model, { id: 0, name: 'a.txt', size: 1000, parent_id: 'root' }, now).model;
    return model;
}

describe('batch registration', () => {
    it('makes one queued row per file, with retry details, before anything starts', () => {
        const model = register_batch(empty_model(), [entry('a.txt'), entry('b.pdf', { drive_id: null })], 0);
        expect(model.items.map((item) => [item.name, item.state])).toEqual([
            ['a.txt', 'queued'],
            ['b.pdf', 'queued'],
        ]);
        expect(model.items[0].retry).toEqual({ path: '/tmp/a.txt', parent_id: 'root', encrypt: false, drive_id: 7 });
        // No drive id means the retry call cannot be made correctly, so no retry is offered.
        expect(model.items[1].retry).toBeNull();
    });

    it('binds the backend start for upload id i to registered row i', () => {
        const model = two_file_batch();
        expect(model.items).toHaveLength(2);
        expect(model.items[0]).toMatchObject({ name: 'a.txt', state: 'uploading', size: 1000, upload_id: 0 });
        expect(model.items[1].state).toBe('queued');
        expect(model.bound).toEqual({ '0': model.items[0].key });
    });

    it('makes a row for an upload nobody registered', () => {
        const model = start_upload(empty_model(), { id: 3, name: 'photo.jpg', size: 42, parent_id: '' }, 0).model;
        expect(model.items).toHaveLength(1);
        expect(model.items[0]).toMatchObject({ name: 'photo.jpg', state: 'uploading', upload_id: 3 });
    });
});

describe('progress and speed', () => {
    it('moves percent forward only and estimates bytes from it', () => {
        let model = two_file_batch(0);
        model = progress_upload(model, 0, 40, 1000);
        model = progress_upload(model, 0, 25, 1100);
        expect(model.items[0].percent).toBe(40);
        expect(model.items[0].sent).toBe(400);
    });

    it('derives speed from the change in bytes over the elapsed time', () => {
        let model = two_file_batch(0);
        model = progress_upload(model, 0, 50, 1000);
        // 500 bytes in one second.
        expect(model.items[0].speed).toBeCloseTo(500, 5);
    });

    it('does not read a rate from a report that lands moments after the start', () => {
        let model = two_file_batch(0);
        model = progress_upload(model, 0, 30, 5);
        expect(model.items[0].speed).toBe(0);
        // The bytes accrue until the sample gap is long enough, then count in full.
        model = progress_upload(model, 0, 50, 1000);
        expect(model.items[0].speed).toBeCloseTo(500, 5);
    });

    it('ignores progress for ids the tray does not know', () => {
        const model = two_file_batch(0);
        expect(progress_upload(model, 99, 80, 1000)).toBe(model);
    });

    it('smooths speed toward new samples instead of jumping to them', () => {
        expect(next_speed(0, 1000, 1000)).toBe(1000);
        expect(next_speed(1000, 3000, 1000)).toBeCloseTo(1000 + (3000 - 1000) * 0.35, 5);
        expect(next_speed(500, -1, 1000)).toBe(500);
        expect(next_speed(500, 10, 0)).toBe(500);
    });
});

describe('state transitions', () => {
    it('finishes a file as done with full percent', () => {
        let model = two_file_batch(0);
        model = finish_upload(model, { id: 0, name: 'a.txt', outcome: 'done', error: '' }, 10);
        expect(model.items[0]).toMatchObject({ state: 'done', percent: 100, sent: 1000, speed: 0 });
        expect(model.bound).toEqual({});
    });

    it('keeps the backend reason on a failed row and offers no speed', () => {
        let model = two_file_batch(0);
        model = finish_upload(model, { id: 0, name: 'a.txt', outcome: 'failed', error: 'Network dropped' }, 10);
        expect(model.items[0]).toMatchObject({ state: 'failed', error: 'Network dropped', speed: 0 });
    });

    it('keeps a cancelled row cancelled when the backend reports the failure that follows', () => {
        let model = two_file_batch(0);
        model = cancel_item(model, model.items[0].key);
        model = finish_upload(model, { id: 0, name: 'a.txt', outcome: 'failed', error: 'canceled' }, 10);
        expect(model.items[0].state).toBe('canceled');
    });

    it('reads a file that finished before the cancel reached it as done', () => {
        let model = two_file_batch(0);
        model = cancel_item(model, model.items[0].key);
        model = finish_upload(model, { id: 0, name: 'a.txt', outcome: 'done', error: '' }, 10);
        expect(model.items[0].state).toBe('done');
    });

    it('asks the caller to stop a queued row that was cancelled before its upload started', () => {
        let model = register_batch(empty_model(), [entry('late.txt')], 0);
        model = cancel_item(model, model.items[0].key);
        const started = start_upload(model, { id: 0, name: 'late.txt', size: 9, parent_id: 'root' }, 5);
        expect(started.cancel).toBe(true);
        expect(started.model.items).toHaveLength(1);
        expect(started.model.items[0].state).toBe('canceled');
    });

    it('requeues a failed row into a one-item batch under its own key', () => {
        let model = two_file_batch(0);
        model = finish_upload(model, { id: 0, name: 'a.txt', outcome: 'failed', error: 'boom' }, 10);
        const key = model.items[0].key;
        model = requeue(model, key, 20);
        expect(model.items).toHaveLength(2);
        expect(model.items[0]).toMatchObject({ key, state: 'queued', error: '', percent: 0 });
        expect(model.batch).toEqual([key]);
        const started = start_upload(model, { id: 0, name: 'a.txt', size: 1000, parent_id: 'root' }, 30);
        expect(started.key).toBe(key);
        expect(started.model.items).toHaveLength(2);
    });
});

describe('folder import rows', () => {
    it('shows the folder as one row from the moment it begins', () => {
        const model = begin_group(empty_model(), 'Holiday', 0);
        expect(model.items).toHaveLength(1);
        expect(model.items[0]).toMatchObject({ kind: 'folder', name: 'Holiday', state: 'queued' });
    });

    it('does not start a second folder row while one is in flight', () => {
        const first = begin_group(empty_model(), 'A', 0);
        expect(begin_group(first, 'B', 1)).toBe(first);
    });

    it('counts files as the backend reports them and settles on complete', () => {
        let model = begin_group(empty_model(), 'Holiday', 0);
        model = group_started(model, 1);
        model = group_total(model, 10);
        model = group_progress(model, { total: 10, done: 3, failed: 1, progress: 40, bytes: 4000 }, 2000);
        const row = model.items[0];
        expect(row).toMatchObject({ state: 'uploading', files_total: 10, files_done: 3, files_failed: 1, percent: 40 });
        model = group_complete(model, { status: 'done', uploaded: 9, failed: 1, error: '' }, 3000);
        expect(model.items[0]).toMatchObject({ state: 'done', files_done: 9, files_failed: 1, percent: 100 });
        expect(model.group_key).toBeNull();
    });

    it('ignores group events when no folder row is running', () => {
        const model = empty_model();
        expect(group_started(model, 0)).toBe(model);
        expect(group_progress(model, { total: 1, done: 0, failed: 0, progress: 1, bytes: 1 }, 0)).toBe(model);
    });
});

describe('resumable jobs', () => {
    const view = (status: string, extra: Partial<Parameters<typeof resumable_changed>[1]> = {}) => ({
        job_id: 'job-1',
        name: 'movie.mkv',
        size: 4_000,
        confirmed_bytes: 1_000,
        status,
        error: '',
        ...extra,
    });

    it('maps stored statuses onto row states', () => {
        const states: Array<[string, string]> = [
            ['uploading', 'uploading'],
            ['waiting_network', 'uploading'],
            ['paused', 'queued'],
            ['completed', 'done'],
            ['canceling', 'canceled'],
            ['needs_source', 'failed'],
        ];
        for (const [status, expected] of states) {
            const model = resumable_changed(empty_model(), view(status, { error: 'why' }), 0);
            expect([status, model.items[0].state]).toEqual([status, expected]);
        }
    });

    it('leaves jobs that belong to a live upload to the upload events', () => {
        const model = empty_model();
        expect(resumable_changed(model, view('uploading', { upload_id: 4 }), 0)).toBe(model);
    });

    it('shows byte progress from the resume events', () => {
        let model = resumable_changed(empty_model(), view('uploading'), 0);
        model = resumable_progress(model, 'job-1', 2_000, 4_000, 1000);
        expect(model.items[0]).toMatchObject({ key: 'job:job-1', name: 'movie.mkv', percent: 50, sent: 2000, state: 'uploading' });
    });
});

describe('summary and grouping', () => {
    it('reports uploading while anything is queued or moving', () => {
        const summary = summarize(two_file_batch());
        expect(summary).toMatchObject({ phase: 'uploading', count: 2, finished: false, in_flight: 2 });
    });

    it('reports complete once everything is done', () => {
        let model = two_file_batch(0);
        model = finish_upload(model, { id: 0, name: 'a.txt', outcome: 'done', error: '' }, 1);
        model = finish_upload(model, { id: 1, name: 'b.pdf', outcome: 'done', error: '' }, 2);
        expect(summarize(model)).toMatchObject({ phase: 'complete', finished: true, done: 2, failed: 0 });
    });

    it('counts failures in the title once nothing else is in flight', () => {
        let model = two_file_batch(0);
        model = finish_upload(model, { id: 0, name: 'a.txt', outcome: 'failed', error: 'x' }, 1);
        model = finish_upload(model, { id: 1, name: 'b.pdf', outcome: 'failed', error: 'y' }, 2);
        expect(summarize(model)).toMatchObject({ phase: 'failed', count: 2, failed: 2, finished: true });
    });

    it('reports canceled when every finished row was cancelled', () => {
        let model = two_file_batch(0);
        model = cancel_item(model, model.items[0].key);
        model = cancel_item(model, model.items[1].key);
        expect(summarize(model).phase).toBe('canceled');
    });

    it('is empty with no rows', () => {
        expect(summarize(empty_model())).toMatchObject({ phase: 'empty', finished: false });
    });

    it('orders rows as active, then failed, then finished', () => {
        let model = register_batch(empty_model(), [entry('done.txt'), entry('bad.txt'), entry('live.txt'), entry('wait.txt')], 0);
        model = start_upload(model, { id: 0, name: 'done.txt', size: 1, parent_id: '' }, 0).model;
        model = finish_upload(model, { id: 0, name: 'done.txt', outcome: 'done', error: '' }, 1);
        model = start_upload(model, { id: 1, name: 'bad.txt', size: 1, parent_id: '' }, 2).model;
        model = finish_upload(model, { id: 1, name: 'bad.txt', outcome: 'failed', error: 'x' }, 3);
        model = start_upload(model, { id: 2, name: 'live.txt', size: 1, parent_id: '' }, 4).model;
        const grouped = group_rows(model.items);
        expect(grouped.active.map((item) => item.name)).toEqual(['live.txt', 'wait.txt']);
        expect(grouped.failed.map((item) => item.name)).toEqual(['bad.txt']);
        expect(grouped.finished.map((item) => item.name)).toEqual(['done.txt']);
    });

    it('clears finished rows and keeps everything still moving', () => {
        let model = two_file_batch(0);
        model = finish_upload(model, { id: 0, name: 'a.txt', outcome: 'done', error: '' }, 1);
        const cleared = clear_finished(model);
        expect(cleared.items.map((item) => item.name)).toEqual(['b.pdf']);
    });

    it('bounds the list by dropping the oldest finished rows first', () => {
        const entries = Array.from({ length: MAX_TRAY_ITEMS + 5 }, (_, index) => entry(`f${index}.txt`));
        let model = register_batch(empty_model(), entries.slice(0, 5), 0);
        for (let index = 0; index < 5; index += 1) {
            model = finish_upload(model, { id: index, name: `f${index}.txt`, outcome: 'done', error: '' }, index);
        }
        model = register_batch(model, entries.slice(5), 1);
        expect(model.items.length).toBeLessThanOrEqual(MAX_TRAY_ITEMS);
        expect(model.items.some((item) => item.state === 'queued')).toBe(true);
    });
});

describe('formatting helpers', () => {
    it('formats seconds as a clock, with hours only when needed', () => {
        expect(format_clock(0)).toBe('0:00');
        expect(format_clock(42)).toBe('0:42');
        expect(format_clock(185)).toBe('3:05');
        expect(format_clock(3729)).toBe('1:02:09');
        expect(format_clock(-5)).toBe('0:00');
    });

    it('estimates time left only for an uploading file with a size and a speed', () => {
        expect(eta_seconds({ state: 'uploading', size: 1000, sent: 250, speed: 250 })).toBe(3);
        expect(eta_seconds({ state: 'uploading', size: 1000, sent: 250, speed: 0 })).toBeNull();
        expect(eta_seconds({ state: 'uploading', size: 0, sent: 0, speed: 10 })).toBeNull();
        expect(eta_seconds({ state: 'queued', size: 1000, sent: 0, speed: 10 })).toBeNull();
    });

    it('reads the extension for the icon, and nothing for dotfiles or bare names', () => {
        expect(extension_of('Report.final.PDF')).toBe('PDF');
        expect(extension_of('.env')).toBe('');
        expect(extension_of('README')).toBe('');
        expect(extension_of('trailing.')).toBe('');
    });
});

describe('folder group completion', () => {
    it('marks a group failed when the backend says so, even with some files uploaded', () => {
        let model = begin_group(empty_model(), 'Archive', 0);
        model = group_complete(model, { status: 'failed', uploaded: 3, failed: 2, error: 'disk full' }, 1);
        expect(model.items[0]).toMatchObject({ state: 'failed', error: 'disk full', files_done: 3, files_failed: 2 });
    });
});
