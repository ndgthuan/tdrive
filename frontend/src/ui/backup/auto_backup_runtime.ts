// Runs automatic photo and video backup on the phone: it keeps the backend
// told about battery and Wi-Fi, starts a run when there is something to send,
// and shows the run as one row in the upload tray. The queue itself is the
// photo backup engine's; this module only decides when to ask it to run.
import { writable } from 'svelte/store';
import { t } from '../../i18n/i18n';
import { backUpAutoBackupNow, getAutoBackupState, onAutoBackupProgress, onAutoBackupState, setAutoBackupPowerState, type AutoBackupProgress } from '../../api/auto_backup';
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

    const runNow = (): void => {
        if (!enabled || stopped) return;
        void backUpAutoBackupNow().catch(() => undefined);
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
            pushTransferStart({ id: ROW_ID, direction: 'up', name: title, total: view.total });
        } else {
            updateTransferName({ id: ROW_ID, direction: 'up', name: title });
        }
        updateTransferProgress({
            id: ROW_ID, direction: 'up', progress: view.percent,
            itemsDone: view.done, itemsTotal: view.total,
            items: progress.currentName ? [{ key: progress.currentName, name: progress.currentName, progress: 0, total: progress.currentBytes }] : [],
        });
        rowVisible = true;
    };

    const onProgress = (progress: AutoBackupProgress): void => {
        enabled = progress.enabled;
        autoBackupProgress.set(progress);
        showRow(progress);
    };

    stops.push(onAutoBackupProgress(onProgress));
    stops.push(onAutoBackupState((state) => {
        const wasEnabled = enabled;
        enabled = state.enabled;
        // Turning the toggle on is the first trigger: start the first run.
        if (enabled && !wasEnabled) {
            void pushReadings();
            runNow();
        }
    }));
    stops.push(onRuntimeEvent('ios:PhotoBackupMediaChanged', () => { void pushReadings(); runNow(); }));
    stops.push(onRuntimeEvent('android:PhotoBackupMediaChanged', () => { void pushReadings(); runNow(); }));

    const onVisibility = (): void => {
        if (typeof document === 'undefined' || document.visibilityState !== 'visible') return;
        void getAutoBackupState().then((state) => { enabled = state.enabled; }).catch(() => undefined).then(() => {
            void pushReadings();
            runNow();
        });
    };
    if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibility);

    void getAutoBackupState()
        .then((state) => { enabled = state.enabled; })
        .catch(() => undefined)
        .then(() => {
            if (enabled) {
                void pushReadings();
                runNow();
            }
        });

    timer = setInterval(() => { void pushReadings(); }, AUTO_BACKUP_POLICY_INTERVAL_MS);

    return () => {
        stopped = true;
        if (timer !== null) clearInterval(timer);
        if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibility);
        for (const stop of stops) stop();
        if (rowVisible) removeTransfer({ id: ROW_ID, direction: 'up' });
        rowVisible = false;
        autoBackupProgress.set(null);
    };
}
