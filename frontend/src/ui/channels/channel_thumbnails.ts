import { load_channel_media_thumbnail } from '../../api/channel_sources';
import { sourceKey, type ChannelMediaItem, type ChannelSource } from './channel_model';

/** Previews are fetched a few at a time, so a long grid does not flood Telegram. */
const MAX_IN_FLIGHT = 3;
/** Each preview is a data URL of a few tens of kilobytes; the oldest go first. */
const MAX_CACHED = 300;

/** A preview is a photo or video post. Other posts keep their type icon. */
export function wants_thumbnail(item: ChannelMediaItem): boolean {
    return (item.kind === 'image' || item.kind === 'video') && !item.blockReason;
}

export function thumbnail_key(source: Pick<ChannelSource, 'accountId' | 'peerKind' | 'peerId' | 'generation'>, msgId: number): string {
    return `${sourceKey(source)}:${msgId}`;
}

const cache = new Map<string, string>();
const pending = new Map<string, Promise<string>>();
const queue: Array<() => void> = [];
let in_flight = 0;

function remember(key: string, url: string): void {
    cache.delete(key);
    cache.set(key, url);
    if (cache.size <= MAX_CACHED) return;
    const oldest = cache.keys().next().value;
    if (oldest !== undefined) cache.delete(oldest);
}

function take_slot(): Promise<void> {
    if (in_flight < MAX_IN_FLIGHT) {
        in_flight += 1;
        return Promise.resolve();
    }
    return new Promise((resolve) => queue.push(() => { in_flight += 1; resolve(); }));
}

function release_slot(): void {
    in_flight -= 1;
    queue.shift()?.();
}

/**
 * The preview for one post as a data URL, or '' when there is none. A preview
 * that already arrived is kept for the session; a failed request is not kept,
 * so the next time the post scrolls into view it is asked for again.
 */
export function request_thumbnail(source: ChannelSource, msgId: number): Promise<string> {
    const key = thumbnail_key(source, msgId);
    const known = cache.get(key);
    if (known !== undefined) return Promise.resolve(known);
    const running = pending.get(key);
    if (running) return running;
    const task = (async () => {
        await take_slot();
        try {
            const url = await load_channel_media_thumbnail(source, msgId);
            remember(key, url);
            return url;
        } catch {
            return '';
        } finally {
            release_slot();
            pending.delete(key);
        }
    })();
    pending.set(key, task);
    return task;
}

/** Calls onLoad once the post's preview is known, when the element scrolls into view. */
export type ThumbnailWatch = { source: ChannelSource; msgId: number; onLoad: (url: string) => void };

/** A null watch does nothing, so a post without a preview can share the same markup. */
export function watch_thumbnail(node: HTMLElement, options: ThumbnailWatch | null) {
    let current: ThumbnailWatch | null = options;
    let observer: IntersectionObserver | undefined;
    let requested = false;

    function stop(): void {
        observer?.disconnect();
        observer = undefined;
    }

    function ask(watch: ThumbnailWatch): void {
        const { source, msgId, onLoad } = watch;
        void request_thumbnail(source, msgId).then((url) => {
            if (url && current?.msgId === msgId && sourceKey(current.source) === sourceKey(source)) onLoad(url);
        });
    }

    function start(): void {
        stop();
        requested = false;
        const watch = current;
        if (!watch) return;
        const known = cache.get(thumbnail_key(watch.source, watch.msgId));
        if (known !== undefined) {
            if (known) watch.onLoad(known);
            return;
        }
        if (typeof IntersectionObserver === 'undefined') {
            ask(watch);
            return;
        }
        observer = new IntersectionObserver((entries) => {
            if (requested || !entries.some((entry) => entry.isIntersecting)) return;
            requested = true;
            stop();
            ask(watch);
        }, { rootMargin: '240px 0px' });
        observer.observe(node);
    }

    start();
    return {
        update(next: ThumbnailWatch | null) {
            const same = next === null || current === null
                ? next === current
                : next.msgId === current.msgId && sourceKey(next.source) === sourceKey(current.source);
            current = next;
            if (!same) start();
        },
        destroy() {
            stop();
            current = null;
        },
    };
}
