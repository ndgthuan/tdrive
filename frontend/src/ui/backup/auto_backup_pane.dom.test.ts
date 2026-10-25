import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';

const state = { enabled: true, permission: 'undetermined', platform: 'ios', phase: 'needs_permission', folderName: 'iPhone', defaultFolderName: 'iPhone', folderNameCustom: false, wifiOnly: false, chargingOnly: false, errors: [] };
const mocks = vi.hoisted(() => ({ read: vi.fn(), request: vi.fn(), permission: vi.fn(), run: vi.fn(), get: vi.fn() }));
vi.mock('../../api/auto_backup', () => ({
    getAutoBackupState: mocks.get, readNativeDeviceIdentity: async () => null,
    setAutoBackupPermission: mocks.permission, backUpAutoBackupNow: mocks.run,
    setAutoBackupEnabled: vi.fn(), setAutoBackupChargingOnly: vi.fn(), setAutoBackupDeviceIdentity: vi.fn(),
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

let app: ReturnType<typeof mount> | undefined;
afterEach(async () => { if (app) await unmount(app); app = undefined; document.body.innerHTML = ''; vi.clearAllMocks(); });

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
});
