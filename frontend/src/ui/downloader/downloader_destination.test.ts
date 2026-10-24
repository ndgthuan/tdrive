// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { SidebarState } from '../sidebar/sidebar_store';
import {
    DESTINATION_STORAGE_KEY,
    destination_at_root,
    destination_place,
    parse_destination,
    read_saved_destination,
    save_destination,
    starting_destination,
} from './downloader_destination';

function sidebar(overrides: Partial<SidebarState> = {}): SidebarState {
    return {
        personal: [{ id: 1, kind: 'personal', title: 'Me' }],
        shared: [{ id: 22, kind: 'shared', title: 'Team' }],
        activeChannelId: 22,
        ...overrides,
    } as unknown as SidebarState;
}

describe('parse_destination', () => {
    it('accepts the drive root and a full folder path', () => {
        expect(parse_destination({ drive_id: 1, folder_id: '', path: [] })).toEqual(destination_at_root(1));
        const nested = { drive_id: 22, folder_id: 'd:b', path: [{ id: 'd:a', name: 'A' }, { id: 'd:b', name: 'B' }] };
        expect(parse_destination(nested)).toEqual(nested);
    });

    it('rejects malformed values instead of guessing', () => {
        expect(parse_destination(null)).toBeNull();
        expect(parse_destination({ drive_id: 0, folder_id: '', path: [] })).toBeNull();
        expect(parse_destination({ drive_id: 1.5, folder_id: '', path: [] })).toBeNull();
        expect(parse_destination({ drive_id: 1, folder_id: 'd:a', path: [] })).toBeNull();
        expect(parse_destination({ drive_id: 1, folder_id: 'd:a', path: [{ id: 'd:b', name: 'B' }] })).toBeNull();
        expect(parse_destination({ drive_id: 1, folder_id: '', path: [{ id: '', name: 'x' }] })).toBeNull();
    });
});

describe('remembered destination', () => {
    afterEach(() => {
        vi.restoreAllMocks();
        window.localStorage.clear();
    });

    it('round-trips through localStorage', () => {
        const chosen = { drive_id: 22, folder_id: 'd:a', path: [{ id: 'd:a', name: 'A' }] };
        save_destination(chosen);
        expect(read_saved_destination()).toEqual(chosen);
    });

    it('falls back to nothing when storage holds junk', () => {
        window.localStorage.setItem(DESTINATION_STORAGE_KEY, '{not json');
        expect(read_saved_destination()).toBeNull();
    });

    it('does not throw when storage is blocked', () => {
        vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
            throw new Error('blocked');
        });
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
            throw new Error('blocked');
        });
        expect(read_saved_destination()).toBeNull();
        expect(() => save_destination(destination_at_root(1))).not.toThrow();
    });
});

describe('starting_destination', () => {
    it('keeps the remembered destination while its drive is still joined', () => {
        const saved = { drive_id: 22, folder_id: 'd:a', path: [{ id: 'd:a', name: 'A' }] };
        expect(starting_destination(saved, sidebar())).toEqual(saved);
    });

    it('starts at My Drive root when the remembered drive was left', () => {
        const saved = { drive_id: 99, folder_id: 'd:a', path: [{ id: 'd:a', name: 'A' }] };
        expect(starting_destination(saved, sidebar())).toEqual(destination_at_root(1));
    });

    it('starts at My Drive root with nothing remembered', () => {
        expect(starting_destination(null, sidebar())).toEqual(destination_at_root(1));
    });

    it('uses the active drive before the drive list loads', () => {
        expect(starting_destination(null, sidebar({ personal: [], shared: [], activeChannelId: 5 }))).toEqual(destination_at_root(5));
    });
});

describe('destination_place', () => {
    it('joins the drive name and folder path', () => {
        const destination = { drive_id: 1, folder_id: 'd:b', path: [{ id: 'd:a', name: 'Videos' }, { id: 'd:b', name: 'Clips' }] };
        expect(destination_place('My Drive', destination)).toBe('My Drive / Videos / Clips');
        expect(destination_place('My Drive', destination_at_root(1))).toBe('My Drive');
    });
});
