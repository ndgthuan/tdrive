import type { Readable } from 'svelte/store';
import type { PhotoBackupState } from '../../api/photo_backup';
import { setPhotoBackupBackgroundLease } from '../../api/photo_backup';
import { canRunInBackground, setPhotoBackupBackgroundDemand, type ForegroundSummary } from '../android_foreground';
import { onRuntimeEvent, runtimeEventsAvailable, type RuntimeUnsubscribe } from '../../api/runtime';
import { setIOSPhotoBackupBackground } from '../photo_backup/native_adapter';

import { PAUSE_BACKUP_ACTION } from '../notification_action_ids';
import { t } from '../../i18n/i18n';

// Built at the moment it is shown, so the shade reads in the current language.
function ongoing_notice() {
    return {
        title: t('photos.notice_title'),
        text: t('photos.notice_text'),
        progress: -1,
        // The queue is the backend's to schedule, so the only thing worth offering
        // from the shade is the one the panel offers: stop sending for now.
        action: PAUSE_BACKUP_ACTION,
    };
}

export function activatePhotoBackupBackground(
    state: Readable<PhotoBackupState | null>,
    report: (message: string) => void,
): () => void {
    let desired = false, applied = false, disposed = false, generation = 0;
    let work = Promise.resolve();
    // What the run had done when it took the process, so the line left behind
    // counts this backup rather than every backup the drive has ever done.
    let latest: PhotoBackupState | null = null;
    let completeAtStart = 0;
    let failedAtStart = 0;

    /**
     * The one line the user finds afterwards.
     *
     * A backup that ran while the phone was in a pocket ends with its ongoing
     * notification simply disappearing, which says nothing about whether the
     * photos are safe. This says so, once. Whether the user was there to watch
     * it is not decided here: the host layer drops a summary raised while the
     * app is on screen, and one place owning that question is the reason it
     * cannot be answered two different ways.
     */
    const summary = (): ForegroundSummary | undefined => {
        const done = Math.max(0, (latest?.status.complete ?? 0) - completeAtStart);
        const failed = Math.max(0, (latest?.status.failed ?? 0) - failedAtStart);
        if (done === 0 && failed === 0) return undefined;
        if (failed > 0) {
            return {
                outcome: 'stopped',
                title: t('photos.attention_title'),
                text: t('photos.could_not_back_up', { count: failed }),
            };
        }
        return {
            outcome: 'complete',
            title: t('photos.backup_done_title'),
            text: t('photos.added_to_drive', { count: done }),
        };
    };
    const acquireNative = async (): Promise<boolean> => {
        if (canRunInBackground()) { await setPhotoBackupBackgroundDemand(ongoing_notice()); return true; }
        return setIOSPhotoBackupBackground(true);
    };
    const releaseNative = async (): Promise<void> => {
        if (canRunInBackground()) await setPhotoBackupBackgroundDemand(null, summary());
        else await setIOSPhotoBackupBackground(false);
    };
    const reconcile = (): void => {
        const ticket = ++generation;
        // Acquire only while visible (Android rejects background starts), then
        // keep an acquired lease across the actual visibility transition.
        const target = desired && (applied || document.visibilityState === 'visible');
        work = work.catch(() => undefined).then(async () => {
            if (target === applied) return;
            if (!target) {
                try {
                    await setPhotoBackupBackgroundLease(false);
                    await releaseNative();
                    applied = false;
                } catch {
                    report(t('photos.background_cleanup_failed'));
                }
                return;
            }
            let acquired = false;
            try {
                acquired = await acquireNative();
                if (!acquired) {
                    report(t('photos.background_unavailable'));
                    return;
                }
                if (disposed || ticket !== generation || !desired || document.visibilityState !== 'visible') {
                    await releaseNative(); return;
                }
                await setPhotoBackupBackgroundLease(true);
                if (disposed || ticket !== generation || !desired) {
                    await setPhotoBackupBackgroundLease(false);
                    await releaseNative(); return;
                }
                applied = true;
                completeAtStart = latest?.status.complete ?? 0;
                failedAtStart = latest?.status.failed ?? 0;
            } catch {
                if (acquired) await releaseNative().catch(() => undefined);
                applied = false;
                report(t('photos.background_start_failed'));
            }
        });
    };
    const unsubscribe = state.subscribe((value) => {
        latest = value;
        desired = Boolean((value?.platform === 'android' || value?.platform === 'ios')
            && value.settings.enabled && !value.manualPaused
            && (value.status.uploading > 0 || value.status.phase === 'uploading'));
        reconcile();
    });
    const expire = () => { desired = false; reconcile(); };
    const visible = () => reconcile();
    const stops: RuntimeUnsubscribe[] = [];
    if (runtimeEventsAvailable()) stops.push(onRuntimeEvent('android:BackgroundTransferExpired', expire));
    window.addEventListener('ios:PhotoBackupBackgroundExpired', expire);
    document.addEventListener('visibilitychange', visible);
    return () => {
        unsubscribe();
        for (const stop of stops) stop();
        window.removeEventListener('ios:PhotoBackupBackgroundExpired', expire);
        document.removeEventListener('visibilitychange', visible);
        disposed = true;
        desired = false;
        reconcile();
    };
}
