import { beforeEach, describe, expect, it, vi } from 'vitest';

const bindings = vi.hoisted(() => ({
    DownloadFile: vi.fn(),
    DownloadFolder: vi.fn(),
    DownloadChannelSourceMedia: vi.fn(),
    ListResumableDownloads: vi.fn(async () => ({ result: { ok: true }, jobs: [] as unknown[] })),
    ResumeDownload: vi.fn(),
    SelectFiles: vi.fn(async () => []),
}));
const passwordModal = vi.hoisted(() => vi.fn(async () => false));
const transferEvents = vi.hoisted(() => ({
    queued: vi.fn(),
    push: vi.fn(),
    progress: vi.fn(),
    rename: vi.fn(),
    done: vi.fn(),
    note: vi.fn(),
}));
const notifications = vi.hoisted(() => ({ notify: vi.fn(), dismissNotification: vi.fn() }));

// Go always emits an event's payload as a JSON array of its original args
// (see RuntimeEventMap in api/runtime.ts); mimic that instead of the removed
// window.runtime.EventsOn bridge.
const eventListeners = vi.hoisted(() => new Map<string, (...args: unknown[]) => void>());
const eventsOn = vi.hoisted(() => vi.fn((eventName: string, callback: (event: { name: string; data: unknown[] }) => void) => {
    eventListeners.set(eventName, (...args: unknown[]) => callback({ name: eventName, data: args }));
    return () => eventListeners.delete(eventName);
}));

vi.mock('../../bindings/TDrive/internal/app/app', () => bindings);
vi.mock('@wailsio/runtime', () => ({ Events: { On: eventsOn } }));
vi.mock('./notif_bell', () => ({
    forgetResumableUpload: vi.fn(),
    setTransferNote: transferEvents.note,
    transferKey: (direction: string, id: string | number) => `xfer:${direction}:${id}`,
    pushQueuedTransfer: transferEvents.queued,
    pushTransferStart: transferEvents.push,
    updateTransferProgress: transferEvents.progress,
    updateTransferName: transferEvents.rename,
    markTransferDone: transferEvents.done,
    wasUploadCanceled: () => false,
}));
vi.mock('./notifications', () => ({
    notify: notifications.notify,
    dismissNotification: notifications.dismissNotification,
}));
vi.mock('./encryption', () => ({ loadEncryptionStatus: vi.fn(async () => undefined) }));
vi.mock('./modals/upload_options', () => ({ openUploadOptionsModal: vi.fn() }));
vi.mock('./modals/import_options', () => ({ openImportOptionsModal: vi.fn() }));
vi.mock('./modals/encryption_setup', () => ({ openEncryptionSetupModal: vi.fn() }));
vi.mock('./modals/encryption_password', () => ({ openEncryptionPasswordModal: passwordModal }));

type DownloadBindingResult = {
    result:
        | { ok: true }
        | { ok: false; error: { code: string; message: string } };
    saved_path: string;
};

function downloadSuccess(savedPath: string): DownloadBindingResult {
    return { result: { ok: true }, saved_path: savedPath };
}

beforeEach(() => {
    vi.clearAllMocks();
    bindings.DownloadFile.mockResolvedValue(downloadSuccess('/tmp/file'));
    bindings.DownloadFolder.mockResolvedValue(downloadSuccess('/tmp/folder'));
    bindings.ListResumableDownloads.mockResolvedValue({ result: { ok: true }, jobs: [] });
    passwordModal.mockResolvedValue(false);
});


const SOURCE = { peerKind: 'channel', peerId: 51, accountId: '7', generation: 'g-1', title: 'Field Recordings' };

async function loadSourceModule() {
    vi.resetModules();
    eventListeners.clear();
    const { idleTransferActivity, state } = await import('../state');
    state.downloadQueue = [];
    state.activeDownloadId = null;
    state.activeDownloadRequestId = null;
    state.transferActivity = idleTransferActivity;
    state.cancelingDownload = false;
    state.activeChannel = { id: 101, title: 'Drive A', kind: 'personal' };
    const mod = await import('./transfers');
    mod.activateTransferSurfaces();
    return { mod, state };
}

beforeEach(() => {
    vi.clearAllMocks();
    bindings.DownloadChannelSourceMedia.mockResolvedValue(downloadSuccess('/home/me/Downloads/clip.mp4'));
});

describe('source post download queue', () => {
    it('sends the post through its own source scope and queues it as a source row', async () => {
        const { mod, state } = await loadSourceModule();
        mod.enqueueSourceDownload(SOURCE, 71, 'forest-dawn.mp4', 2048);

        expect(transferEvents.queued).toHaveBeenCalledWith({
            id: 'source:51:channel:71', direction: 'down', name: 'forest-dawn.mp4', total: 2048,
        });
        await vi.waitFor(() => expect(bindings.DownloadChannelSourceMedia).toHaveBeenCalledWith(
            'channel', 51, 71, 7, 'g-1', expect.stringMatching(/^source:51:channel:71@/),
        ));
        await vi.waitFor(() => expect(state.downloadQueue).toEqual([]));
    });

    it('records the chosen save path on the transfer row and marks it done', async () => {
        const { mod } = await loadSourceModule();
        mod.enqueueSourceDownload(SOURCE, 71, 'forest-dawn.mp4', 2048);

        await vi.waitFor(() => expect(transferEvents.note).toHaveBeenCalledWith(expect.objectContaining({
            id: 'source:51:channel:71', direction: 'down', note: expect.stringContaining('/home/me/Downloads/clip.mp4'),
        })));
        expect(transferEvents.done).toHaveBeenCalledWith({ id: 'source:51:channel:71', direction: 'down', status: 'done' });
    });

    it('treats a canceled save as canceled, without an error toast', async () => {
        bindings.DownloadChannelSourceMedia.mockResolvedValue({
            result: { ok: false, error: { code: 'canceled', message: 'Download canceled' } }, saved_path: '',
        });
        const { mod } = await loadSourceModule();
        mod.enqueueSourceDownload(SOURCE, 71, 'forest-dawn.mp4', 2048);

        await vi.waitFor(() => expect(transferEvents.done).toHaveBeenCalledWith({ id: 'source:51:channel:71', direction: 'down', status: 'canceled' }));
        expect(notifications.notify).not.toHaveBeenCalled();
    });

    it('reports a failed save as an error toast with a retry that re-queues it', async () => {
        bindings.DownloadChannelSourceMedia.mockResolvedValueOnce({
            result: { ok: false, error: { code: 'unavailable', message: 'Telegram source is no longer available' } }, saved_path: '',
        });
        const { mod } = await loadSourceModule();
        mod.enqueueSourceDownload(SOURCE, 71, 'forest-dawn.mp4', 2048);

        await vi.waitFor(() => expect(notifications.notify).toHaveBeenCalledWith(expect.objectContaining({ level: 'error' })));
        const toast = notifications.notify.mock.calls[0][0] as { action: { run: () => void } };
        toast.action.run();
        await vi.waitFor(() => expect(bindings.DownloadChannelSourceMedia).toHaveBeenCalledTimes(2));
    });

    it('ignores a post with no message id before queuing anything', async () => {
        const { mod, state } = await loadSourceModule();
        mod.enqueueSourceDownload(SOURCE, 0, 'nothing.mp4', 1);
        expect(state.downloadQueue).toEqual([]);
        expect(bindings.DownloadChannelSourceMedia).not.toHaveBeenCalled();
    });
});
