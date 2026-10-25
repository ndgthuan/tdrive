// Public boundary for automatic photo and video backup. The generated Wails
// binding owns the toggle, folder and policy; the device identity and the
// settings deep link come from the native hosts, so both are read here and
// handed to the backend rather than trusted from the page.
import * as autoBindings from '../../bindings/TDrive/internal/app/autobackupservice';
import { callBridge, hasBridgeMethod } from '../modules/android_bridge';
import { invokeBackend } from './gateway';
import { onRuntimeEvent } from './runtime';
import { asRecord, boundedText } from './shared';

export type AutoBackupPhase = 'off' | 'needs_permission' | 'paused' | 'waiting_for_wifi' | 'waiting_for_charging' | 'running' | 'up_to_date' | 'failed';
export type AutoBackupPermission = 'undetermined' | 'granted' | 'limited' | 'denied';
export type AutoBackupPlatform = 'ios' | 'android' | '';

export interface AutoBackupState {
    enabled: boolean;
    folderName: string;
    defaultFolderName: string;
    folderNameCustom: boolean;
    folderId: string;
    wifiOnly: boolean;
    chargingOnly: boolean;
    permission: AutoBackupPermission;
    phase: AutoBackupPhase;
    platform: AutoBackupPlatform;
    errors: string[];
}

export interface AutoBackupDeviceIdentity {
    platform: 'ios' | 'android';
    manufacturer: string;
    model: string;
}

const PHASES: readonly AutoBackupPhase[] = ['off', 'needs_permission', 'paused', 'waiting_for_wifi', 'waiting_for_charging', 'running', 'up_to_date', 'failed'];
const PERMISSIONS: readonly AutoBackupPermission[] = ['undetermined', 'granted', 'limited', 'denied'];

function oneOf<T extends string>(value: unknown, allowed: readonly T[], fallback: T): T {
    return allowed.includes(value as T) ? (value as T) : fallback;
}

/**
 * Turns the generated binding payload into the pane's own shape. Unknown
 * phases and permissions fall back to the safe reading (off, undetermined)
 * rather than letting a new backend value reach the UI unchecked.
 */
export function normalizeAutoBackupState(raw: unknown): AutoBackupState {
    const value = asRecord(raw);
    const errors = Array.isArray(value.errors)
        ? value.errors.map((item) => boundedText(item, 300)).filter((item) => item !== '')
        : [];
    const platform = value.platform === 'ios' || value.platform === 'android' ? value.platform : '';
    return {
        enabled: value.enabled === true,
        folderName: boundedText(value.folder_name ?? value.folderName, 240) || 'Phone',
        defaultFolderName: boundedText(value.default_folder_name ?? value.defaultFolderName, 240) || 'Phone',
        folderNameCustom: value.folder_name_custom === true || value.folderNameCustom === true,
        folderId: boundedText(value.folder_id ?? value.folderId, 512),
        wifiOnly: (value.wifi_only ?? value.wifiOnly) !== false,
        chargingOnly: (value.charging_only ?? value.chargingOnly) === true,
        permission: oneOf(value.permission, PERMISSIONS, 'undetermined'),
        phase: oneOf(value.phase, PHASES, 'off'),
        platform,
        errors,
    };
}

export async function getAutoBackupState(): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.GetState));
}

export async function setAutoBackupEnabled(enabled: boolean): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.SetEnabled, enabled));
}

export async function setAutoBackupFolderName(name: string): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.SetFolderName, name));
}

export async function setAutoBackupWifiOnly(value: boolean): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.SetWifiOnly, value));
}

export async function setAutoBackupChargingOnly(value: boolean): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.SetChargingOnly, value));
}

/** Reports the media grant the host returned; the backend keeps it for the pane. */
export async function setAutoBackupPermission(status: string): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.SetPermission, status));
}

/** Sends the native device identity so the backend can name the folder. */
export async function setAutoBackupDeviceIdentity(identity: AutoBackupDeviceIdentity): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.SetDeviceIdentity, identity.platform, identity.manufacturer, identity.model));
}

interface IOSPhotosBridge { getDeviceIdentity?: () => unknown; openAppSettings?: () => unknown; }

function iosBridge(): IOSPhotosBridge | null {
    return typeof window !== 'undefined' ? (window as Window & { tdriveIOSPhotos?: IOSPhotosBridge }).tdriveIOSPhotos ?? null : null;
}

function parseIdentity(raw: unknown): AutoBackupDeviceIdentity | null {
    const value = asRecord(raw);
    const platform = value.platform === 'ios' || value.platform === 'android' ? value.platform : null;
    if (!platform) return null;
    return {
        platform,
        manufacturer: boundedText(value.manufacturer, 80),
        model: boundedText(value.model, 80),
    };
}

/**
 * Reads the device identity from whichever native host is present. Returns
 * null on desktop and in any host that does not provide it, so the caller
 * keeps the generic default folder rather than guessing.
 */
export async function readNativeDeviceIdentity(): Promise<AutoBackupDeviceIdentity | null> {
    const ios = iosBridge();
    if (ios?.getDeviceIdentity) {
        const result = await Promise.resolve(ios.getDeviceIdentity());
        return parseIdentity(typeof result === 'string' ? safeJSON(result) : result);
    }
    if (hasBridgeMethod('deviceIdentity')) {
        const raw = await callBridge('deviceIdentity', [], 'Device identity is unavailable.');
        return parseIdentity(safeJSON(raw));
    }
    return null;
}

/** Opens the system page where the photo permission can be changed. */
export async function openNativeAppSettings(): Promise<boolean> {
    const ios = iosBridge();
    if (ios?.openAppSettings) {
        await Promise.resolve(ios.openAppSettings());
        return true;
    }
    if (hasBridgeMethod('openAppSettings')) {
        await callBridge('openAppSettings', [], 'Settings are unavailable.');
        return true;
    }
    return false;
}

function safeJSON(raw: string): unknown {
    try { return JSON.parse(raw || '{}'); } catch { return {}; }
}

/** The queue summary: counts for the row, and the file in flight for the pane. */
export interface AutoBackupProgress {
    enabled: boolean;
    done: number;
    total: number;
    currentName: string;
    currentBytes: number;
    currentBytesDone?: number;
    paused: boolean;
    folderName: string;
}

function count(value: unknown): number {
    return typeof value === 'number' && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
}

export function normalizeAutoBackupProgress(raw: unknown): AutoBackupProgress {
    const value = asRecord(raw);
    return {
        enabled: value.enabled === true,
        done: count(value.done),
        total: count(value.total),
        currentName: boundedText(value.current_name ?? value.currentName, 512),
        currentBytes: count(value.current_bytes ?? value.currentBytes),
        currentBytesDone: count(value.current_bytes_done ?? value.currentBytesDone),
        paused: value.paused === true,
        folderName: boundedText(value.folder_name ?? value.folderName, 240),
    };
}

/** Starts a run now. The backend refuses it while automatic backup is off. */
export async function backUpAutoBackupNow(): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.BackUpNow));
}

export async function pauseAutoBackup(): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.PauseBackup));
}

export async function resumeAutoBackup(): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.ResumeBackup));
}

/**
 * Sends one battery reading. levelPercent is 0 to 100, or negative when the
 * host cannot say. The backend treats a reading older than two minutes as
 * unknown, and a charging-only backup waits on an unknown reading.
 */
export async function setAutoBackupPowerState(charging: boolean, levelPercent: number, lowPowerMode: boolean): Promise<AutoBackupState> {
    return normalizeAutoBackupState(await invokeBackend(autoBindings.SetPowerState, charging, Math.round(levelPercent), lowPowerMode));
}

export async function getAutoBackupProgress(): Promise<AutoBackupProgress> {
    return normalizeAutoBackupProgress(await invokeBackend(autoBindings.GetProgress));
}

/** Subscribes to the queue summary the backend emits while automatic backup is on. */
export function onAutoBackupProgress(callback: (progress: AutoBackupProgress) => void): () => void {
    return onRuntimeEvent('auto_backup:progress', (payload: unknown) => callback(normalizeAutoBackupProgress(payload)));
}

/** Subscribes to state changes, such as the toggle turning on or off. */
export function onAutoBackupState(callback: (state: AutoBackupState) => void): () => void {
    return onRuntimeEvent('auto_backup:state', (payload: unknown) => callback(normalizeAutoBackupState(payload)));
}
