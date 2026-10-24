import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, tick, unmount } from 'svelte';
import ProfileMenu from './profile_menu.svelte';
import { encryptionEntryVisible, profileLoaded, profileUser } from './profile_store';
import { settings_view } from '../settings/settings_store';
import { set_locale } from '../../i18n/i18n';

let component: Record<string, unknown> | null = null;
let host: HTMLElement | null = null;

function setup(): void {
    host = document.createElement('div');
    document.body.appendChild(host);
    component = mount(ProfileMenu, {
        target: host,
        props: {
            onOpen: vi.fn(),
            onEncryptionSettings: vi.fn(),
            onLogout: vi.fn(),
        },
    });
    flushSync();
}

function click(selector: string): void {
    const element = host?.querySelector<HTMLElement>(selector);
    if (!element) throw new Error(`missing ${selector}`);
    element.click();
    flushSync();
}

afterEach(async () => {
    if (component) await unmount(component);
    host?.remove();
    component = null;
    host = null;
    profileUser.set(null);
    profileLoaded.set(false);
    encryptionEntryVisible.set(false);
    settings_view.set({ open: false, pane: 'general' });
    // Flush the dialog's close now, so its focus trap cannot swallow the next test's keys.
    flushSync();
    await tick();
    await set_locale('en');
});

describe('ProfileMenu items', () => {
    it('lists Settings, photo backup and log out, and no longer lists updates or appearance', () => {
        profileLoaded.set(true);
        profileUser.set({ displayName: 'Thuận Nguyễn', username: 'thuannguyen7706' } as never);
        setup();
        click('#profile-trigger');

        const items = Array.from(host?.querySelectorAll('[role="menuitem"]') ?? []).map((item) => item.textContent?.trim());
        expect(items).toEqual(['Settings', 'Photo & video backup', 'Log out']);
        expect(host?.querySelector('#profile-menu-updates')).toBeNull();
        expect(host?.querySelector('#profile-menu-appearance')).toBeNull();
        expect(host?.querySelector('#profile-menu-name')?.textContent).toBe('Thuận Nguyễn');
        expect(host?.querySelector('#profile-menu-handle')?.textContent).toBe('@thuannguyen7706');
    });

    it('omits local cache details from the account menu', () => {
        setup();
        click('#profile-trigger');

        expect(host?.querySelector('#profile-menu-storage')).toBeNull();
        expect(host?.querySelector('.photo-cache-panel')).toBeNull();
    });

    it('closes on Escape from the account menu', async () => {
        setup();
        click('#profile-trigger');
        const menu = host?.querySelector<HTMLElement>('#profile-menu');
        expect(menu?.hidden).toBe(false);
        menu?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
        flushSync();
        await tick();
        await Promise.resolve();
        await tick();

        expect(menu?.hidden).toBe(true);
    });

    it('opens the settings dialog on General from the Settings item', () => {
        setup();
        click('#profile-trigger');
        click('#profile-menu-settings');

        let current: { open: boolean; pane: string } = { open: false, pane: '' };
        const stop = settings_view.subscribe((value) => { current = value; });
        stop();
        expect(current).toEqual({ open: true, pane: 'general' });
        expect(document.getElementById('settings-modal')).not.toBeNull();
        expect(host?.querySelector('#profile-menu')?.hasAttribute('hidden')).toBe(true);
    });

    it('shows the encryption entry only when encryption is on', () => {
        encryptionEntryVisible.set(true);
        setup();
        click('#profile-trigger');

        expect(host?.querySelector('#profile-menu-encryption-settings')).not.toBeNull();
    });
});
