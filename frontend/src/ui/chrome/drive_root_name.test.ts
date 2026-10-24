import { beforeEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { set_locale, t } from '../../i18n/i18n';
import { sidebarState, type SidebarState } from '../sidebar/sidebar_store';
import { active_drive_root_name, drive_root_name, drive_root_name_for_id } from './drive_root_name';
import type { DriveChannel } from '../../types';

const personal: DriveChannel = { id: 1, title: 'TDrive', kind: 'personal', isActive: false, inviteLink: '' };
const temp: DriveChannel = { id: 2, title: 'temp', kind: 'shared', isActive: false, inviteLink: '' };

function sidebar(active: number | null): SidebarState {
    return { personal: [personal], shared: [temp], pending: [], activeChannelId: active, virtualView: null };
}

beforeEach(async () => {
    await set_locale('en');
    sidebarState.set(sidebar(1));
});

describe('drive_root_name', () => {
    it('calls the personal drive My Drive, whatever its stored title', () => {
        expect(drive_root_name(personal, t)).toBe('My Drive');
        expect(drive_root_name(null, t)).toBe('My Drive');
    });

    it('names a shared drive by its own title', () => {
        expect(drive_root_name(temp, t)).toBe('temp');
    });

    it('falls back to Untitled for a shared drive without a title', () => {
        expect(drive_root_name({ kind: 'shared', title: '  ' }, t)).toBe('Untitled');
    });

    it('is localized for the personal drive', async () => {
        await set_locale('vi');
        try {
            expect(drive_root_name(personal, t)).toBe(t('chrome.my_drive'));
            expect(drive_root_name(personal, t)).not.toBe('My Drive');
        } finally {
            await set_locale('en');
        }
    });
});

describe('active drive root name', () => {
    it('follows the open drive when it switches to a shared drive', () => {
        expect(get(active_drive_root_name)).toBe('My Drive');
        sidebarState.set(sidebar(2));
        expect(get(active_drive_root_name)).toBe('temp');
        sidebarState.set(sidebar(1));
        expect(get(active_drive_root_name)).toBe('My Drive');
    });
});

describe('drive_root_name_for_id', () => {
    it('names a drive by id from the sidebar list', () => {
        expect(drive_root_name_for_id(2)).toBe('temp');
        expect(drive_root_name_for_id(1)).toBe('My Drive');
    });
});
