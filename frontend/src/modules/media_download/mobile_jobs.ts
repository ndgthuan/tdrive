// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Keeps the phone's download list current. It starts the native relay once,
// listens for progress, and loads the job list the Transfers section shows.

import { cancel_download, list_downloads, subscribe_media_progress } from '../../api/media_download';
import type { RuntimeUnsubscribe } from '../../api/gateway';
import { apply_progress_rows, download_rows, load_download_rows } from '../../ui/downloader/downloader_state';
import { native_engine_present, start_host_relay } from './native_engine';

let progress_stop: RuntimeUnsubscribe | null = null;

/** Idempotent. Does nothing on a device without a native download engine. */
export function start_mobile_downloads(): void {
    if (!native_engine_present()) return;
    start_host_relay();
    if (!progress_stop) {
        progress_stop = subscribe_media_progress((event) => {
            download_rows.update((rows) => apply_progress_rows(rows, event));
        });
    }
    void list_downloads()
        .then(load_download_rows)
        .catch(() => undefined);
}

/** Stops one running job. The job reports its own cancelled stage afterwards. */
export function cancel_mobile_download(job_id: string): Promise<void> {
    return cancel_download(job_id);
}
