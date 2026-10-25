import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import { bootTDrive, expect, resolves, test } from './wails_mock';

const automatic = { enabled: true, permission: 'granted', phase: 'running', platform: 'ios', folder_id: 'phone', folder_name: 'iPhone', wifi_only: true, errors: [] };
const queue = { enabled: true, paused: false, done: 632, total: 823, current_name: 'IMG_4461.PNG', current_bytes: 6000000, current_bytes_done: 3000000 };
const core = { platform: 'ios', encryption_required: false, manual_paused: false,
    settings: { enabled: true, photos: true, videos: true, encrypt: true },
    destination: { id: '1', title: 'My Drive', kind: 'personal' },
    sources: [{ id: 'device-library', kind: 'device-library', name: 'iPhone', enabled: true, scan_complete: false }],
    status: { phase: 'uploading', complete: 632, pending: 189, uploading: 2, current_file: 'IMG_4461.PNG', current_file_bytes_total: 6000000, current_file_bytes_done: 3000000 },
};

// Save the exact mock initialization used by the browser check so a local
// preview can load the production frontend with the same synthetic data.
function capturePreview(page: Page, scene: string): () => void {
    const scripts: string[] = [];
    const original = page.addInitScript.bind(page);
    page.addInitScript = async (script: unknown, arg?: unknown) => {
        if (typeof script === 'function') scripts.push(`(${script.toString()})(${JSON.stringify(arg) ?? 'undefined'});`);
        else if (typeof script === 'string') scripts.push(script);
        return original(script as (arg: unknown) => void, arg);
    };
    return () => {
        const directory = resolve('../.task/design-preview');
        mkdirSync(directory, { recursive: true });
        writeFileSync(resolve(directory, `${scene}.js`), scripts.join('\n') + `\n(${previewNavigation.toString()})(${[scene, core, queue].map((value) => JSON.stringify(value)).join(',')});`);
    };
}

function previewNavigation(scene: string, snapshot: typeof core, report: typeof queue): void {
    document.addEventListener('click', (event) => {
        const button = (event.target as HTMLElement)?.closest('button');
        if (!button?.closest('.ab-status-actions')) return;
        const action = button.textContent?.trim();
        if (action !== 'Pause' && action !== 'Resume') return;
        const paused = action === 'Pause';
        window.__wailsMock.setPlan('GetPhotoBackupState', { kind: 'resolve', delayMs: 0, value: { ...snapshot, manual_paused: paused, status: { ...snapshot.status, phase: paused ? 'paused' : 'uploading' } } });
        window.__wailsMock.setPlan('GetProgress', { kind: 'resolve', delayMs: 0, value: { ...report, paused } });
        setTimeout(() => window.__wailsMock.emit('auto_backup:progress', { ...report, paused }), 0);
    }, true);
    let step = 0;
    const timer = setInterval(() => {
        const buttons = [...document.querySelectorAll<HTMLButtonElement>('button')];
        if (scene === 'audio') {
            const file = document.querySelector<HTMLElement>('.file-row');
            if (file) { file.click(); clearInterval(timer); }
            return;
        }
        if (step === 0) {
            const account = document.querySelector<HTMLButtonElement>('.tab-bar button:last-child') || buttons.find((button) => button.textContent?.trim() === 'Account');
            if (account) { account.click(); step = 1; }
        } else if (step === 1) {
            const backup = buttons.find((button) => button.classList.contains('account-row') && button.textContent?.includes('Photo & video'));
            if (backup) { backup.click(); step = 2; }
        } else if (scene === 'encryption') {
            const unlock = buttons.find((button) => button.textContent?.includes('Unlock and back up'));
            if (unlock) { unlock.click(); clearInterval(timer); }
        } else clearInterval(timer);
    }, 100);
}

async function openBackup(page: Page): Promise<void> {
    await page.getByRole('navigation', { name: 'Primary' }).getByRole('button', { name: 'Account', exact: true }).click();
    await page.getByRole('button', { name: /Photo & video backup/ }).click();
}

test('iPhone backup restores progress without toggling and pauses from its status card', async ({ page }) => {
    const savePreview = capturePreview(page, 'backup');
    await page.setViewportSize({ width: 390, height: 844 });
    const mock = await bootTDrive(page, { GetState: resolves(automatic), GetProgress: resolves(queue), GetPhotoBackupState: resolves(core), PausePhotoBackup: resolves({ ok: true }), ResumePhotoBackup: resolves({ ok: true }) }, { url: '/?mobile=ios' });
    await openBackup(page);
    const panel = page.locator('.auto-backup');
    await expect(panel).toContainText('632 of 823');
    await expect(panel).toContainText('IMG_4461.PNG');
    await expect(panel.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
    await expect(panel.getByRole('button', { name: 'Files', exact: true })).toBeVisible();
    await page.screenshot({ path: '../.task/backup-redesign.png' });
    await mock.setPlan('GetPhotoBackupState', resolves({ ...core, manual_paused: true, status: { ...core.status, phase: 'paused' } }));
    await panel.getByRole('button', { name: 'Pause', exact: true }).click();
    await expect(panel.getByRole('button', { name: 'Resume', exact: true })).toBeVisible();
    expect(await mock.calls('PausePhotoBackup')).toHaveLength(1);
    await mock.setPlan('GetPhotoBackupState', resolves(core));
    await panel.getByRole('button', { name: 'Resume', exact: true }).click();
    await expect(panel.getByRole('button', { name: 'Pause', exact: true })).toBeVisible();
    expect(await mock.calls('ResumePhotoBackup')).toHaveLength(1);
    await panel.getByRole('button', { name: 'Files', exact: true }).click();
    await expect(panel).toBeHidden();
    await expect(page.getByRole('heading', { name: 'iPhone', exact: true })).toBeVisible();
    savePreview();
});

test('iPhone encryption setup has spaced labels and usable fields', async ({ page }) => {
    const savePreview = capturePreview(page, 'encryption');
    await page.setViewportSize({ width: 390, height: 844 });
    await bootTDrive(page, { GetState: resolves(automatic), GetPhotoBackupState: resolves({ ...core, encryption_required: true, status: { phase: 'idle' } }) }, { url: '/?mobile=ios' });
    await openBackup(page);
    await page.getByRole('button', { name: 'Unlock and back up', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Set an encryption password' });
    await expect(dialog).toBeVisible();
    const inputs = dialog.locator('.enc-fields input');
    await expect(inputs).toHaveCount(3);
    const boxes = await inputs.evaluateAll((elements) => elements.map((element) => { const box = element.getBoundingClientRect(); return { top: box.top, bottom: box.bottom, height: box.height }; }));
    expect(boxes.every((box) => box.height >= 44)).toBe(true);
    expect(boxes[1].top - boxes[0].bottom).toBeGreaterThan(12);
    await expect(dialog.getByRole('button', { name: 'Set password', exact: true })).toBeInViewport();
    await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeInViewport();
    await page.screenshot({ path: '../.task/encryption-redesign.png' });
    await page.setViewportSize({ width: 844, height: 390 });
    await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeInViewport();
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(dialog).toBeHidden();
    savePreview();
});

function sampleWav(): Buffer {
    const samples = 8000 * 32;
    const data = Buffer.alloc(44 + samples * 2);
    data.write('RIFF'); data.writeUInt32LE(data.length - 8, 4); data.write('WAVEfmt ', 8); data.writeUInt32LE(16, 16); data.writeUInt16LE(1, 20); data.writeUInt16LE(1, 22); data.writeUInt32LE(8000, 24); data.writeUInt32LE(16000, 28); data.writeUInt16LE(2, 32); data.writeUInt16LE(16, 34); data.write('data', 36); data.writeUInt32LE(samples * 2, 40);
    return data;
}

test('iPhone audio opens from Files and plays, seeks, changes speed and pauses', async ({ page, browserName }) => {
    test.skip(browserName === 'webkit' && process.platform === 'win32', 'This Windows WebKit build does not start WAV decoding; the same playback check runs on macOS WebKit in CI.');
    const savePreview = capturePreview(page, 'audio');
    await page.setViewportSize({ width: 390, height: 844 });
    const wav = sampleWav();
    mkdirSync(resolve('../.task/design-preview'), { recursive: true });
    writeFileSync(resolve('../.task/design-preview/sample.wav'), wav);
    await page.route('**/__fixtures__/sample.wav', async (route) => {
        const range = route.request().headers().range?.match(/bytes=(\d+)-(\d*)/);
        const start = range ? Number(range[1]) : 0;
        const end = range?.[2] ? Math.min(wav.length - 1, Number(range[2])) : wav.length - 1;
        await route.fulfill({ status: range ? 206 : 200, contentType: 'audio/wav', body: wav.subarray(start, end + 1), headers: { 'Accept-Ranges': 'bytes', ...(range ? { 'Content-Range': `bytes ${start}-${end}/${wav.length}` } : {}) } });
    });
    await bootTDrive(page, {
        GetFileList: resolves([{ id: 701, name: 'Evening session.wav', size: 512044, date: 1735689600 }]),
        OpenStream: resolves({ token: 'audio-test', url: 'http://127.0.0.1:4173/__fixtures__/sample.wav', name: 'Evening session.wav', kind: 'audio', mime_type: 'audio/wav', supports_range: true, info: { channel_id: 1, msg_id: 701, name: 'Evening session.wav', size: 512044 } }),
        CloseMedia: resolves(null),
    }, { url: '/?mobile=ios' });
    await page.locator('.file-row').filter({ hasText: 'Evening session.wav' }).click();
    const player = page.locator('.audio-viewer');
    await expect(player).toBeVisible();
    const audio = player.locator('audio');
    await player.getByRole('button', { name: 'Play audio', exact: true }).click();
    await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.duration)).toBe(32);
    await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(false);
    await player.getByRole('button', { name: 'Forward 10 seconds' }).click();
    await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.currentTime)).toBeGreaterThanOrEqual(10);
    await player.getByRole('button', { name: 'Playback speed' }).click();
    await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.playbackRate)).toBe(1.25);
    await player.getByRole('button', { name: 'Pause audio', exact: true }).click();
    await expect.poll(() => audio.evaluate((element: HTMLAudioElement) => element.paused)).toBe(true);
    await page.screenshot({ path: '../.task/audio-redesign.png' });
    savePreview();
});
