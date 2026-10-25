// @vitest-environment happy-dom
import { afterEach, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
    discover: vi.fn(), state: vi.fn(),
    events: new Map<string, (value: unknown) => void>(),
}));
vi.mock('../../api/auto_backup', () => ({
    getAutoBackupState: mocks.state,
    onAutoBackupProgress: () => () => {},
    onAutoBackupState: (cb: (value: unknown) => void) => { mocks.events.set('state', cb); return () => mocks.events.delete('state'); },
    setAutoBackupPowerState: vi.fn(),
}));
vi.mock('../../modules/photo_backup/controller', () => ({ discoverAutomaticPhotoBackup: mocks.discover }));
vi.mock('../../api/photo_backup', () => ({ setPhotoBackupPolicy: vi.fn() }));
vi.mock('../../api/runtime', () => ({
    onRuntimeEvent: (name: string, cb: (value: unknown) => void) => { mocks.events.set(name, cb); return () => mocks.events.delete(name); },
}));
vi.mock('../../modules/photo_backup/native_adapter', () => ({ nativePhotoBackupPolicy: async () => null, nativePowerStatus: async () => null }));
vi.mock('../../modules/notif_bell', () => ({ markTransferDone: vi.fn(), pushTransferStart: vi.fn(), removeTransfer: vi.fn(), updateTransferName: vi.fn(), updateTransferProgress: vi.fn() }));
import { startAutoBackupRuntime } from './auto_backup_runtime';

let stop = () => {};
afterEach(() => { stop(); vi.clearAllMocks(); mocks.events.clear(); });

it('starts native discovery when enabled from the separate Account settings surface', async () => {
    mocks.state.mockResolvedValue({ enabled: false });
    mocks.discover.mockResolvedValue(undefined);
    stop = startAutoBackupRuntime();
    await Promise.resolve();
    mocks.events.get('state')?.({ enabled: true });
    await vi.waitFor(() => expect(mocks.discover).toHaveBeenCalledOnce());
    mocks.events.get('ios:PhotoBackupMediaChanged')?.({});
    expect(mocks.discover).toHaveBeenCalledTimes(2);
});

it('discovers an already-enabled library on startup without waiting for a new photo', async () => {
    mocks.state.mockResolvedValue({ enabled: true });
    stop = startAutoBackupRuntime();
    await vi.waitFor(() => expect(mocks.discover).toHaveBeenCalledOnce());
});
