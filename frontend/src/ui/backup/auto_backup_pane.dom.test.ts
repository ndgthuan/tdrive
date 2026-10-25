import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';

const state = { enabled: true, permission: 'undetermined', platform: 'ios', phase: 'needs_permission', folderName: 'iPhone', defaultFolderName: 'iPhone', folderNameCustom: false, wifiOnly: false, chargingOnly: false, errors: [] };
const mocks = vi.hoisted(() => ({ read: vi.fn(), request: vi.fn(), permission: vi.fn(), run: vi.fn(), get: vi.fn(), enable: vi.fn(), refresh: vi.fn() }));
vi.mock('../../modules/photo_backup/controller', async () => {
    const { writable } = await import('svelte/store');
    return { photoBackupState: writable(null), photoBackupError: writable(''), refreshPhotoBackup: mocks.refresh, startPhotoBackup: mocks.run };
});
vi.mock('../../api/auto_backup', () => ({
    getAutoBackupState: mocks.get, readNativeDeviceIdentity: async () => null,
    setAutoBackupPermission: mocks.permission, backUpAutoBackupNow: mocks.run,
    setAutoBackupEnabled: mocks.enable, setAutoBackupChargingOnly: vi.fn(), setAutoBackupDeviceIdentity: vi.fn(),
    setAutoBackupFolderName: vi.fn(), setAutoBackupWifiOnly: vi.fn(), openNativeAppSettings: vi.fn(),
}));
vi.mock('../../modules/photo_backup/native_adapter', () => ({
    nativePhotoBackupAvailable: () => true, requestNativePhotoBackupAccess: mocks.request, readNativePhotoBackupAccess: mocks.read,
}));
vi.mock('./auto_backup_runtime', async () => {
    const { writable } = await import('svelte/store');
    return { autoBackupProgress: writable(null) };
});
import Pane from './auto_backup_pane.svelte';
import { photoBackupError, photoBackupState } from '../../modules/photo_backup/controller';
import type { PhotoBackupState } from '../../api/photo_backup';

let app: ReturnType<typeof mount> | undefined;
afterEach(async () => { if (app) await unmount(app); app = undefined; document.body.innerHTML = ''; photoBackupState.set(null); photoBackupError.set(''); vi.clearAllMocks(); });

describe('iOS backup permission recovery', () => {
    it('requests Photos when a previous installation left backup enabled without a grant', async () => {
        mocks.get.mockResolvedValue(state);
        mocks.read.mockResolvedValue('undetermined');
        mocks.request.mockResolvedValue({ status: 'granted', detail: '' });
        mocks.permission.mockResolvedValue({ ...state, permission: 'granted' });
        mocks.run.mockResolvedValue({ ...state, permission: 'granted', phase: 'running' });
        app = mount(Pane, { target: document.body });
        flushSync();
        await vi.waitFor(() => expect(mocks.run).toHaveBeenCalledOnce());
        expect(mocks.request).toHaveBeenCalledOnce();
        expect(mocks.permission).toHaveBeenCalledWith('granted');
    });

    it('shows an explicit permission action when automatic backup is off', async () => {
        mocks.get.mockResolvedValue({ ...state, enabled: false });
        mocks.read.mockResolvedValue('undetermined');
        app = mount(Pane, { target: document.body });
        flushSync();
        await vi.waitFor(() => expect(document.body.textContent).toContain('Allow photo access'));
        expect(mocks.request).not.toHaveBeenCalled();
    });

    it('starts the discovery controller after enabling instead of leaving an empty upload queue', async () => {
        mocks.get.mockResolvedValue({ ...state, enabled: false });
        mocks.read.mockResolvedValue('granted');
        mocks.request.mockResolvedValue({ status: 'granted' });
        mocks.permission.mockResolvedValue({ ...state, enabled: false, permission: 'granted' });
        mocks.enable.mockResolvedValue({ ...state, permission: 'granted', phase: 'up_to_date' });
        app = mount(Pane, { target: document.body });
        flushSync();
        await vi.waitFor(() => expect(document.querySelector('[role="switch"]')).not.toBeNull());
        (document.querySelector('[role="switch"]') as HTMLButtonElement).click();
        await vi.waitFor(() => expect(mocks.enable).toHaveBeenCalledWith(true));
        await vi.waitFor(() => expect(mocks.run).toHaveBeenCalledOnce());
        const refreshOrder = mocks.refresh.mock.invocationCallOrder;
        expect(refreshOrder[refreshOrder.length - 1]).toBeLessThan(mocks.run.mock.invocationCallOrder[0]);
        expect(document.body.textContent).not.toContain('Everything is backed up');
    });

    it('shows the encryption blocker and a retry action even with zero queued files', async () => {
        mocks.get.mockResolvedValue({ ...state, permission: 'granted', phase: 'up_to_date' });
        mocks.read.mockResolvedValue('granted');
        photoBackupState.set({ encryptionRequired: true } as PhotoBackupState);
        photoBackupError.set('Enter your encryption password first.');
        app = mount(Pane, { target: document.body });
        flushSync();
        await vi.waitFor(() => expect(document.body.textContent).toContain('Unlock and back up'));
        expect(document.body.textContent).not.toContain('Everything is backed up');
        expect(document.body.textContent).toContain('Enter your encryption password first.');
        const button = [...document.querySelectorAll('button')].find((item) => item.textContent?.includes('Unlock and back up'))!;
        button.click();
        await vi.waitFor(() => expect(mocks.run).toHaveBeenCalledOnce());
    });
});
