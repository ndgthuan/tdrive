// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { afterEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { settings_view } from './settings_store';

// The dialog itself is covered by its pane tests; this only checks that the
// window event reaches the store through the host's listener.
vi.mock('./settings_dialog.svelte', () => ({ default: () => undefined }));

import { OPEN_SETTINGS_EVENT } from './settings_host';

afterEach(() => {
    settings_view.set({ open: false, pane: 'general' });
});

describe('settings host window event', () => {
    it('opens the requested pane', () => {
        window.dispatchEvent(new CustomEvent(OPEN_SETTINGS_EVENT, { detail: { section: 'ai' } }));
        expect(get(settings_view)).toEqual({ open: true, pane: 'ai' });
    });

    it('opens General when no section is given', () => {
        settings_view.set({ open: false, pane: 'language' });
        window.dispatchEvent(new CustomEvent(OPEN_SETTINGS_EVENT));
        expect(get(settings_view)).toEqual({ open: true, pane: 'general' });
    });

    it('uses the event name the AI composer dispatches', () => {
        expect(OPEN_SETTINGS_EVENT).toBe('tdrive:open_settings');
    });
});
