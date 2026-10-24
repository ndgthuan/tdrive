import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { createThemeController, THEME_MIGRATION_KEY, THEME_STORAGE_KEY } from './theme_controller';
import { themesForAppearance } from './theme_model';

/** A Map-backed stand-in for localStorage, so each test sees its own history. */
function memory_storage(initial: Record<string, string> = {}) {
    const values = new Map(Object.entries(initial));
    return {
        values,
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => { values.set(key, value); },
    };
}

describe('one-time move to TDrive', () => {
    it('replaces a saved Ayu Dark choice with TDrive the first time and records the move', () => {
        const storage = memory_storage({
            [THEME_STORAGE_KEY]: JSON.stringify({ mode: 'dark', lightThemeId: 'tdrive-light', darkThemeId: 'ayu-dark' }),
        });
        const controller = createThemeController({ storage });

        controller.start();

        expect(get(controller.state).resolvedThemeId).toBe('tdrive');
        expect(storage.values.get(THEME_MIGRATION_KEY)).toBeTruthy();
        expect(JSON.parse(storage.values.get(THEME_STORAGE_KEY) ?? '{}')).toMatchObject({ mode: 'dark', darkThemeId: 'tdrive' });
        controller.destroy();
    });

    it('keeps a theme the person picks after the move', () => {
        const storage = memory_storage({
            [THEME_MIGRATION_KEY]: '1',
            [THEME_STORAGE_KEY]: JSON.stringify({ mode: 'dark', lightThemeId: 'tdrive-light', darkThemeId: 'ayu-dark' }),
        });
        const controller = createThemeController({ storage });

        controller.start();

        expect(get(controller.state).resolvedThemeId).toBe('ayu-dark');
        controller.destroy();
    });

    it('moves a first run with nothing saved and still records the flag', () => {
        const storage = memory_storage();
        const controller = createThemeController({ storage });

        controller.start();

        expect(get(controller.state).resolvedThemeId).toBe('tdrive');
        expect(storage.values.get(THEME_MIGRATION_KEY)).toBeTruthy();
        controller.destroy();
    });

    it('still starts when storage refuses reads', () => {
        const storage = {
            getItem: () => { throw new Error('denied'); },
            setItem: () => { throw new Error('denied'); },
        };
        const controller = createThemeController({ storage });

        expect(() => controller.start()).not.toThrow();
        expect(get(controller.state).resolvedThemeId).toBe('tdrive');
        controller.destroy();
    });
});

describe('picker order', () => {
    it('lists TDrive first among the dark and the light palettes', () => {
        expect(themesForAppearance('dark')[0]?.id).toBe('tdrive');
        expect(themesForAppearance('light')[0]?.id).toBe('tdrive-light');
    });
});
