import { writable } from 'svelte/store';
import type { JobView } from '../../api/media_download';
import type { media_progress_event } from '../../api/media_download';

/** One row of the jobs list. The url and choice come from the dialog, the rest from events. */
export interface download_row {
    job_id: string;
    url: string;
    choice: string;
    title: string;
    stage: string;
    percent: number;
    speed: number;
    eta: number;
    error: string;
    file_name: string;
    folder_id: string;
    started_at: number;
}

/** Whether the download dialog is shown. The header button and the dialog both read it. */
export const downloader_open = writable(false);

/** Jobs newest first. Mutated only through the functions below. */
export const download_rows = writable<download_row[]>([]);

export function open_downloader(): void {
    downloader_open.set(true);
}

export function close_downloader(): void {
    downloader_open.set(false);
}

function row_from_view(view: JobView): download_row {
    return {
        job_id: view.job_id,
        url: view.url,
        choice: view.choice,
        title: view.title,
        stage: view.stage,
        percent: view.percent,
        speed: view.speed,
        eta: view.eta,
        error: view.error ?? '',
        file_name: view.file_name ?? '',
        folder_id: view.folder_id ?? '',
        started_at: view.started_at,
    };
}

/** Replaces the list with the backend's snapshot, keeping its order. */
export function load_download_rows(views: JobView[]): void {
    download_rows.set(views.map(row_from_view));
}

/**
 * Applies one progress event. An unknown job is inserted at the top, since the
 * backend emits its first event before Start returns the id. A stage never moves
 * backwards: a late event from an earlier stage is ignored.
 */
export function apply_progress_rows(rows: download_row[], event: media_progress_event): download_row[] {
    const index = rows.findIndex((row) => row.job_id === event.job_id);
    if (index < 0) {
        const created: download_row = {
            job_id: event.job_id,
            url: '',
            choice: '',
            title: event.title ?? '',
            stage: event.stage,
            percent: event.percent,
            speed: event.speed,
            eta: event.eta,
            error: event.error ?? '',
            file_name: event.file_name ?? '',
            folder_id: event.folder_id ?? '',
            started_at: Date.now(),
        };
        return [created, ...rows];
    }
    const current = rows[index];
    if (is_stage_regression(current.stage, event.stage)) return rows;
    const next: download_row = {
        ...current,
        stage: event.stage,
        percent: event.percent,
        speed: event.speed,
        eta: event.eta,
        title: event.title || current.title,
        error: event.error ?? current.error,
        file_name: event.file_name || current.file_name,
        folder_id: event.folder_id || current.folder_id,
    };
    const copy = rows.slice();
    copy[index] = next;
    return copy;
}

const STAGE_RANK: Record<string, number> = {
    downloading: 0,
    merging: 1,
    uploading: 2,
    done: 3,
    error: 3,
    cancelled: 3,
};

/** True when the new stage is earlier than the current one, or a terminal stage is being replaced. */
export function is_stage_regression(current: string, next: string): boolean {
    const current_rank = STAGE_RANK[current] ?? 0;
    const next_rank = STAGE_RANK[next] ?? 0;
    if (current_rank === 3) return true;
    return next_rank < current_rank;
}

/** Records the url and quality the dialog sent, once Start has returned the job id. */
export function attach_request(rows: download_row[], job_id: string, url: string, choice: string): download_row[] {
    const index = rows.findIndex((row) => row.job_id === job_id);
    if (index < 0) {
        const created: download_row = {
            job_id,
            url,
            choice,
            title: '',
            stage: 'downloading',
            percent: 0,
            speed: 0,
            eta: -1,
            error: '',
            file_name: '',
            folder_id: '',
            started_at: Date.now(),
        };
        return [created, ...rows];
    }
    const copy = rows.slice();
    copy[index] = { ...copy[index], url, choice };
    return copy;
}
