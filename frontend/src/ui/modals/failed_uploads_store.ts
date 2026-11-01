import { createModalController } from './modal_store';
import type { UploadFailure } from '../../types';
import { writable } from 'svelte/store';

export interface FailedUploadsPayload {
    paths: string[];
    failures: UploadFailure[];
    parentId: string;
    encrypt: boolean;
    driveId: number | null;
    driveTitle: string;
    /**
     * Retries these failures by a route other than re-uploading `paths`. An
     * Android folder import keeps its files in the bridge, not on disk, so its
     * retry is supplied here and runs only the files that failed.
     */
    retry?: () => Promise<void>;
    /** Failures beyond the ones kept in `failures`, named in the copied list. */
    omitted?: number;
}

export const failedUploadsModal = createModalController<FailedUploadsPayload>();

// Session-only sources and actions must never enter persisted notification history.
export const failedUploadReports = writable<FailedUploadsPayload[]>([]);

export function rememberFailedUploadReport(payload: FailedUploadsPayload): void {
    failedUploadReports.update((reports) => [payload, ...reports].slice(0, 10));
}

export function dismissFailedUploadReport(payload: FailedUploadsPayload): void {
    failedUploadReports.update((reports) => reports.filter((report) => report !== payload));
}
