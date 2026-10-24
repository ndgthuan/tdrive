import { afterEach, describe, expect, it } from 'vitest';
import { flushSync, mount, tick, unmount } from 'svelte';
import AccountPane from './account_pane.svelte';
import { profileLoaded, profileUser } from '../chrome/profile_store';

let component: Record<string, unknown> | null = null;
let host: HTMLElement | null = null;

async function settle(): Promise<void> {
    for (let i = 0; i < 6; i += 1) {
        await Promise.resolve();
        await tick();
    }
    flushSync();
}

afterEach(async () => {
    if (component) await unmount(component);
    host?.remove();
    component = null;
    host = null;
    profileUser.set(null);
    profileLoaded.set(false);
});

describe('account pane', () => {
    it('shows name and handle without a log out button', async () => {
        profileUser.set({ displayName: 'Ada L.', username: 'ada', userId: 7 });
        profileLoaded.set(true);
        host = document.createElement('div');
        document.body.appendChild(host);
        component = mount(AccountPane, { target: host });
        await settle();

        expect(host.textContent).toContain('Ada L.');
        expect(host.textContent).toContain('@ada');
        expect(host.querySelector('button')).toBeNull();
        expect(host.textContent).not.toContain('Log out');
    });
});
