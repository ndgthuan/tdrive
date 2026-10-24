import { afterEach, describe, expect, it, vi } from 'vitest';

const load = vi.hoisted(() => vi.fn<(source: unknown, msgId: number) => Promise<string>>());
vi.mock('../../api/channel_sources', () => ({ load_channel_media_thumbnail: load }));

import { request_thumbnail, thumbnail_key, wants_thumbnail, watch_thumbnail } from './channel_thumbnails';
import type { ChannelMediaItem, ChannelSource } from './channel_model';

// Each test gets its own account, so the module cache from one test cannot answer another.
let account = 100;
function source_for(): ChannelSource {
    account += 1;
    return { peerKind: 'channel', peerId: 9, title: 'Reels', username: '', connected: true, protected: false, available: true, accountId: String(account), generation: 'g' };
}

function item(kind: string, extra: Partial<ChannelMediaItem> = {}): ChannelMediaItem {
    return { msgId: 1, date: 0, name: 'x', size: 1, duration: 0, mimeType: '', kind, caption: '', streamable: true, blockReason: '', telegramUrl: '', ...extra };
}

function deferred<T>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>((res) => { resolve = res; });
    return { promise, resolve };
}

afterEach(() => {
    load.mockReset();
    vi.unstubAllGlobals();
});

describe('thumbnail wants', () => {
    it('asks only for photo and video posts that are not withheld', () => {
        expect(wants_thumbnail(item('image'))).toBe(true);
        expect(wants_thumbnail(item('video'))).toBe(true);
        expect(wants_thumbnail(item('pdf'))).toBe(false);
        expect(wants_thumbnail(item('audio'))).toBe(false);
        expect(wants_thumbnail(item('video', { blockReason: 'paid' }))).toBe(false);
    });
});

describe('request_thumbnail', () => {
    it('keeps a preview for the session instead of asking again', async () => {
        const source = source_for();
        load.mockResolvedValue('data:image/jpeg;base64,AAAA');
        await expect(request_thumbnail(source, 5)).resolves.toBe('data:image/jpeg;base64,AAAA');
        await expect(request_thumbnail(source, 5)).resolves.toBe('data:image/jpeg;base64,AAAA');
        expect(load).toHaveBeenCalledTimes(1);
    });

    it('shares one request between two callers for the same post', async () => {
        const source = source_for();
        const answer = deferred<string>();
        load.mockReturnValue(answer.promise);
        const first = request_thumbnail(source, 6);
        const second = request_thumbnail(source, 6);
        answer.resolve('data:image/jpeg;base64,BBBB');
        await expect(Promise.all([first, second])).resolves.toEqual(['data:image/jpeg;base64,BBBB', 'data:image/jpeg;base64,BBBB']);
        expect(load).toHaveBeenCalledTimes(1);
    });

    it('does not keep a failed request, so the next view asks again', async () => {
        const source = source_for();
        load.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce('data:image/jpeg;base64,CCCC');
        await expect(request_thumbnail(source, 7)).resolves.toBe('');
        await expect(request_thumbnail(source, 7)).resolves.toBe('data:image/jpeg;base64,CCCC');
        expect(load).toHaveBeenCalledTimes(2);
    });

    it('keeps no more than three requests in flight', async () => {
        const source = source_for();
        const answers = Array.from({ length: 5 }, () => deferred<string>());
        load.mockImplementation((_source, msgId) => answers[msgId - 1]!.promise);
        const pending = [1, 2, 3, 4, 5].map((msgId) => request_thumbnail(source, msgId));
        await vi.waitFor(() => expect(load).toHaveBeenCalledTimes(3));
        answers[0]!.resolve('data:image/jpeg;base64,1');
        await vi.waitFor(() => expect(load).toHaveBeenCalledTimes(4));
        answers.forEach((answer) => answer.resolve(''));
        await Promise.all(pending);
        expect(load).toHaveBeenCalledTimes(5);
    });
});

describe('watch_thumbnail', () => {
    it('asks for the preview only once the tile is on screen', async () => {
        const source = source_for();
        load.mockResolvedValue('data:image/jpeg;base64,DDDD');
        let visible: ((entries: IntersectionObserverEntry[]) => void) | undefined;
        const observe = vi.fn();
        vi.stubGlobal('IntersectionObserver', class {
            constructor(callback: (entries: IntersectionObserverEntry[]) => void) { visible = callback; }
            observe = observe;
            disconnect(): void {}
        });
        const onLoad = vi.fn();
        const node = document.createElement('span');
        const action = watch_thumbnail(node, { source, msgId: 8, onLoad });
        expect(observe).toHaveBeenCalledWith(node);
        expect(load).not.toHaveBeenCalled();
        visible?.([{ isIntersecting: false } as IntersectionObserverEntry]);
        expect(load).not.toHaveBeenCalled();
        visible?.([{ isIntersecting: true } as IntersectionObserverEntry]);
        await vi.waitFor(() => expect(onLoad).toHaveBeenCalledWith('data:image/jpeg;base64,DDDD'));
        expect(load).toHaveBeenCalledTimes(1);
        action.destroy();
    });

    it('does nothing for a post without a preview', () => {
        const observe = vi.fn();
        vi.stubGlobal('IntersectionObserver', class {
            observe = observe;
            disconnect(): void {}
        });
        const action = watch_thumbnail(document.createElement('span'), null);
        expect(observe).not.toHaveBeenCalled();
        expect(load).not.toHaveBeenCalled();
        action.destroy();
    });

    it('keys previews by channel and post', () => {
        const source = source_for();
        expect(thumbnail_key(source, 3)).toBe(`${source.accountId}:channel:9:g:3`);
    });
});
