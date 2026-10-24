// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { derived, get, writable } from 'svelte/store';
import { cancelUpload, cancelUploadById, onRuntimeEvent, retryUploadToDriveFs, type RuntimeUnsubscribe } from '../../api';
import { appActions } from '../../modules/app_actions';
import { t_store } from '../../i18n/i18n';
import { humanizeBackendError } from '../../modules/errors';
import {
    begin_group,
    cancel_item,
    clear_finished,
    empty_model,
    fail_item,
    finish_upload,
    group_complete,
    group_progress,
    group_rows,
    group_started,
    group_total,
    is_in_flight,
    progress_upload,
    register_batch,
    requeue,
    resumable_changed,
    resumable_progress,
    start_upload,
    summarize,
    type RegisterEntry,
    type UploadTrayModel,
} from './upload_tray_model';

/** How long a clean, finished tray stays on screen before it fades away. */
export const TRAY_FADE_MS = 6000;
/** Coalesces bursts of completions into one list refresh. */
const REFRESH_DEBOUNCE_MS = 250;

const model_store = writable<UploadTrayModel>(empty_model());

/** Whether the card is folded to its one-line form. Set by the auto-collapse, or by the chevron. */
export const tray_collapsed = writable(false);
/** Whether the pointer is over the tray. A hovered tray does not fade. */
export const tray_hovered = writable(false);

/** The rows and their summary, for the card. */
export const upload_tray = derived(model_store, ($model) => ({
    items: $model.items,
    summary: summarize($model),
    rows: group_rows($model.items),
}));

let fade_timer: ReturnType<typeof setTimeout> | null = null;
let refresh_timer: ReturnType<typeof setTimeout> | null = null;

function now(): number {
    return Date.now();
}

function cancel_fade(): void {
    if (fade_timer === null) return;
    clearTimeout(fade_timer);
    fade_timer = null;
}

function schedule_fade(): void {
    cancel_fade();
    if (get(tray_hovered)) return;
    fade_timer = setTimeout(() => {
        fade_timer = null;
        const current = get(model_store);
        const summary = summarize(current);
        // A failure never fades: it stays until it is retried or dismissed.
        if (!summary.finished || summary.failed > 0) return;
        commit(clear_finished(current));
    }, TRAY_FADE_MS);
}

/**
 * Stores the next model and moves the card's state with it: a finished tray
 * folds to its one-line form and, when nothing failed, starts its fade; new work
 * unfolds it again.
 */
function commit(next: UploadTrayModel): void {
    const before = summarize(get(model_store));
    model_store.set(next);
    const after = summarize(next);
    if (after.in_flight > 0) {
        cancel_fade();
        if (before.finished) tray_collapsed.set(false);
        return;
    }
    if (after.finished && (!before.finished || before.in_flight > 0)) {
        // A batch with a failure stays open, so its Retry is in reach.
        if (after.failed === 0) {
            tray_collapsed.set(true);
            schedule_fade();
        } else {
            tray_collapsed.set(false);
            cancel_fade();
        }
        return;
    }
    if (next.items.length === 0) cancel_fade();
}

function refresh_files_soon(): void {
    if (refresh_timer !== null) return;
    refresh_timer = setTimeout(() => {
        refresh_timer = null;
        appActions().refreshFiles();
    }, REFRESH_DEBOUNCE_MS);
}

function is_in_flight_upload(id: unknown): number | null {
    const value = Number(id);
    return Number.isSafeInteger(value) && value >= 0 ? value : null;
}

function as_record(value: unknown): Record<string, unknown> {
    return value && typeof value === 'object' ? (value as Record<string, unknown>) : {};
}

/**
 * Registers a plain selection before the backend is asked to upload it, so its
 * rows exist (queued) the moment the upload starts. Row i answers to upload id i.
 */
export function expect_upload_batch(entries: RegisterEntry[]): void {
    if (entries.length === 0) return;
    commit(register_batch(get(model_store), entries, now()));
}

/** A folder or archive import has been confirmed. Its row shows before the walk begins. */
export function begin_folder_upload(name: string): void {
    commit(begin_group(get(model_store), name, now()));
}

/**
 * Closes out the registered batch once its call has returned. A row the
 * backend reported as failed keeps that reason; a row it did not report as
 * failed is done, because the call only returns after every file has settled.
 * Rows still in flight when the call threw, or was cancelled, are settled the
 * same way so no row is left spinning after its call is over.
 */
export function settle_upload_batch(input: {
    failures: Array<{ index: number; message: string }>;
    canceled: boolean;
    threw: boolean;
}): void {
    const model = get(model_store);
    if (!model.batch) return;
    const fallback = get(t_store)('transfers.failed_default');
    let next = model;
    model.batch.forEach((key, index) => {
        const item = next.items.find((entry) => entry.key === key);
        if (!item || !is_in_flight(item.state)) return;
        if (input.canceled) {
            next = cancel_item(next, key);
            return;
        }
        const failure = input.failures.find((entry) => entry.index === index);
        if (failure) {
            next = finish_upload(next, { id: index, name: item.name, outcome: 'failed', error: failure.message }, now());
        } else if (input.threw) {
            next = finish_upload(next, { id: index, name: item.name, outcome: 'failed', error: fallback }, now());
        } else {
            next = finish_upload(next, { id: index, name: item.name, outcome: 'done', error: '' }, now());
        }
    });
    commit({ ...next, batch: null });
    refresh_files_soon();
}

/** The folder import failed before the backend began reporting it. */
export function abandon_folder_upload(message: string): void {
    const model = get(model_store);
    if (model.group_key === null) return;
    commit(group_complete(model, { status: 'failed', uploaded: 0, failed: 0, error: message }, now()));
}

/** Stops one in-flight file. A queued file is cancelled here and stopped when the backend starts it. */
export function cancel_tray_item(key: string): void {
    const item = get(model_store).items.find((entry) => entry.key === key);
    if (!item || !is_in_flight(item.state)) return;
    if (item.kind === 'folder') {
        void cancelUpload().catch(() => undefined);
        return;
    }
    if (item.source !== 'upload') return;
    commit(cancel_item(get(model_store), key));
    if (item.upload_id !== null) void cancelUploadById(item.upload_id).catch(() => undefined);
}

/** Sends a failed file again, through the same call that first sent it. */
export function retry_tray_item(key: string): void {
    const item = get(model_store).items.find((entry) => entry.key === key);
    if (!item || item.state !== 'failed' || !item.retry) return;
    const retry = item.retry;
    commit(requeue(get(model_store), key, now()));
    void retryUploadToDriveFs([retry.path], [retry.parent_id], retry.encrypt, retry.drive_id)
        .then((upload) => {
            if (upload.result.ok) return;
            commit(fail_item(get(model_store), key, humanizeBackendError(upload.result.error)));
        })
        .catch((error: unknown) => {
            commit(fail_item(get(model_store), key, humanizeBackendError(error)));
        });
}

/** Clears the finished rows. The close control is only offered once nothing is in flight. */
export function dismiss_finished(): void {
    cancel_fade();
    commit(clear_finished(get(model_store)));
}

export function set_tray_hovered(hovered: boolean): void {
    tray_hovered.set(hovered);
    const summary = summarize(get(model_store));
    if (hovered) cancel_fade();
    else if (summary.finished && summary.failed === 0) schedule_fade();
}

export function toggle_tray_collapsed(): void {
    tray_collapsed.update((collapsed) => !collapsed);
}

/** Subscribes the tray to the upload, import and resumable events. Returns the teardown. */
export function activate_upload_tray(): () => void {
    const unsubscribers: RuntimeUnsubscribe[] = [
        onRuntimeEvent('upload_start', (id, name, size, parentId) => {
            const uploadId = is_in_flight_upload(id);
            if (uploadId === null) return;
            // The backend reports one folder import as a single aggregate, so
            // per-file starts during one are not rows of their own.
            if (get(model_store).group_key !== null) return;
            const started = start_upload(get(model_store), {
                id: uploadId,
                name: String(name ?? ''),
                size: Number(size) || 0,
                parent_id: String(parentId ?? ''),
            }, now());
            commit(started.model);
            if (started.cancel) void cancelUploadById(uploadId).catch(() => undefined);
        }),
        onRuntimeEvent('upload_progress', (id, percent) => {
            const uploadId = is_in_flight_upload(id);
            if (uploadId === null || get(model_store).group_key !== null) return;
            commit(progress_upload(get(model_store), uploadId, Number(percent), now()));
        }),
        onRuntimeEvent('upload_complete', (id, name) => {
            const uploadId = is_in_flight_upload(id);
            if (uploadId === null || get(model_store).group_key !== null) return;
            commit(finish_upload(get(model_store), { id: uploadId, name: String(name ?? ''), outcome: 'done', error: '' }, now()));
            refresh_files_soon();
        }),
        onRuntimeEvent('upload_error', (id, name, message) => {
            const uploadId = is_in_flight_upload(id);
            if (uploadId === null || get(model_store).group_key !== null) return;
            commit(finish_upload(get(model_store), {
                id: uploadId,
                name: String(name ?? ''),
                outcome: 'failed',
                error: humanizeBackendError(message),
            }, now()));
        }),
        onRuntimeEvent('import_start', () => {
            if (get(model_store).group_key === null) commit(begin_group(get(model_store), '', now()));
            commit(group_started(get(model_store), now()));
        }),
        onRuntimeEvent('import_uploading', (info) => {
            const files = Number(as_record(info).files) || 0;
            commit(group_started(group_total(get(model_store), files), now()));
        }),
        onRuntimeEvent('import_upload_progress', (info) => {
            const payload = as_record(info);
            commit(group_progress(get(model_store), {
                total: Number(payload.total) || 0,
                done: Number(payload.done) || 0,
                failed: Number(payload.failed) || 0,
                progress: Number(payload.progress) || 0,
                bytes: Number(payload.bytes) || 0,
            }, now()));
        }),
        onRuntimeEvent('import_complete', (info) => {
            const payload = as_record(info);
            const failed = Number(payload.failed) || 0;
            const uploaded = Number(payload.uploaded) || 0;
            const backend_status = String(payload.status ?? '').toLowerCase();
            const status = backend_status === 'canceled'
                ? 'canceled'
                : backend_status === 'failed' || failed > 0 ? 'failed' : 'done';
            commit(group_complete(get(model_store), {
                status,
                uploaded,
                failed,
                error: humanizeBackendError(String(payload.error ?? '')),
            }, now()));
            refresh_files_soon();
        }),
        onRuntimeEvent('resumable_upload_changed', (payload) => {
            const view = as_record(payload);
            if (typeof view.job_id !== 'string') return;
            const next = resumable_changed(get(model_store), {
                job_id: view.job_id,
                name: String(view.name ?? ''),
                size: Number(view.size) || 0,
                confirmed_bytes: Number(view.confirmed_bytes) || 0,
                status: String(view.status ?? ''),
                error: String(view.error ?? ''),
                upload_id: typeof view.upload_id === 'number' ? view.upload_id : null,
            }, now());
            commit(next);
            if (String(view.status ?? '') === 'completed') refresh_files_soon();
        }),
        onRuntimeEvent('resumable_upload_progress', (jobId, sent, size) => {
            if (typeof jobId !== 'string') return;
            commit(resumable_progress(get(model_store), jobId, Number(sent) || 0, Number(size) || 0, now()));
        }),
    ];
    return () => {
        for (const unsubscribe of unsubscribers) unsubscribe();
        cancel_fade();
        if (refresh_timer !== null) {
            clearTimeout(refresh_timer);
            refresh_timer = null;
        }
    };
}

/** Test seam: the current model, without subscribing to it. */
export function current_tray_model(): UploadTrayModel {
    return get(model_store);
}

/** Test seam: replaces the model outright. */
export function reset_upload_tray(model: UploadTrayModel = empty_model()): void {
    cancel_fade();
    model_store.set(model);
    tray_collapsed.set(false);
    tray_hovered.set(false);
}
