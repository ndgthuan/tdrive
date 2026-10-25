// Runs automatic photo and video backup on the phone: it keeps the backend
// told about battery and Wi-Fi, starts a run when there is something to send,
// and shows the run as one row in the upload tray. The queue itself is the
// photo backup engine's; this module only decides when to ask it to run.
import { writable } from 'svelte/store';
import { t } from '../../i18n/i18n';
import { getAutoBackupProgress, getAutoBackupState, onAutoBackupProgress, onAutoBackupState, setAutoBackupPowerState, type AutoBackupProgress } from '../../api/auto_backup';
import { discoverAutomaticPhotoBackup, photoBackupState } from '../../modules/photo_backup/controller';
import { setPhotoBackupPolicy } from '../../api/photo_backup';
import { onRuntimeEvent } from '../../api/runtime';
import { nativePhotoBackupPolicy, nativePowerStatus } from '../../modules/photo_backup/native_adapter';
import { markTransferDone, pushTransferStart, removeTransfer, updateTransferName, updateTransferProgress } from '../../modules/notif_bell';
import { transferView } from './auto_backup_model';

/** How often the battery and Wi-Fi readings are sent while automatic backup is on. */
export const AUTO_BACKUP_POLICY_INTERVAL_MS = 30_000;

const ROW_ID = 'auto-backup';

/** The latest queue summary, for the pane and anything else that shows it. */
export const autoBackupProgress = writable<AutoBackupProgress | null>(null);

/**
 * Starts the runtime and returns the function that stops it. Triggers: the
 * toggle turning on, the app coming to the foreground, a library change from
 * the host, and a timer that keeps battery and Wi-Fi readings fresh.
 */
export function startAutoBackupRuntime(): () => void {
    let enabled = false;
    let rowVisible = false;
    let timer: ReturnType<typeof setInterval> | null = null;
    const stops: Array<() => void> = [];
    let stopped = false;
    let readingProgress = false;
    let stateEpoch = 0;

    const runNow = (): void => {
        if (!enabled || stopped) return;
        void discoverAutomaticPhotoBackup();
    };

    const pushReadings = async (): Promise<void> => {
        if (!enabled || stopped) return;
        try {
            const power = await nativePowerStatus();
            if (power) await setAutoBackupPowerState(power.charging, power.levelPercent, power.lowPower);
            const wifi = await nativePhotoBackupPolicy();
            if (wifi !== null) await setPhotoBackupPolicy(wifi);
        } catch {
            // A reading that fails is simply not sent. The backend already treats
            // an old reading as unknown, so nothing is uploaded on a guess.
        }
    };

    const showRow = (progress: AutoBackupProgress): void => {
        const view = transferView(progress);
        if (!progress.enabled || progress.paused || progress.total <= 0) {
            if (rowVisible) removeTransfer({ id: ROW_ID, direction: 'up' });
            rowVisible = false;
            return;
        }
        if (view.complete) {
            if (rowVisible) {
                updateTransferName({ id: ROW_ID, direction: 'up', name: t('auto_backup.transfers_row', { done: view.done, total: view.total }) });
                markTransferDone({ id: ROW_ID, direction: 'up', status: 'done' });
            }
            rowVisible = false;
            return;
        }
        const title = t('auto_backup.transfers_row', { done: view.done, total: view.total });
        if (!rowVisible) {
            pushTransferStart({ id: ROW_ID, direction: 'up', name: title, total: 0 });
        } else {
            updateTransferName({ id: ROW_ID, direction: 'up', name: title });
        }
        updateTransferProgress({
            id: ROW_ID, direction: 'up', progress: view.percent,
            bytes: 0, total: 0,
            itemsDone: view.done, itemsTotal: view.total,
            items: progress.currentName ? [{ key: progress.currentName, name: progress.currentName, progress: progress.currentBytes > 0 ? Math.min(100, (progress.currentBytesDone ?? 0) / progress.currentBytes * 100) : 0, total: progress.currentBytes }] : [],
        });
        rowVisible = true;
    };

    const onProgress = (progress: AutoBackupProgress): void => {
        if (stopped || progress.enabled !== enabled) return;
        autoBackupProgress.set(progress);
        showRow(progress);
    };

    // Restored settings do not necessarily start the backend's event ticker.
    // Read a snapshot immediately and while foregrounded so reopening the app
    // cannot leave the backup page and Transfers empty during a running upload.
    const refreshProgress = async (): Promise<void> => {
        if (!enabled || stopped || readingProgress || document.visibilityState !== 'visible') return;
        readingProgress = true;
        const epoch = stateEpoch;
        try {
            const progress = await getAutoBackupProgress();
            if (epoch === stateEpoch) onProgress(progress);
        } catch { /* Keep the last observed progress; the controller exposes queue failures. */ }
        finally { readingProgress = false; }
    };

    stops.push(onAutoBackupProgress(onProgress));
    stops.push(onAutoBackupState((state) => {
        const wasEnabled = enabled;
        stateEpoch += 1;
        enabled = state.enabled;
        if (!enabled) {
            if (rowVisible) markTransferDone({ id: ROW_ID, direction: 'up', status: 'stopped' });
            rowVisible = false;
            autoBackupProgress.set(null);
        }
        // Turning the toggle on is the first trigger: start the first run.
        if (enabled && !wasEnabled) {
            void pushReadings();
            runNow();
            void refreshProgress();
        }
    }));
    stops.push(onRuntimeEvent('ios:PhotoBackupMediaChanged', () => { void pushReadings(); runNow(); }));
    stops.push(onRuntimeEvent('android:PhotoBackupMediaChanged', () => { void pushReadings(); runNow(); }));

    const restoreState = async (): Promise<void> => {
        const epoch = stateEpoch;
        try {
            const state = await getAutoBackupState();
            if (stopped || epoch !== stateEpoch) return;
            enabled = state.enabled;
            if (enabled) { void pushReadings(); runNow(); void refreshProgress(); }
        } catch { /* A later foreground entry or scope snapshot can retry readiness. */ }
    };
    const onVisibility = (): void => {
        if (typeof document === 'undefined' || document.visibilityState !== 'visible') return;
        void restoreState();
    };
    if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibility);

    void restoreState();
    let observedScope: string | null = null;
    stops.push(photoBackupState.subscribe((state) => {
        const scope = state?.destination.id ?? null;
        if (scope === observedScope) return;
        observedScope = scope;
        stateEpoch += 1;
        autoBackupProgress.set(null);
        if (rowVisible) removeTransfer({ id: ROW_ID, direction: 'up' });
        rowVisible = false;
        if (scope !== null) void restoreState();
    }));

    timer = setInterval(() => { void pushReadings(); }, AUTO_BACKUP_POLICY_INTERVAL_MS);
    const progressTimer = setInterval(() => { void refreshProgress(); }, 1500);

    return () => {
        stopped = true;
        if (timer !== null) clearInterval(timer);
        clearInterval(progressTimer);
        if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility);
        for (const stop of stops) stop();
        if (rowVisible) removeTransfer({ id: ROW_ID, direction: 'up' });
        rowVisible = false;
        autoBackupProgress.set(null);
    };
}
