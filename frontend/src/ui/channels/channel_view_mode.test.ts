import { afterEach, describe, expect, it, vi } from 'vitest';
import { load_source_view_mode, save_source_view_mode, SOURCE_VIEW_STORAGE_KEY } from './channel_view_mode';

function memory_storage(initial: Record<string, string> = {}) {
    const values = new Map(Object.entries(initial));
    return {
        values,
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => { values.set(key, value); },
        removeItem: (key: string) => { values.delete(key); },
        clear: () => values.clear(),
        key: () => null,
        get length() { return values.size; },
    };
}

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('source view mode', () => {
    it('shows the grid when nothing is saved', () => {
        vi.stubGlobal('localStorage', memory_storage());
        expect(load_source_view_mode()).toBe('grid');
    });

    it('restores the list the user last chose', () => {
        vi.stubGlobal('localStorage', memory_storage({ [SOURCE_VIEW_STORAGE_KEY]: 'list' }));
        expect(load_source_view_mode()).toBe('list');
    });

    it('ignores a value it does not know', () => {
        vi.stubGlobal('localStorage', memory_storage({ [SOURCE_VIEW_STORAGE_KEY]: 'table' }));
        expect(load_source_view_mode()).toBe('grid');
    });

    it('saves the choice under tdrive_source_view', () => {
        const storage = memory_storage();
        vi.stubGlobal('localStorage', storage);
        save_source_view_mode('list');
        expect(storage.values.get('tdrive_source_view')).toBe('list');
        expect(load_source_view_mode()).toBe('list');
    });

    it('falls back to the grid and does not throw when storage is blocked', () => {
        const blocked = {
            getItem: () => { throw new Error('storage disabled'); },
            setItem: () => { throw new Error('quota exceeded'); },
        };
        vi.stubGlobal('localStorage', blocked);
        expect(load_source_view_mode()).toBe('grid');
        expect(() => save_source_view_mode('list')).not.toThrow();
    });
});
