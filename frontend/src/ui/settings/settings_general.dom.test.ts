// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import { set_locale } from '../../i18n/i18n';
import { settings_view } from './settings_store';
import SettingsDialog from './settings_dialog.svelte';
import GeneralPane from './general_pane.svelte';
import { appVersionInfo, updateState } from '../updates/update_store';
import { checkForUpdates } from '../../modules/updates';
import { initialUpdateState } from '../updates/update_model';

vi.mock('../../modules/updates', () => ({
    cancelUpdateDownload: vi.fn(),
    checkForUpdates: vi.fn(async () => {}),
    downloadUpdate: vi.fn(async () => {}),
    getRestartRisks: vi.fn(async () => []),
    installUpdate: vi.fn(async () => {}),
}));

let host: HTMLElement | null = null;
let component: ReturnType<typeof mount> | null = null;

function mountInto(target: unknown): void {
    host = document.createElement('div');
    document.body.appendChild(host);
    component = mount(target as never, { target: host });
    flushSync();
}

beforeEach(async () => {
    await set_locale('en');
    appVersionInfo.set({ version: '1.7.2', os: 'windows', arch: 'amd64', devBuild: false, storeManaged: false } as never);
    updateState.set(initialUpdateState());
    settings_view.set({ open: true, pane: 'general' });
});

afterEach(async () => {
    if (component) await unmount(component);
    host?.remove();
    component = null;
    host = null;
    settings_view.set({ open: false, pane: 'general' });
    vi.clearAllMocks();
});

describe('Settings navigation', () => {
    it('no longer has an Updates pane', () => {
        mountInto(SettingsDialog);
        const labels = Array.from(host?.querySelectorAll('.settings-nav-item') ?? []).map((item) => item.textContent?.trim());
        expect(labels).toEqual(['General', 'Appearance', 'Language', 'AI Models', 'Account']);
        expect(labels).not.toContain('Updates');
    });
});

describe('General pane updates', () => {
    it('shows the current version and a Check for updates button', () => {
        mountInto(GeneralPane);
        expect(host?.textContent).toContain('1.7.2');
        const button = Array.from(host?.querySelectorAll<HTMLButtonElement>('button') ?? [])
            .find((item) => item.textContent?.trim() === 'Check for updates');
        expect(button).toBeDefined();
    });

    it('runs an explicit update check from the button', () => {
        mountInto(GeneralPane);
        const button = Array.from(host?.querySelectorAll<HTMLButtonElement>('button') ?? [])
            .find((item) => item.textContent?.trim() === 'Check for updates');
        button?.click();
        flushSync();
        expect(checkForUpdates).toHaveBeenCalledWith({ explicit: true });
    });

    it('offers the restart when an update is ready', () => {
        updateState.set({ ...initialUpdateState(), phase: 'ready', latest: { version: '1.7.3' } as never });
        mountInto(GeneralPane);
        expect(host?.textContent).toContain('Restart to update');
    });

    it('hides the check on builds the store manages', () => {
        appVersionInfo.set({ version: '1.7.2', os: 'windows', arch: 'amd64', devBuild: false, storeManaged: true } as never);
        mountInto(GeneralPane);
        expect(host?.querySelector('.update-footer')).toBeNull();
    });
});
