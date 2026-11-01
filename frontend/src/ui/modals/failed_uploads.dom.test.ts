import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import FailedUploadsModal from './failed_uploads_modal.svelte';
import { failedUploadsModal } from './failed_uploads_store';

let host: HTMLElement;
let component: ReturnType<typeof mount>;
const retry = vi.fn();

beforeEach(() => {
    retry.mockClear();
    host = document.createElement('div');
    host.id = 'failed-uploads-modal';
    document.body.appendChild(host);
    component = mount(FailedUploadsModal, { target: host, props: { onClose: () => failedUploadsModal.close(), onRetry: retry } });
});

afterEach(async () => {
    failedUploadsModal.close();
    flushSync();
    await unmount(component);
    host.remove();
});

function open(failures: { index: number; name: string; message: string; code: string; retryable: boolean }[]) {
    failedUploadsModal.open({ paths: [], failures, parentId: 'folder', encrypt: false, driveId: 1, driveTitle: 'Personal' });
    flushSync();
}

describe('failed upload review', () => {
    it('copies one name and reason per line, and says how many more were left out', async () => {
        const writeText = vi.fn(async (_text: string) => undefined);
        Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
        failedUploadsModal.open({
            paths: [],
            failures: [
                { index: 0, name: 'Camera/a.jpg', message: 'could not be read (the file may be a temporary or deleted file)', code: 'unreadable', retryable: true },
                { index: 1, name: 'b.mp4', message: 'quota exceeded', code: 'upload_failed', retryable: true },
            ],
            parentId: 'folder', encrypt: false, driveId: 1, driveTitle: 'Personal', omitted: 4998,
        });
        flushSync();

        const copy = [...host.querySelectorAll('button')].find((button) => button.textContent?.trim() === 'Copy list');
        copy?.click();
        await vi.waitFor(() => expect(writeText).toHaveBeenCalled());

        const text = writeText.mock.calls[0][0];
        const lines = text.split('\n');
        expect(lines[0]).toBe('Camera/a.jpg: could not be read (the file may be a temporary or deleted file)');
        expect(lines[1]).toMatch(/^b\.mp4: /);
        expect(lines[2]).toBe('and 4998 more');
        expect(lines).toHaveLength(3);
    });

    it('shows every error with bounded pages and distinguishes safe retry from uncertain acceptance', () => {
        open(Array.from({ length: 22 }, (_, index) => ({ index, name: `file-${index}.txt`, message: `error ${index}`, code: index === 21 ? 'outcome_unknown' : 'upload_failed', retryable: index !== 21 })));
        expect(host.querySelectorAll('li')).toHaveLength(20);
        const next = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Next');
        next?.click();
        flushSync();
        expect(host.querySelectorAll('li')).toHaveLength(2);
        expect(host.textContent).toContain('file-21.txt');
        expect(host.textContent).toContain('Ready to retry');
        expect(host.textContent).toContain('Check the folder before uploading it again');
        const button = host.querySelector<HTMLButtonElement>('#retry-failed-uploads');
        expect(button?.textContent).toContain('(21)');
        button?.click();
        expect(retry).toHaveBeenCalledTimes(1);
    });

    it('disables fresh retries when outcomes are uncertain or owned by resumable recovery', () => {
        open([
            { index: 0, name: 'unknown.txt', message: 'lost receipt', code: 'outcome_unknown', retryable: false },
            { index: 1, name: 'movie.mkv', message: 'paused', code: 'resumable_upload', retryable: false },
        ]);
        expect(host.querySelector<HTMLButtonElement>('#retry-failed-uploads')?.disabled).toBe(true);
        expect(host.textContent).toContain('Resume controls in Transfers');
    });
});
