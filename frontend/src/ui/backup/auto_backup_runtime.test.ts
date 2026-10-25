// @vitest-environment happy-dom
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

const mocks = vi.hoisted(() => ({
    discover: vi.fn(), state: vi.fn(), progress: vi.fn(), start: vi.fn(), update: vi.fn(), done: vi.fn(),
    events: new Map<string, (value: unknown) => void>(),
}));
vi.mock('../../api/auto_backup', () => ({
    getAutoBackupState: mocks.state,
    getAutoBackupProgress: mocks.progress,
    onAutoBackupProgress: () => () => {},
    onAutoBackupState: (cb: (value: unknown) => void) => { mocks.events.set('state', cb); return () => mocks.events.delete('state'); },
    setAutoBackupPowerState: vi.fn(),
}));
vi.mock('../../modules/photo_backup/controller', async () => {
    const { writable } = await import('svelte/store');
    return { discoverAutomaticPhotoBackup: mocks.discover, photoBackupState: writable(null) };
});
vi.mock('../../api/photo_backup', () => ({ setPhotoBackupPolicy: vi.fn() }));
vi.mock('../../api/runtime', () => ({
    onRuntimeEvent: (name: string, cb: (value: unknown) => void) => { mocks.events.set(name, cb); return () => mocks.events.delete(name); },
}));
vi.mock('../../modules/photo_backup/native_adapter', () => ({ nativePhotoBackupPolicy: async () => null, nativePowerStatus: async () => null }));
vi.mock('../../modules/notif_bell', () => ({ markTransferDone: mocks.done, pushTransferStart: mocks.start, removeTransfer: vi.fn(), updateTransferName: vi.fn(), updateTransferProgress: mocks.update }));
import { autoBackupProgress, startAutoBackupRuntime } from './auto_backup_runtime';

let stop = () => {};
beforeEach(() => { Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' }); mocks.progress.mockResolvedValue({ enabled: true, paused: false, done: 632, total: 823, currentName: 'IMG_1234.JPG', currentBytes: 6000000, currentBytesDone: 3000000 }); });
afterEach(() => { stop(); vi.useRealTimers(); vi.clearAllMocks(); mocks.events.clear(); });

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
    await vi.waitFor(() => expect(get(autoBackupProgress)?.done).toBe(632));
    expect(mocks.start).toHaveBeenCalledWith(expect.objectContaining({ total: 0 }));
    expect(mocks.update).toHaveBeenCalledWith(expect.objectContaining({ bytes: 0, total: 0, itemsDone: 632, itemsTotal: 823, items: [expect.objectContaining({ progress: 50 })] }));
});

it('keeps progress live without a toggle or backend progress event', async () => {
    vi.useFakeTimers();
    mocks.state.mockResolvedValue({ enabled: true });
    stop = startAutoBackupRuntime();
    await vi.advanceTimersByTimeAsync(0);
    expect(get(autoBackupProgress)?.done).toBe(632);
    mocks.progress.mockResolvedValue({ enabled: true, paused: false, done: 650, total: 823 });
    await vi.advanceTimersByTimeAsync(1500);
    expect(get(autoBackupProgress)?.done).toBe(650);
    mocks.events.get('state')?.({ enabled: false });
    expect(get(autoBackupProgress)).toBeNull();
    expect(mocks.done).toHaveBeenCalledWith({ id: 'auto-backup', direction: 'up', status: 'stopped' });
});

it('ignores an in-flight enabled snapshot after backup is switched off', async () => {
    let resolveProgress!: (value: unknown) => void;
    mocks.progress.mockReturnValue(new Promise((resolve) => { resolveProgress = resolve; }));
    mocks.state.mockResolvedValue({ enabled: true });
    stop = startAutoBackupRuntime();
    await vi.waitFor(() => expect(mocks.progress).toHaveBeenCalled());
    mocks.events.get('state')?.({ enabled: false });
    resolveProgress({ enabled: true, paused: false, done: 5, total: 10 });
    await Promise.resolve();
    expect(get(autoBackupProgress)).toBeNull();
    expect(mocks.start).not.toHaveBeenCalled();
});
