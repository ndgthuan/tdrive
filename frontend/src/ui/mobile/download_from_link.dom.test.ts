import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';

const mocks = vi.hoisted(() => ({ paste: vi.fn(), probe: vi.fn(), start: vi.fn() }));
vi.mock('@wailsio/runtime', () => ({ Clipboard: { Text: mocks.paste } }));
vi.mock('../../api/media_download', () => ({
    read_tool_status: async () => ({ yt_dlp: { found: true }, ffmpeg: { found: false } }),
    probe_link: mocks.probe, start_download: mocks.start, install_tool: vi.fn(), update_ytdlp: vi.fn(), subscribe_media_setup: () => () => undefined,
}));
vi.mock('../../modules/media_download/native_engine', () => ({ native_engine_present: () => true }));
vi.mock('../../modules/media_download/mobile_jobs', () => ({ start_mobile_downloads: vi.fn() }));
vi.mock('../downloader/destination_picker.svelte', () => ({ default: () => undefined }));
import Sheet from './download_from_link_sheet.svelte';

let app: ReturnType<typeof mount> | undefined;
afterEach(async () => { if (app) await unmount(app); app = undefined; document.body.innerHTML = ''; vi.clearAllMocks(); });

describe('mobile link downloader', () => {
    it('pastes the full shared video URL and lists only usable formats without FFmpeg', async () => {
        mocks.paste.mockResolvedValue('Watch this https://youtu.be/video123?si=copy');
        mocks.probe.mockResolvedValue({ title: 'Example video', uploader: '', duration_s: 10, formats: [
            { choice: '18', height: 360, ext: 'mp4', has_video: true, has_audio: true },
            { choice: '137+ba', height: 1080, ext: 'mp4', has_video: true, has_audio: false },
            { choice: '140', height: 0, ext: 'm4a', has_video: false, has_audio: true },
        ] });
        app = mount(Sheet, { target: document.body, props: { open: true, page: true, onclose: vi.fn() } });
        flushSync();
        await vi.waitFor(() => expect(document.querySelector<HTMLButtonElement>('.chip')?.disabled).toBe(false));
        document.querySelector<HTMLButtonElement>('.chip')!.click();
        await vi.waitFor(() => expect(document.querySelector<HTMLInputElement>('input[type=url]')?.value).toBe('https://youtu.be/video123?si=copy'));
        document.querySelector<HTMLButtonElement>('button.secondary')!.click();
        await vi.waitFor(() => expect(document.body.textContent).toContain('Example video'));
        expect(mocks.probe).toHaveBeenCalledWith('https://youtu.be/video123?si=copy');
        expect(document.querySelector('input[value="18"]')).not.toBeNull();
        expect(document.querySelector('input[value="137+ba"]')).toBeNull();
        expect(document.querySelector('input[value="1080p"]')).toBeNull();
        expect(document.querySelector('input[value="audio_mp3"]')).toBeNull();
        expect(document.querySelector<HTMLInputElement>('input[value="18"]')?.checked).toBe(true);
        expect(document.querySelector('[role="dialog"]')).toBeNull();
    });
});
