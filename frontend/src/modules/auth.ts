import { t } from '../i18n/i18n';
// Authentication flows for TDrive frontend.
//
// auth_screens.svelte owns rendering while this module orchestrates the login
// state machine, personal-drive recovery, dashboard bring-up, and the Telegram
// event stream that advances the typed application view.

import { get } from 'svelte/store';
import { invalidateFolderIndex, state } from '../state';
import type { PersonalDriveCandidate, PersonalDriveSetup } from '../types';
import {
    checkSystemStatus,
    saveSetup,
    loginPhoneNumber,
    submitCode as submitLoginCode,
    submitPassword as submitLoginPassword,
    checkLoginStatus,
    waitForBackendReady,
    preparePersonalDrive,
    discoverPersonalDrives as fetchPersonalDrives,
    selectPersonalDrive as selectPersonalDriveApi,
    createPersonalDrive as createPersonalDriveApi,
    getMyUserId,
    syncChannel,
    onRuntimeEvent,
} from '../api';
import { renderBreadcrumb } from './navigation';
import { loadChannels } from './channels';
import { loadEncryptionStatus } from './encryption';
import { loadSelfUser } from './profile_menu';
import { notify } from './notifications';
import { appActions } from './app_actions';
import { humanizeBackendError, toAppError } from './errors';
import { flood_wait_seconds } from './flood_wait';
import {
    authHint,
    authPhone,
    authSubmission,
    beginAuthSubmission,
    failAuthSubmission,
    finishAuthSubmission,
    resetAuthSubmissions,
    type AuthFlow,
} from '../ui/auth/auth_store';
import {
    authScreen,
    showAuthView,
    showDashboardView,
    showFatalView,
    showLoadingView,
    appView,
} from '../ui/app/app_store';
import { parseDriveScanProgress, personalDriveSetup } from '../ui/auth/personal_drive_store';

let personalDriveFlowVersion = 0;

function startPersonalDriveFlow(): number {
    personalDriveFlowVersion += 1;
    return personalDriveFlowVersion;
}

function isCurrentPersonalDriveFlow(version: number): boolean {
    return version === personalDriveFlowVersion;
}

// Wails rejects bindings with a plain string; keep whatever text we get so
// the user sees the real cause instead of a guess.
function errorText(err: unknown): string {
    if (err instanceof Error) return err.message;
    return String(err ?? '').trim();
}

async function showDashboardForFlow(flowVersion: number): Promise<void> {
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;

    showDashboardView();
    state.currentFolderId = "";
    state.folderPath = [];
    state.myUserID = 0;
    renderBreadcrumb();

    // Identity is independent of channel discovery. Resolve both together,
    // but publish identity only after this dashboard flow is still current.
    const userIDPromise = getMyUserId().catch((err: unknown) => {
        if (isCurrentPersonalDriveFlow(flowVersion)) console.warn('getMyUserId failed:', err);
        return 0;
    });

    // Load drive list (personal + any joined shared) before the first
    // refresh, so the sidebar populates and folder-control gating runs
    // based on the active drive. triggerRefresh syncs from Telegram first
    // — important for users coming back to a drive that's seen new
    // activity since they last had the app open.
    try {
        await loadChannels();
    } catch (err) {
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
        console.error('Drive list load failed:', err);
        notify({
            level: 'error',
            title: t('messages.notify_load_drive_failed'),
            body: t('messages.notify_load_drive_failed_body'),
        });
        return;
    }
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;

    // Owner-only shared-drive actions remain default-deny until identity resolves.
    const userID = await userIDPromise;
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;
    state.myUserID = Number(userID) || 0;

    // Pull Telegram metadata before reading encryption state. On a fresh
    // reinstall this is what restores the wrapped master key into SQLite.
    try {
        await appActions().triggerRefresh();
    } catch (err) {
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
        console.warn('Initial drive refresh failed:', err);
        notify({
            level: 'warning',
            title: t('messages.notify_refresh_unavailable'),
            body: t('messages.notify_refresh_unavailable_body'),
        });
    }
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;
    const personal = state.channels.find((c) => c?.kind === 'personal');
    if (personal && Number(personal.id) !== Number(state.activeChannel?.id || 0)) {
        try {
            await syncChannel(Number(personal.id));
            if (!isCurrentPersonalDriveFlow(flowVersion)) return;
            invalidateFolderIndex(personal.id);
        } catch (err) {
            if (!isCurrentPersonalDriveFlow(flowVersion)) return;
            console.warn('Personal sync before encryption status failed:', err);
        }
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
    }

    // Refresh the personal-drive encryption snapshot so the upload dialog
    // can decide between first-time setup and password entry.
    try {
        await loadEncryptionStatus();
    } catch (err) {
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
        console.warn('Encryption status load failed:', err);
        notify({
            level: 'warning',
            title: t('messages.notify_status_unavailable'),
            body: t('messages.notify_status_unavailable_body'),
        });
    }
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;

    // Hydrate the profile menu (display name, photo). Failure is non-fatal —
    // the avatar falls back to a blank circle.
    loadSelfUser();
}

function showDriveSetupScreen(): void {
    showAuthView('drive');
    personalDriveSetup.loading();
}

async function discoverPersonalDrives(flowVersion: number): Promise<void> {
    let candidates: PersonalDriveCandidate[];
    try {
        candidates = await fetchPersonalDrives();
    } catch (err) {
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
        console.error('Personal drive discovery failed:', err);
        const seconds = flood_wait_seconds(errorText(err));
        if (seconds !== null) {
            personalDriveSetup.discoveryError(t('leftovers.auth_discovery_flood_wait', { seconds }), errorText(err));
        } else {
            personalDriveSetup.discoveryError(t('leftovers.auth_discovery_failed'), errorText(err));
        }
        return;
    }
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;
    personalDriveSetup.showCandidates(candidates);
}

// Activates the saved drive without touching the screen: the common case
// goes straight to the dashboard. Only when the user actually has to choose
// does the drive picker appear, and only then does discovery hit Telegram.
export async function preparePersonalDriveAndContinue(): Promise<void> {
    const flowVersion = startPersonalDriveFlow();

    if (get(appView).kind !== 'startup') showLoadingView();
    try {
        await waitForBackendReady();
    } catch (err) {
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
        console.error('Backend startup failed:', err);
        showFatalView(toAppError(err, { source: 'startup' }));
        return;
    }
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;

    let setup: PersonalDriveSetup;
    try {
        setup = await preparePersonalDrive();
    } catch (err) {
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
        console.error('Personal drive preparation failed:', err);
        showDriveSetupScreen();
        personalDriveSetup.discoveryError(t('leftovers.auth_saved_drive_failed'), errorText(err));
        return;
    }
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;

    if (setup.status === 'ready') {
        await showDashboardForFlow(flowVersion);
        return;
    }
    showDriveSetupScreen();
    if (setup.status === 'selection_required') {
        await discoverPersonalDrives(flowVersion);
        return;
    }
    personalDriveSetup.discoveryError(
        t('leftovers.auth_prepare_failed'),
        t('leftovers.auth_unexpected_status', { status: setup.status }),
    );
}

export async function selectPersonalDrive(channelID: string): Promise<void> {
    const flowVersion = startPersonalDriveFlow();
    if (!/^[1-9]\d*$/.test(channelID)) {
        personalDriveSetup.recoveryError(t('leftovers.auth_recover_channel_invalid'));
        return;
    }
    personalDriveSetup.recovering({ createRetry: false });
    try {
        await selectPersonalDriveApi(channelID);
    } catch (err) {
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
        console.error('Personal drive recovery failed:', err);
        personalDriveSetup.recoveryError(
            t('leftovers.auth_recover_channel_failed'),
            { detail: errorText(err) },
        );
        return;
    }
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;
    await showDashboardForFlow(flowVersion);
}

export async function createPersonalDrive(): Promise<void> {
    const flowVersion = startPersonalDriveFlow();
    personalDriveSetup.recovering();
    try {
        await createPersonalDriveApi();
    } catch (err) {
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
        console.error('Personal drive creation failed:', err);
        personalDriveSetup.recoveryError(
            t('leftovers.auth_setup_failed'),
            { detail: errorText(err), createRetry: true },
        );
        return;
    }
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;
    await showDashboardForFlow(flowVersion);
}

export async function initializeSession(): Promise<void> {
    const flowVersion = startPersonalDriveFlow();
    resetAuthSubmissions();

    try {
        await waitForBackendReady();
    } catch (err) {
        if (!isCurrentPersonalDriveFlow(flowVersion)) return;
        throw err;
    }
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;
    const status = await checkSystemStatus();
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;
    if (status === 'NEEDS_SETUP') {
        showAuthView('setup');
        return;
    }

    const loggedIn = await checkLoginStatus();
    if (!isCurrentPersonalDriveFlow(flowVersion)) return;
    if (loggedIn) {
        await preparePersonalDriveAndContinue();
        return;
    }

    showAuthView('phone');
}

// --- screen submit handlers (wired into AuthScreens) ---

function currentAuthFlow(): AuthFlow | null {
    const screen = get(authScreen);
    return screen === 'setup' || screen === 'phone' || screen === 'code' || screen === 'password'
        ? screen
        : null;
}

async function submitSetup(apiIdRaw: string, apiHashRaw: string): Promise<void> {
    if (!beginAuthSubmission('setup')) return;

    const apiIdText = apiIdRaw.trim();
    const apiHash = apiHashRaw.trim();
    const apiId = Number(apiIdText);
    if (!/^\d+$/.test(apiIdText) || !Number.isSafeInteger(apiId) || apiId <= 0) {
        failAuthSubmission('setup', t('leftovers.auth_api_id_invalid'));
        return;
    }
    if (!apiHash) {
        failAuthSubmission('setup', t('leftovers.auth_api_hash_missing'));
        return;
    }

    try {
        const result = await saveSetup(apiId, apiHash);
        if (result !== 'Success') {
            failAuthSubmission('setup', humanizeBackendError(result));
            return;
        }
        finishAuthSubmission('setup');
        location.reload();
    } catch (err) {
        failAuthSubmission('setup', humanizeBackendError(err));
    }
}

async function submitPhone(phoneRaw: string): Promise<void> {
    if (!beginAuthSubmission('phone')) return;

    const phone = phoneRaw.trim();
    if (!phone) {
        failAuthSubmission('phone', t('leftovers.auth_phone_missing'));
        return;
    }

    try {
        await loginPhoneNumber(phone);
        // A very fast Telegram failure can arrive before the Wails promise
        // resolves. Do not advance over the inline error in that case.
        if (get(authSubmission).phone.error) return;
        finishAuthSubmission('phone');
        finishAuthSubmission('code');
        authPhone.set(phone);
        showAuthView('code');
    } catch (err) {
        failAuthSubmission('phone', humanizeBackendError(err));
    }
}

async function submitCode(codeRaw: string): Promise<void> {
    if (!beginAuthSubmission('code')) return;

    const code = codeRaw.trim();
    if (!code) {
        failAuthSubmission('code', t('leftovers.auth_code_missing'));
        return;
    }

    try {
        await submitLoginCode(code);
        // Keep this flow busy until Telegram emits success, password-required,
        // code-invalid, or login-error. The bridge call only queues the code.
    } catch (err) {
        failAuthSubmission('code', humanizeBackendError(err));
    }
}

async function submitPassword(passwordRaw: string): Promise<void> {
    if (!beginAuthSubmission('password')) return;

    const password = passwordRaw.trim();
    if (!password) {
        failAuthSubmission('password', t('leftovers.auth_password_missing'));
        return;
    }

    try {
        await submitLoginPassword(password);
        // As with the code, the terminal result arrives over the event stream.
    } catch (err) {
        failAuthSubmission('password', humanizeBackendError(err));
    }
}

function backToPhone(): void {
    finishAuthSubmission('code');
    finishAuthSubmission('phone');
    showAuthView('phone');
}

export const authScreenActions = {
    onSetup: submitSetup,
    onPhone: submitPhone,
    onCode: submitCode,
    onPassword: submitPassword,
    onBackToPhone: backToPhone,
    onDriveSelect: selectPersonalDrive,
    onDriveCreate: createPersonalDrive,
    onDriveRetry: preparePersonalDriveAndContinue,
};

export function connectAuthEvents(): () => void {
    const disconnectors = [
        onRuntimeEvent('login-success', () => {
            resetAuthSubmissions();
            void preparePersonalDriveAndContinue();
        }),
        onRuntimeEvent('drive_scan_progress', (payload) => {
            const update = parseDriveScanProgress(payload);
            if (update) personalDriveSetup.scanProgress(update);
        }),
        onRuntimeEvent('login-password-required', () => {
            finishAuthSubmission('code');
            finishAuthSubmission('password');
            authHint.set('');
            showAuthView('password');
        }),
        onRuntimeEvent('login-error', (msg) => {
            const message = humanizeBackendError(msg || t('leftovers.try_again'));
            const flow = currentAuthFlow();
            if (flow) {
                failAuthSubmission(flow, message);
                return;
            }
            notify({ level: 'error', title: t('messages.notify_login_failed'), body: message });
        }),
        // The backend keeps this login attempt alive. Keep the entered code
        // visible so the user can correct it without requesting another code.
        onRuntimeEvent('login-code-invalid', () => {
            showAuthView('code');
            failAuthSubmission('code', t('leftovers.auth_code_incorrect'));
        }),
        onRuntimeEvent('gothint', (hint) => {
            const text = (hint ?? '').toString().trim();
            const normalized = text.replace(/^(hint\s*:?[\s\u00A0]*)+/i, '').trim();
            authHint.set(!normalized || normalized.toLowerCase().includes('no hint') ? '' : normalized);
        }),
    ];

    let connected = true;
    return () => {
        if (!connected) return;
        connected = false;
        startPersonalDriveFlow();
        for (const disconnect of disconnectors) disconnect();
    };
}
