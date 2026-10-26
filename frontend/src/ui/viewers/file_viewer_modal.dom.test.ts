import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, tick, unmount } from 'svelte';
import FileViewerModal from './file_viewer_modal.svelte';
import { closeFileViewerView, openFileViewerView } from './file_viewer_store';

Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
    configurable: true,
    get() {
        return this.parentElement;
    },
});

let host: HTMLElement;
let app: Record<string, unknown> | null = null;

async function settle(): Promise<void> {
    flushSync();
    for (let i = 0; i < 2; i += 1) {
        await Promise.resolve();
        await tick();
        await new Promise((resolve) => { setTimeout(resolve, 0); });
    }
    flushSync();
}

async function waitForElement<T extends Element>(selector: string): Promise<T> {
    const deadline = Date.now() + 3_000;
    while (Date.now() < deadline) {
        await settle();
        const element = host.querySelector<T>(selector);
        if (element) return element;
        await new Promise((resolve) => { setTimeout(resolve, 25); });
    }
    throw new Error(`Expected ${selector} to render. DOM: ${host.innerHTML}`);
}

function mountViewer(): void {
    app = mount(FileViewerModal, {
        target: host,
        props: {
            onClose: vi.fn(),
            onDownload: vi.fn(),
        },
    });
}

beforeEach(() => {
    host = document.createElement('div');
    host.id = 'viewer-modal';
    document.body.appendChild(host);
});

afterEach(async () => {
    closeFileViewerView();
    flushSync();
    vi.unstubAllGlobals();
    if (app) await unmount(app);
    host.remove();
    app = null;
});

describe('FileViewerModal source (channel) viewing', () => {
    it('waits for an audio stream URL before mounting the media element', async () => {
        const audioView = {
            kind: 'audio' as const, token: '', url: '', title: 'sample.mp3',
            meta: 'MP3', mimeType: 'audio/mpeg', loading: true, error: '',
        };
        openFileViewerView(audioView);
        mountViewer();
        await waitForElement('.audio-viewer');
        expect(host.querySelector('audio')).toBeNull();
        expect(host.querySelector('.audio-subtitle')?.textContent).toBe('Opening');
        openFileViewerView({ ...audioView, token: 'sample', url: 'http://127.0.0.1/media/sample', loading: false });
        const audio = await waitForElement<HTMLAudioElement>('audio');
        audio.dispatchEvent(new Event('error'));
        await settle();
        expect(host.querySelector('.audio-subtitle')?.textContent).toBe('Could not load this audio stream');
    });
    function open_image(readOnly: boolean, downloadable = false, protectedContent = false): void {
        openFileViewerView({
            kind: 'image',
            token: 'tok',
            url: 'http://127.0.0.1/media/file/tok',
            title: 'photo.jpg',
            meta: 'JPG · 1.0 KB',
            mimeType: 'image/jpeg',
            loading: false,
            error: '',
            readOnly,
            downloadable,
            protected: protectedContent,
        });
    }

    it('hides the download control and blocks save gestures for read-only media', async () => {
        open_image(true);
        mountViewer();
        const stage = await waitForElement<HTMLElement>('.file-viewer-stage');
        await settle();

        expect(host.querySelector('[aria-label="Download"]')).toBeNull();
        expect(host.querySelector('img')?.getAttribute('draggable')).toBe('false');
        const menu = new Event('contextmenu', { bubbles: true, cancelable: true });
        stage.dispatchEvent(menu);
        expect(menu.defaultPrevented).toBe(true);
    });

    it('offers a save for a source post only when its source allows it', async () => {
        open_image(true, true);
        mountViewer();
        await waitForElement<HTMLElement>('.file-viewer-stage');
        await settle();
        expect(host.querySelector('[aria-label="Download"]')).not.toBeNull();
    });

    it('never offers a save for protected source content, even when downloadable is set', async () => {
        open_image(true, true, true);
        mountViewer();
        await waitForElement<HTMLElement>('.file-viewer-stage');
        await settle();
        expect(host.querySelector('[aria-label="Download"]')).toBeNull();
    });

    it('leaves the drive viewer alone: download stays and the image is still draggable', async () => {
        open_image(false);
        mountViewer();
        await waitForElement<HTMLElement>('.file-viewer-stage');
        await settle();

        expect(host.querySelector('[aria-label="Download"]')).not.toBeNull();
        const image = await waitForElement<HTMLImageElement>('img');
        expect(image.getAttribute('draggable')).not.toBe('false');
    });
});

describe('FileViewerModal text behavior', () => {
    it('renders plain text files after the initial chunk load', async () => {
        vi.stubGlobal('fetch', vi.fn(async () => {
            return new Response('Alpha\nBeta\nGamma\n', {
                status: 206,
                headers: {
                    'Content-Range': 'bytes 0-16/17',
                },
            });
        }));

        openFileViewerView({
            kind: 'text',
            token: 'tok',
            url: 'http://127.0.0.1/media/file/tok',
            title: 'notes.txt',
            meta: 'TXT · 17 B',
            mimeType: 'text/plain; charset=utf-8',
            loading: false,
            error: '',
        });

        mountViewer();
        await settle();

        const body = host.querySelector('.text-viewer-body');
        expect(body?.textContent).toContain('Alpha');
        expect(body?.textContent).toContain('Gamma');
        expect(host.textContent).toContain('End of file');
    });

    it('lets markdown switch between rendered and raw views without refetching', async () => {
        const fetchMock = vi.fn(async () => {
            return new Response('# Hello\n\n**world**\n', {
                status: 206,
                headers: {
                    'Content-Range': 'bytes 0-18/19',
                },
            });
        });
        vi.stubGlobal('fetch', fetchMock);

        openFileViewerView({
            kind: 'text',
            token: 'tok',
            url: 'http://127.0.0.1/media/file/tok',
            title: 'README.md',
            meta: 'MD · 19 B',
            mimeType: 'text/plain; charset=utf-8',
            loading: false,
            error: '',
        });

        mountViewer();

        const heading = await waitForElement('.text-viewer-body.is-markdown h1');
        expect(heading.textContent).toBe('Hello');

        const buttons = host.querySelectorAll<HTMLButtonElement>('.text-viewer-toggle button');
        expect(buttons).toHaveLength(2);
        expect(host.querySelector('.file-viewer-actions .text-viewer-toggle')).not.toBeNull();
        expect(host.querySelector('.text-viewer-footer .text-viewer-toggle')).toBeNull();
        expect(host.querySelector('.text-viewer-footer')?.textContent ?? '').not.toContain('100%');
        expect(host.querySelector('.text-viewer-footer')?.textContent ?? '').not.toContain('Preview ready');
        buttons[1].dispatchEvent(new MouseEvent('click', { bubbles: true }));
        flushSync();

        const rawBody = host.querySelector('pre.text-viewer-body');
        expect(rawBody?.textContent).toContain('# Hello');
        expect(rawBody?.textContent).toContain('**world**');
        expect(fetchMock).toHaveBeenCalledTimes(1);
    });
});
