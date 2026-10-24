/**
 * What the backup panel says, derived from what the backend reported.
 *
 * The state carries a phase, counts, a message, and two flags that override
 * the phase (a manual pause, a locked vault). Reading all of that in markup
 * turned into a wall of conditions with three competing definitions of
 * "paused". This is the one place that ranks them, so the panel only renders
 * and the activity row and tests can share the same reading.
 */

import type { PhotoBackupState, PhotoBackupStatus } from '../../api/photo_backup';
import { t } from '../../i18n/i18n';

export type BackupTone = 'idle' | 'busy' | 'success' | 'warning' | 'locked' | 'danger';

export type BackupAction = 'start' | 'pause' | 'resume' | 'retry';

export interface BackupSituation {
    tone: BackupTone;
    /** One short line, the thing a glance should take away. */
    title: string;
    /** The sentence under it; empty when the title says it all. */
    body: string;
    /**
     * The backend's own words about a failure, for the reader who wants them.
     *
     * Kept out of `body` on purpose. What the backend reports is a cause, not a
     * sentence -- an upload path and an errno, which told one user their photos
     * had stopped backing up by filling half the panel with
     * "/data/user/0/.../image-1000219416/.england-london-bridge (2).jpg.partial:
     * open failed: ENOENT". The panel says what happened and what to do; this
     * sits behind a disclosure for whoever is diagnosing it.
     */
    detail: string;
    /** The one thing to do about it, if anything. */
    primary: BackupAction | null;
    /** A less urgent second option, shown smaller. */
    secondary: BackupAction | null;
    /** True while a transfer or scan is under way and the bar should move. */
    progress: boolean;
}

/** The message key for an action's label; the locked variants say what the tap will do first. */
export function action_label_key(action: BackupAction, locked: boolean): string {
    switch (action) {
        case 'start': return locked ? 'photos.action_unlock_back_up' : 'photos.action_back_up_now';
        case 'resume': return locked ? 'photos.action_unlock_resume' : 'photos.action_resume';
        case 'retry': return locked ? 'photos.action_unlock_retry' : 'photos.action_retry';
        case 'pause': return 'photos.action_pause';
    }
}

/** Labels per action, in the current language. */
export function actionLabel(action: BackupAction, locked: boolean): string {
    return t(action_label_key(action, locked));
}

/** "24 backed up · 3 waiting · 1 failed", leaving out whatever is zero. */
export function summaryLine(status: PhotoBackupStatus): string {
    const parts: string[] = [];
    const waiting = status.pending + status.uploading;
    if (status.complete > 0) parts.push(t('photos.summary_backed_up', { count: status.complete }));
    if (waiting > 0) parts.push(t('photos.summary_waiting', { count: waiting }));
    if (status.failed > 0) parts.push(t('photos.summary_failed', { count: status.failed }));
    if (status.paused > 0) parts.push(t('photos.summary_interrupted', { count: status.paused }));
    return parts.join(' · ');
}

/**
 * How far through the queue the backup is, counted in files, or null when
 * there is nothing yet to measure.
 *
 * Files, not bytes: the backend does not total the queue's bytes, so a bar
 * wired to them sits at zero for the whole run. A barber pole says only that
 * something is happening, which the spinner beside it already said.
 */
export function queueProgress(status: PhotoBackupStatus): { value: number; max: number } | null {
    const max = status.complete + status.pending + status.uploading + status.failed + status.paused;
    return max > 0 ? { value: status.complete, max } : null;
}

/** The file name alone; sources hand over paths and the row is one line tall. */
export function currentFileName(status: PhotoBackupStatus): string {
    const parts = status.currentFile.split(/[\\/]/).filter(Boolean);
    return parts[parts.length - 1] ?? '';
}

export function describeBackup(state: PhotoBackupState): BackupSituation {
    const { status } = state;
    const waiting = status.pending + status.uploading;
    const hasSources = state.sources.some((source) => source.enabled);
    // iOS gives the app no background execution, so backup only advances while
    // TDrive is open. Say so rather than imply a scheduler the platform has not.
    // Android's foreground service keeps running, so its copy stays as is.
    const whileOpen = state.platform === 'ios';

    if (!state.settings.enabled) {
        return { tone: 'idle', title: t('photos.off_title'), body: whileOpen ? t('photos.off_body_open') : t('photos.off_body'), detail: '', primary: null, secondary: null, progress: false };
    }
    if (state.encryptionRequired) {
        // The queue's own phase decides what the unlock leads into.
        const next: BackupAction = state.manualPaused ? 'resume' : status.failed > 0 || status.paused > 0 ? 'retry' : 'start';
        return { tone: 'locked', title: t('photos.locked_title'), body: t('photos.locked_body'), detail: '', primary: next, secondary: null, progress: false };
    }
    // Bodies never carry counts: the summary line under them does, once.
    if (state.manualPaused) {
        return { tone: 'idle', title: t('photos.paused_title'), body: waiting > 0 ? t('photos.paused_body_waiting') : t('photos.paused_body_idle'), detail: '', primary: 'resume', secondary: null, progress: false };
    }
    switch (status.phase) {
        case 'uploading':
            return { tone: 'busy', title: t('photos.uploading_title'), body: currentFileName(status) || t('photos.preparing_next'), detail: '', primary: null, secondary: 'pause', progress: true };
        case 'scanning':
            return { tone: 'busy', title: t('photos.scanning_title'), body: '', detail: '', primary: null, secondary: 'pause', progress: true };
        case 'failed':
            return {
                tone: 'danger',
                title: t('photos.could_not_back_up', { count: status.failed }),
                body: t('photos.failed_body'),
                detail: status.message,
                primary: 'retry', secondary: null, progress: false,
            };
        case 'paused':
            // Not the user's pause -- that returned above. Either a device
            // condition the backend named, or items whose last upload was
            // cut off with the remote outcome unknown.
            if (status.paused > 0) {
                return { tone: 'warning', title: t('photos.interrupted', { count: status.paused }), body: t('photos.interrupted_body'), detail: '', primary: 'retry', secondary: null, progress: false };
            }
            return { tone: 'warning', title: t('photos.waiting_title'), body: status.message || t('photos.waiting_default'), detail: '', primary: 'start', secondary: null, progress: false };
        case 'queued':
            return { tone: 'idle', title: t('photos.ready_to_back_up'), body: '', detail: '', primary: 'start', secondary: null, progress: false };
        case 'complete':
            if (state.sources.some((source) => source.enabled && source.kind === 'device-folder' && source.scanComplete === false)) {
                return { tone: 'warning', title: t('photos.still_checking_title'), body: t('photos.still_checking_body'), detail: '', primary: 'start', secondary: null, progress: false };
            }
            return { tone: 'success', title: t('photos.up_to_date_title'), body: whileOpen ? t('photos.up_to_date_body_open') : '', detail: '', primary: 'start', secondary: null, progress: false };
        default:
            if (!hasSources) {
                return { tone: 'idle', title: t('photos.choose_title'), body: t('photos.choose_body'), detail: '', primary: null, secondary: null, progress: false };
            }
            return { tone: 'idle', title: t('photos.ready_title'), body: status.complete > 0 ? '' : t('photos.nothing_backed_up'), detail: '', primary: 'start', secondary: null, progress: false };
    }
}

/**
 * Where backups land, in the words the drive would use.
 *
 * The backend builds the destination as "Photo backup / <device> / <source>",
 * and the device segment carries a hex suffix so two phones of the same make
 * cannot collide in one drive. That suffix is for the folder, not the reader --
 * "Android device (3c48be52)" tells a person nothing their own phone does not
 * already tell them -- so it is dropped here and kept in the drive.
 */
export function destinationLabel(title: string): string {
    return title
        .split(' / ')
        .map((segment) => segment.replace(/\s*\([0-9a-f]{6,}\)$/i, ''))
        .join(' / ');
}

/** Whether the primary action can be taken right now. */
export function canStart(state: PhotoBackupState): boolean {
    const { settings } = state;
    return settings.enabled && (settings.photos || settings.videos) && state.sources.some((source) => source.enabled);
}
