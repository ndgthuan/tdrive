import { t } from '../../i18n/i18n';
// Encryption settings: safely change the encryption password and hint.
// There is intentionally no "forgot password" reset; without the current
// password encrypted files cannot be recovered.

import { get } from 'svelte/store';
import { changeEncryptionPassword, disableBiometricUnlock, enableBiometricUnlock, getBiometricStatus, type OperationResult } from '../../api';
import { cancelBiometricAuthentication, waitForBiometricCancellation } from '../biometric';
import { state } from '../../state';
import { loadEncryptionStatus } from '../encryption';
import { notify } from '../notifications';
import { humanizeBackendError } from '../errors';
import { encryptionSettingsModal } from '../../ui/modals/encryption_settings_modal_store';
let generation = 0;

function isCurrent(request: number): boolean {
    return generation === request && get(encryptionSettingsModal.state).open;
}

export function cancelEncryptionSettings(): void {
    const nativeBusy = get(encryptionSettingsModal.state).payload?.biometricBusy;
    generation += 1;
    encryptionSettingsModal.close();
    if (nativeBusy) {
        cancelBiometricAuthentication();
    }
}

export function openEncryptionSettingsModal() {
    const request = ++generation;
    encryptionSettingsModal.open({ hint: String(state.encryption?.hint || '') });
    void refreshBiometricStatus(request);
}

async function refreshBiometricStatus(request: number): Promise<void> {
    try {
        const biometric = await getBiometricStatus();
        if (!isCurrent(request)) return;
        const payload = get(encryptionSettingsModal.state).payload;
        if (payload) encryptionSettingsModal.setPayload({ ...payload, biometric });
    } catch {
        if (!isCurrent(request)) return;
        const payload = get(encryptionSettingsModal.state).payload;
        if (payload) encryptionSettingsModal.setPayload({ ...payload, biometric: { available: false, enrolled: false, kind: 'biometric', reason: 'unavailable' } });
    }
}

export async function submitBiometricEnrollment(password: string): Promise<void> {
    const view = get(encryptionSettingsModal.state);
    if (!view.open || view.busy || !view.payload?.biometric?.available) return;
    if (!password) {
        encryptionSettingsModal.setError(t('messages.enc_bio_password_required'));
        return;
    }
    await changeBiometricEnrollment(() => enableBiometricUnlock(password), true);
}

export async function removeBiometricEnrollment(): Promise<void> {
    const view = get(encryptionSettingsModal.state);
    if (!view.open || view.busy || !view.payload?.biometric?.enrolled) return;
    await changeBiometricEnrollment(disableBiometricUnlock, false);
}

async function changeBiometricEnrollment(call: () => Promise<OperationResult>, enabling: boolean): Promise<void> {
    const request = generation;
    const payload = get(encryptionSettingsModal.state).payload;
    if (!payload) return;
    encryptionSettingsModal.setError('');
    encryptionSettingsModal.setPayload({ ...payload, biometricBusy: enabling });
    encryptionSettingsModal.setBusy(true);
    try {
        await waitForBiometricCancellation();
        if (!isCurrent(request)) return;
        const result = await call();
        if (!result.ok && result.error.code === 'biometric_cleanup_required') {
            const body = humanizeBackendError(result.error);
            // Cleanup is actionable even when authentication was canceled and
            // its dialog closed. Never hide a surviving device enrollment.
            notify({ level: 'error', title: t('messages.enc_review_bio'), body });
            if (isCurrent(request)) {
                encryptionSettingsModal.setError(body);
                await refreshBiometricStatus(request);
            }
            return;
        }
        if (!isCurrent(request)) return;
        if (!result.ok) {
            encryptionSettingsModal.setError(humanizeBackendError(result.error));
            return;
        }
        await refreshBiometricStatus(request);
        if (!isCurrent(request)) return;
        notify({ level: 'success', title: enabling ? t('messages.enc_bio_enabled_title') : t('messages.enc_bio_disabled_title'), body: enabling ? t('messages.enc_bio_enabled_body') : t('messages.enc_bio_disabled_body') });
    } catch (err) {
        if (isCurrent(request)) encryptionSettingsModal.setError(humanizeBackendError(err));
    } finally {
        if (isCurrent(request)) {
            const currentPayload = get(encryptionSettingsModal.state).payload;
            if (currentPayload) encryptionSettingsModal.setPayload({ ...currentPayload, biometricBusy: false });
            encryptionSettingsModal.setBusy(false);
        }
    }
}

export async function submitEncryptionSettings(
    currentPassword: string,
    newPassword: string,
    confirmPassword: string,
    hint: string,
): Promise<void> {
    const request = generation;
    if (!isCurrent(request) || get(encryptionSettingsModal.state).busy) return;
    encryptionSettingsModal.setError('');
    if (!currentPassword) {
        encryptionSettingsModal.setError(t('messages.enc_current_required'));
        return;
    }
    if (newPassword.length < 8) {
        encryptionSettingsModal.setError(t('messages.enc_new_min'));
        return;
    }
    if (newPassword !== confirmPassword) {
        encryptionSettingsModal.setError(t('messages.enc_new_mismatch'));
        return;
    }

    encryptionSettingsModal.setBusy(true);
    try {
        const result = await changeEncryptionPassword(currentPassword, newPassword, hint);
        if (!isCurrent(request)) return;
        if (!result.ok) {
            encryptionSettingsModal.setError(humanizeBackendError(result.error));
            return;
        }
        await loadEncryptionStatus();
        if (!isCurrent(request)) return;
        cancelEncryptionSettings();
        notify({
            level: 'success',
            title: t('messages.enc_password_changed'),
            body: t('messages.enc_password_changed_body'),
        });
    } catch (err) {
        if (isCurrent(request)) encryptionSettingsModal.setError(humanizeBackendError(err));
    } finally {
        if (isCurrent(request)) encryptionSettingsModal.setBusy(false);
    }
}
