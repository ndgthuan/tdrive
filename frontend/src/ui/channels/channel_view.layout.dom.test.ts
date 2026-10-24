import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, tick, unmount } from 'svelte';

const load = vi.hoisted(() => vi.fn<(source: unknown, msgId: number) => Promise<string>>());
vi.mock('../../api/channel_sources', () => ({ load_channel_media_thumbnail: load }));

import ChannelView from './channel_view.svelte';
import { SOURCE_VIEW_STORAGE_KEY } from './channel_view_mode';
import type { ChannelMediaFetcher, ChannelMediaItem, ChannelSource } from './channel_model';

const SOURCE: ChannelSource = { peerKind: 'channel', peerId: 61, title: 'Dawn Reels', username: 'dawnreels', connected: true, protected: false, available: true, accountId: '7', generation: 'layout' };
const CLIP: ChannelMediaItem = {
    msgId: 81, date: 1_735_689_600, name: 'ridge.mp4', size: 3_145_728, duration: 95, mimeType: 'video/mp4', kind: 'video',
    caption: 'Ridge at dawn', streamable: true, blockReason: '', telegramUrl: 'https://t.me/dawnreels/81',
};
const DOC: ChannelMediaItem = { ...CLIP, msgId: 80, kind: 'unknown', name: 'notes.pdf', caption: '', streamable: false, blockReason: 'unsupported_format', size: 1024 };

let app: Record<string, unknown> | null = null;
let host: HTMLElement | null = null;

async function settle(): Promise<void> {
    for (let index = 0; index < 8; index += 1) {
        flushSync();
        await tick();
        await Promise.resolve();
    }
    flushSync();
}

function answering(...items: ChannelMediaItem[]): ChannelMediaFetcher {
    return vi.fn(async (request) => ({ peerKind: request.peerKind, peerId: request.peerId, accountId: '7', generation: 'layout', items, nextOffsetId: 0, hasMore: false }));
}

function render(fetchMedia: ChannelMediaFetcher) {
    const props = { source: SOURCE, fetchMedia, onOpenPost: vi.fn(), onOpenTelegram: vi.fn(), onPostActions: vi.fn() };
    host = document.createElement('div');
    document.body.append(host);
    app = mount(ChannelView, { target: host, props });
    return props;
}

function toggle(label: 'Grid' | 'List'): HTMLButtonElement {
    const found = host?.querySelector<HTMLButtonElement>(`.channel-view-toggle button[aria-label="${label}"]`);
    if (!found) throw new Error(`no ${label} toggle`);
    return found;
}

afterEach(async () => {
    vi.unstubAllGlobals();
    load.mockReset();
    if (app) await unmount(app);
    host?.remove();
    app = null;
    host = null;
});

describe('ChannelView layouts', () => {
    it('opens as a grid and switches to a table of rows, remembering the choice', async () => {
        const storage = new Map<string, string>();
        vi.stubGlobal('localStorage', { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value) });
        vi.stubGlobal('IntersectionObserver', class { observe(): void {} disconnect(): void {} });
        render(answering(CLIP, DOC));
        await settle();
        const section = host?.querySelector('.channel-view');
        expect(section?.classList.contains('is-grid')).toBe(true);
        expect(toggle('Grid').getAttribute('aria-pressed')).toBe('true');

        toggle('List').click();
        await settle();
        expect(section?.classList.contains('is-list')).toBe(true);
        expect(toggle('List').getAttribute('aria-pressed')).toBe('true');
        expect(storage.get(SOURCE_VIEW_STORAGE_KEY)).toBe('list');
        const columns = Array.from(host?.querySelectorAll('.channel-row-main') ?? []).map((row) => ({
            kind: row.querySelector('.channel-row-kind')?.textContent?.trim(),
            size: row.querySelector('.channel-row-size')?.textContent?.trim(),
        }));
        expect(columns.map((column) => column.kind)).toEqual(['Video', 'PDF']);
        expect(columns[0]?.size).toMatch(/MB/);
    });

    it('reopens in the list when that was the last choice', async () => {
        vi.stubGlobal('localStorage', { getItem: () => 'list', setItem: () => undefined });
        vi.stubGlobal('IntersectionObserver', class { observe(): void {} disconnect(): void {} });
        render(answering(CLIP));
        await settle();
        expect(host?.querySelector('.channel-view')?.classList.contains('is-list')).toBe(true);
        expect(toggle('List').getAttribute('aria-pressed')).toBe('true');
    });

    it('opens a row the same way from either layout', async () => {
        vi.stubGlobal('localStorage', { getItem: () => 'list', setItem: () => undefined });
        vi.stubGlobal('IntersectionObserver', class { observe(): void {} disconnect(): void {} });
        const props = render(answering(CLIP));
        await settle();
        host?.querySelector<HTMLButtonElement>('.channel-row-main')?.click();
        expect(props.onOpenPost).toHaveBeenCalledWith(CLIP, SOURCE, [CLIP]);
    });

    it('shows the preview the backend sends for a video, and keeps the type icon for a document', async () => {
        load.mockResolvedValue('data:image/jpeg;base64,PREVIEW');
        vi.stubGlobal('IntersectionObserver', class {
            constructor(private readonly callback: IntersectionObserverCallback) {}
            observe(): void { this.callback([{ isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver); }
            disconnect(): void {}
        });
        render(answering(CLIP, DOC));
        await vi.waitFor(() => expect(host?.querySelector('img.channel-chip-preview')).not.toBeNull());
        await settle();
        expect(host?.querySelector<HTMLImageElement>('img.channel-chip-preview')?.getAttribute('src')).toBe('data:image/jpeg;base64,PREVIEW');
        expect(load).toHaveBeenCalledTimes(1);
        expect(load).toHaveBeenCalledWith(expect.objectContaining({ peerId: 61 }), 81);
    });
});
