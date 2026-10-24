import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import { set_locale } from '../../i18n/i18n';
import { sidebarState } from '../sidebar/sidebar_store';
import { breadcrumbPath } from './breadcrumb_store';
import Breadcrumb from './breadcrumb.svelte';

const drag = {
    isActive: () => false,
    canDrop: () => false,
    highlight: vi.fn(),
    leave: vi.fn(),
    dropOn: vi.fn(),
    registerRoot: vi.fn(),
};

let host: HTMLElement | null = null;
let component: ReturnType<typeof mount> | null = null;

function render(): void {
    host = document.createElement('div');
    document.body.appendChild(host);
    component = mount(Breadcrumb, {
        target: host,
        props: { onNavigate: vi.fn(), onBack: vi.fn(), drag },
    });
    flushSync();
}

function root_label(): string {
    return host?.querySelector('[data-index="-1"]')?.textContent?.trim() ?? '';
}

beforeEach(async () => {
    await set_locale('en');
    breadcrumbPath.set([]);
    sidebarState.set({
        personal: [{ id: 1, title: 'TDrive', kind: 'personal', isActive: true, inviteLink: '' }],
        shared: [{ id: 77, title: 'temp', kind: 'shared', isActive: false, inviteLink: '' }],
        pending: [],
        activeChannelId: 1,
        virtualView: null,
    });
});

afterEach(async () => {
    if (component) await unmount(component);
    host?.remove();
    component = null;
    host = null;
});

describe('breadcrumb root', () => {
    it('names the personal drive My Drive', () => {
        render();
        expect(root_label()).toBe('My Drive');
    });

    it('names the root after the shared drive that is open', () => {
        render();
        sidebarState.update((current) => ({ ...current, activeChannelId: 77 }));
        flushSync();
        expect(root_label()).toBe('temp');
        expect(host?.textContent).not.toContain('My Drive');
    });

    it('keeps the shared drive name above an open folder', () => {
        sidebarState.update((current) => ({ ...current, activeChannelId: 77 }));
        breadcrumbPath.set([{ id: 'f1', name: 'Reports' }]);
        render();
        expect(root_label()).toBe('temp');
        expect(host?.textContent).toContain('Reports');
    });
});
