// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Pure state for the upload tray. Nothing here touches the DOM, the runtime or
// the stores: every transition takes the previous model and returns the next
// one, so the tray's behaviour can be read and tested as data.
//
// The tray is fed by the backend's per-file upload events (upload_start,
// upload_progress, upload_complete, upload_error), by the folder import events
// (import_*), and by resumable-job events. Plain multi-file selections are
// registered up front, so their rows exist -- as queued -- before the first
// byte moves, which is what makes the tray appear the moment an upload starts.

import { format_bytes } from '../../i18n/i18n';

export type TrayItemState = 'queued' | 'uploading' | 'done' | 'failed' | 'canceled';
export type TrayItemKind = 'file' | 'folder';

/** Everything needed to send one file again. Absent when the source is not known. */
export interface TrayRetry {
    path: string;
    parent_id: string;
    encrypt: boolean;
    drive_id: number;
}

export interface TrayItem {
    key: string;
    kind: TrayItemKind;
    name: string;
    /** Total bytes, or 0 when the backend has not said. */
    size: number;
    /** Bytes sent so far, estimated from percent for files. */
    sent: number;
    percent: number;
    state: TrayItemState;
    /** Smoothed bytes per second; 0 until two samples exist. */
    speed: number;
    error: string;
    /** Folder rows: files finished, files in total, and files that failed. */
    files_done: number;
    files_total: number;
    files_failed: number;
    /** The backend's id for an in-flight file. Null until upload_start binds it. */
    upload_id: number | null;
    retry: TrayRetry | null;
    /** When the last speed sample was taken. */
    sampled_at: number;
    /** Bytes sent at that sample, so the next sample measures only what moved since. */
    sampled_sent: number;
    /** Where the item came from. Resumable jobs have no upload id, so they cannot be cancelled here. */
    source: 'upload' | 'folder' | 'resumable';
}

export interface UploadTrayModel {
    items: TrayItem[];
    seq: number;
    /** upload id (as a string) -> item key, for uploads in flight. */
    bound: Record<string, string>;
    /** Item keys of the registered batch, indexed by the upload id the backend assigns. */
    batch: string[] | null;
    /** Key of the folder or archive import row, while one is running. */
    group_key: string | null;
}

export interface RegisterEntry {
    path: string;
    name: string;
    parent_id: string;
    encrypt: boolean;
    drive_id: number | null;
}

export interface UploadStart {
    id: number;
    name: string;
    size: number;
    parent_id: string;
}

export interface UploadFinish {
    id: number;
    name: string;
    outcome: 'done' | 'failed' | 'canceled';
    error: string;
}

export interface ImportProgressSample {
    total: number;
    done: number;
    failed: number;
    progress: number;
    bytes: number;
}

export interface ResumableView {
    job_id: string;
    name: string;
    size: number;
    confirmed_bytes: number;
    status: string;
    error: string;
    upload_id?: number | null;
}

/** Keeps the list bounded: the oldest finished rows go first once the tray is this full. */
export const MAX_TRAY_ITEMS = 300;

/** Smoothing for speed. Higher reacts faster; lower reads steadier. */
const SPEED_SMOOTHING = 0.35;

/**
 * Shortest gap between two speed samples. A first report that lands a few
 * milliseconds after the start would otherwise read as a rate of hundreds of
 * megabytes a second; bytes accrue until the gap is long enough to mean something.
 */
const MIN_SAMPLE_MS = 250;

/** Records bytes sent, and takes a speed sample once the sample gap has passed. */
function take_sample(item: TrayItem, sent: number, now: number): TrayItem {
    const elapsed = now - item.sampled_at;
    if (elapsed < MIN_SAMPLE_MS) return { ...item, sent };
    return {
        ...item,
        sent,
        speed: next_speed(item.speed, sent - item.sampled_sent, elapsed),
        sampled_at: now,
        sampled_sent: sent,
    };
}

export function empty_model(): UploadTrayModel {
    return { items: [], seq: 0, bound: {}, batch: null, group_key: null };
}

function next_key(model: UploadTrayModel, prefix: string): { key: string; seq: number } {
    const seq = model.seq + 1;
    return { key: `${prefix}-${seq}`, seq };
}

function blank_item(key: string, kind: TrayItemKind, source: TrayItem['source'], name: string, now: number): TrayItem {
    return {
        key,
        kind,
        name,
        size: 0,
        sent: 0,
        percent: 0,
        state: 'queued',
        speed: 0,
        error: '',
        files_done: 0,
        files_total: 0,
        files_failed: 0,
        upload_id: null,
        retry: null,
        sampled_at: now,
        sampled_sent: 0,
        source,
    };
}

function is_finished(state: TrayItemState): boolean {
    return state === 'done' || state === 'failed' || state === 'canceled';
}

export function is_in_flight(state: TrayItemState): boolean {
    return state === 'queued' || state === 'uploading';
}

function replace_item(model: UploadTrayModel, key: string, update: (item: TrayItem) => TrayItem): UploadTrayModel {
    let found = false;
    const items = model.items.map((item) => {
        if (item.key !== key) return item;
        found = true;
        return update(item);
    });
    return found ? { ...model, items } : model;
}

function find_item(model: UploadTrayModel, key: string | null): TrayItem | undefined {
    if (key === null) return undefined;
    return model.items.find((item) => item.key === key);
}

/**
 * Moves an item's speed estimate forward. Speed is the change in bytes over the
 * time since the last sample, blended into the previous value so one slow or
 * bursty report does not make the figure jump around.
 */
export function next_speed(previous: number, bytes_delta: number, elapsed_ms: number): number {
    if (!(elapsed_ms > 0) || bytes_delta < 0) return previous;
    const instant = (bytes_delta / elapsed_ms) * 1000;
    if (previous <= 0) return instant;
    return previous + (instant - previous) * SPEED_SMOOTHING;
}

function clamp_percent(value: unknown): number {
    const number = Number(value);
    if (!Number.isFinite(number)) return 0;
    return Math.max(0, Math.min(100, number));
}

function prune(model: UploadTrayModel): UploadTrayModel {
    if (model.items.length <= MAX_TRAY_ITEMS) return model;
    const overflow = model.items.length - MAX_TRAY_ITEMS;
    const kept: TrayItem[] = [];
    let dropped = 0;
    for (const item of model.items) {
        if (dropped < overflow && is_finished(item.state)) {
            dropped += 1;
            continue;
        }
        kept.push(item);
    }
    return { ...model, items: kept };
}

/**
 * Adds one queued row per file of a plain upload, before the backend has been
 * asked to start. Row i answers to upload id i, which is how the backend numbers
 * a single call's files.
 */
export function register_batch(model: UploadTrayModel, entries: RegisterEntry[], now: number): UploadTrayModel {
    let next = { ...model };
    const keys: string[] = [];
    const created: TrayItem[] = [];
    for (const entry of entries) {
        const { key, seq } = next_key(next, 'up');
        next = { ...next, seq };
        keys.push(key);
        const item = blank_item(key, 'file', 'upload', entry.name || 'Untitled', now);
        item.retry = entry.path && entry.drive_id !== null
            ? { path: entry.path, parent_id: entry.parent_id, encrypt: entry.encrypt, drive_id: entry.drive_id }
            : null;
        created.push(item);
    }
    next = { ...next, items: [...next.items, ...created], batch: keys };
    return prune(next);
}

/**
 * Sends a failed row back into the queue, reusing its row so the list does not
 * grow a duplicate. The row becomes the only member of the next batch.
 */
export function requeue(model: UploadTrayModel, key: string, now: number): UploadTrayModel {
    const bound = Object.fromEntries(Object.entries(model.bound).filter(([, value]) => value !== key));
    const next = replace_item({ ...model, bound, batch: [key] }, key, (item) => ({
        ...item,
        state: 'queued',
        error: '',
        percent: 0,
        sent: 0,
        speed: 0,
        upload_id: null,
        sampled_at: now,
    }));
    return next;
}

/**
 * Binds the backend's start event to a row. The batch row for that id wins when
 * one is registered and still unbound; otherwise a row is made for the file.
 * `cancel` is true when the row was cancelled while still queued, so the caller
 * can stop the upload the backend is about to run.
 */
export function start_upload(model: UploadTrayModel, start: UploadStart, now: number): { model: UploadTrayModel; key: string; cancel: boolean } {
    const registered = model.batch?.[start.id] ?? null;
    const candidate = find_item(model, registered);
    let next = model;
    let key: string;
    if (candidate && candidate.upload_id === null && !is_finished(candidate.state)) {
        key = candidate.key;
    } else if (candidate && candidate.upload_id === null && candidate.state === 'canceled') {
        key = candidate.key;
    } else {
        const created = next_key(model, 'up');
        key = created.key;
        next = {
            ...model,
            seq: created.seq,
            items: [...model.items, blank_item(key, 'file', 'upload', start.name || 'Untitled', now)],
        };
        next = prune(next);
    }
    const bound_item = find_item(next, key);
    const cancel = bound_item?.state === 'canceled';
    next = replace_item(next, key, (item) => {
        if (item.state === 'canceled') return { ...item, upload_id: start.id };
        return {
            ...item,
            name: item.name && item.name !== 'Untitled' ? item.name : (start.name || 'Untitled'),
            size: start.size > 0 ? start.size : item.size,
            state: 'uploading',
            upload_id: start.id,
            sampled_at: now,
            sampled_sent: 0,
        };
    });
    return { model: { ...next, bound: { ...next.bound, [String(start.id)]: key } }, key, cancel };
}

/** Folds one progress report into its row, with a speed sample from the change in bytes. */
export function progress_upload(model: UploadTrayModel, id: number, percent: number, now: number): UploadTrayModel {
    const key = model.bound[String(id)];
    if (!key) return model;
    return replace_item(model, key, (item) => {
        if (item.state !== 'uploading') return item;
        const value = Math.max(item.percent, clamp_percent(percent));
        const sent = item.size > 0 ? Math.round((item.size * value) / 100) : 0;
        return take_sample({ ...item, percent: value }, sent, now);
    });
}

/**
 * Settles a file's row. A cancelled row stays cancelled when the backend's
 * failure arrives, but a file that finished before the cancel reached it reads
 * as done, because that is what happened.
 */
export function finish_upload(model: UploadTrayModel, finish: UploadFinish, now: number): UploadTrayModel {
    const bound_key = model.bound[String(finish.id)];
    const registered = model.batch?.[finish.id] ?? null;
    let key = bound_key ?? null;
    if (key === null && registered !== null && find_item(model, registered)?.upload_id === null) key = registered;
    let next = model;
    if (key === null) {
        const created = next_key(model, 'up');
        key = created.key;
        const item = blank_item(key, 'file', 'upload', finish.name || 'Untitled', now);
        next = prune({ ...model, seq: created.seq, items: [...model.items, item] });
    }
    const bound = Object.fromEntries(Object.entries(next.bound).filter(([id]) => id !== String(finish.id)));
    next = { ...next, bound };
    return replace_item(next, key, (item) => {
        if (item.state === 'canceled' && finish.outcome !== 'done') return { ...item, upload_id: null };
        if (item.state === 'done' || item.state === 'failed') return item;
        const name = item.name && item.name !== 'Untitled' ? item.name : (finish.name || 'Untitled');
        if (finish.outcome === 'done') {
            return { ...item, name, state: 'done', percent: 100, sent: item.size, speed: 0, error: '', upload_id: null, sampled_at: now };
        }
        if (finish.outcome === 'canceled') {
            return { ...item, name, state: 'canceled', speed: 0, error: '', upload_id: null, sampled_at: now };
        }
        return { ...item, name, state: 'failed', speed: 0, error: finish.error, upload_id: null, sampled_at: now };
    });
}

/** Marks a row cancelled from the tray. The caller tells the backend when the row has an upload id. */
export function cancel_item(model: UploadTrayModel, key: string): UploadTrayModel {
    return replace_item(model, key, (item) => {
        if (!is_in_flight(item.state)) return item;
        return { ...item, state: 'canceled', speed: 0, error: '', sampled_at: item.sampled_at };
    });
}

/** Marks a row failed when the call that should have started it throws before any event arrives. */
export function fail_item(model: UploadTrayModel, key: string, message: string): UploadTrayModel {
    return replace_item(model, key, (item) => {
        if (is_finished(item.state)) return item;
        return { ...item, state: 'failed', speed: 0, error: message, upload_id: null };
    });
}

/** Starts a folder or archive import row. Its own counts replace per-file rows while it runs. */
export function begin_group(model: UploadTrayModel, name: string, now: number): UploadTrayModel {
    const existing = find_item(model, model.group_key);
    if (existing && is_in_flight(existing.state)) return model;
    const { key, seq } = next_key(model, 'folder');
    const item = blank_item(key, 'folder', 'folder', name || 'Folder', now);
    return prune({ ...model, seq, group_key: key, items: [...model.items, item] });
}

/** The backend has started walking the folder. Only the first call matters; later ones keep the row. */
export function group_started(model: UploadTrayModel, now: number): UploadTrayModel {
    if (model.group_key === null) return model;
    const current = find_item(model, model.group_key);
    if (current && current.state === 'uploading') return model;
    return replace_item(model, model.group_key, (item) => ({ ...item, state: 'uploading', sampled_at: now }));
}

/** The number of files is known once the walk ends. */
export function group_total(model: UploadTrayModel, files: number): UploadTrayModel {
    if (model.group_key === null) return model;
    return replace_item(model, model.group_key, (item) => ({ ...item, files_total: Math.max(0, Math.floor(files)) }));
}

export function group_progress(model: UploadTrayModel, sample: ImportProgressSample, now: number): UploadTrayModel {
    if (model.group_key === null) return model;
    return replace_item(model, model.group_key, (item) => {
        const value = clamp_percent(sample.progress);
        return take_sample({
            ...item,
            state: 'uploading',
            percent: Math.max(item.percent, value),
            files_total: Math.max(item.files_total, Math.floor(sample.total) || 0),
            files_done: Math.max(0, Math.floor(sample.done) || 0),
            files_failed: Math.max(0, Math.floor(sample.failed) || 0),
        }, Math.max(0, sample.bytes), now);
    });
}

export function group_complete(
    model: UploadTrayModel,
    outcome: { status: 'done' | 'failed' | 'canceled'; uploaded: number; failed: number; error: string },
    now: number,
): UploadTrayModel {
    if (model.group_key === null) return model;
    const next = replace_item(model, model.group_key, (item) => ({
        ...item,
        state: outcome.status,
        percent: outcome.status === 'done' ? 100 : item.percent,
        files_done: Math.max(0, Math.floor(outcome.uploaded) || 0),
        files_failed: Math.max(0, Math.floor(outcome.failed) || 0),
        speed: 0,
        error: outcome.error,
        sampled_at: now,
    }));
    return { ...next, group_key: null };
}

/**
 * A resumable job's progress. Jobs that are not running in this session have
 * no upload id, so they are keyed by job id and shown from their change events.
 */
export function resumable_progress(model: UploadTrayModel, job_id: string, sent: number, size: number, now: number): UploadTrayModel {
    const key = `job:${job_id}`;
    const exists = find_item(model, key);
    const base = exists ? model : { ...model, items: [...model.items, blank_item(key, 'file', 'resumable', '', now)] };
    return replace_item(base, key, (item) => {
        const total = size > 0 ? size : item.size;
        const bytes = Math.max(0, Math.min(total, sent));
        const percent = total > 0 ? (bytes / total) * 100 : 0;
        return take_sample({
            ...item,
            size: total,
            percent,
            state: item.state === 'done' ? 'done' : 'uploading',
        }, bytes, now);
    });
}

/** Maps a resumable job's stored status onto a row state. Jobs linked to an upload id are left to the upload events. */
export function resumable_changed(model: UploadTrayModel, view: ResumableView, now: number): UploadTrayModel {
    if (view.upload_id !== undefined && view.upload_id !== null) return model;
    const key = `job:${view.job_id}`;
    const exists = find_item(model, key);
    const base = exists ? model : { ...model, items: [...model.items, blank_item(key, 'file', 'resumable', view.name, now)] };
    const status = view.status;
    return replace_item(base, key, (item) => {
        const size = view.size > 0 ? view.size : item.size;
        const sent = Math.max(0, Math.min(size, view.confirmed_bytes));
        const common = {
            ...item,
            name: view.name || item.name,
            size,
            sent,
            percent: size > 0 ? (sent / size) * 100 : 0,
            sampled_at: now,
            sampled_sent: sent,
        };
        if (status === 'completed') return { ...common, state: 'done', percent: 100, sent: size, speed: 0, error: '' };
        if (status === 'canceling') return { ...common, state: 'canceled', speed: 0 };
        if (status === 'uploading' || status === 'waiting_network') return { ...common, state: 'uploading', error: '' };
        if (status === 'paused') return { ...common, state: 'queued', speed: 0, error: '' };
        return { ...common, state: 'failed', speed: 0, error: view.error || status };
    });
}

/** Removes finished rows. Used by the close control and by the auto-fade. */
export function clear_finished(model: UploadTrayModel): UploadTrayModel {
    return {
        ...model,
        items: model.items.filter((item) => !is_finished(item.state)),
    };
}

export interface TraySummary {
    /** Rows still queued or uploading. */
    in_flight: number;
    failed: number;
    done: number;
    canceled: number;
    /** True when there are rows and none of them is still in flight. */
    finished: boolean;
    /** What the card's title says. */
    phase: 'uploading' | 'complete' | 'failed' | 'canceled' | 'empty';
    /** The count the title is about: rows in flight, or rows that failed. */
    count: number;
}

export function summarize(model: UploadTrayModel): TraySummary {
    let in_flight = 0;
    let failed = 0;
    let done = 0;
    let canceled = 0;
    for (const item of model.items) {
        if (is_in_flight(item.state)) in_flight += 1;
        else if (item.state === 'failed') failed += 1;
        else if (item.state === 'done') done += 1;
        else canceled += 1;
    }
    const finished = model.items.length > 0 && in_flight === 0;
    let phase: TraySummary['phase'] = 'empty';
    let count = in_flight;
    if (model.items.length === 0) phase = 'empty';
    else if (!finished) phase = 'uploading';
    else if (failed > 0) {
        phase = 'failed';
        count = failed;
    } else if (done === 0 && canceled > 0) phase = 'canceled';
    else phase = 'complete';
    return { in_flight, failed, done, canceled, finished, phase, count };
}

/**
 * Orders rows for display: what is moving first, then what needs attention,
 * then what finished. Insertion order is kept inside each group.
 */
export function group_rows(items: readonly TrayItem[]): { active: TrayItem[]; failed: TrayItem[]; finished: TrayItem[] } {
    const active: TrayItem[] = [];
    const failed: TrayItem[] = [];
    const finished: TrayItem[] = [];
    for (const item of items) {
        if (is_in_flight(item.state)) active.push(item);
        else if (item.state === 'failed') failed.push(item);
        else finished.push(item);
    }
    return { active, failed, finished };
}

/** Estimated seconds left, or null when there is not enough to say (no size, or no speed yet). */
export function eta_seconds(item: Pick<TrayItem, 'size' | 'sent' | 'speed' | 'state'>): number | null {
    if (item.state !== 'uploading' || !(item.speed > 0) || !(item.size > 0)) return null;
    const remaining = Math.max(0, item.size - item.sent);
    return Math.ceil(remaining / item.speed);
}

/** "0:42", "3:05", "1:02:09": a clock reading, so the same text reads in every language. */
export function format_clock(seconds: number): string {
    const total = Math.max(0, Math.floor(seconds));
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const secs = total % 60;
    const pad = (value: number) => String(value).padStart(2, '0');
    return hours > 0 ? `${hours}:${pad(minutes)}:${pad(secs)}` : `${minutes}:${pad(secs)}`;
}

/** "2.4 MB" for a rate of bytes per second, without the unit suffix (the caller adds it). */
export function format_rate(bytes_per_second: number): string {
    return format_bytes(Math.max(0, bytes_per_second));
}

/** Whole percent for display, clamped to 0..100. */
export function percent_text(percent: number): number {
    return Math.round(clamp_percent(percent));
}

/** Splits a display name into base and extension, lowercase-agnostic. Used to pick the file icon. */
export function extension_of(name: string): string {
    const dot = name.lastIndexOf('.');
    if (dot <= 0 || dot === name.length - 1) return '';
    return name.slice(dot + 1);
}
