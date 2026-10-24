import { writable } from 'svelte/store';

export type settings_pane = 'general' | 'appearance' | 'language' | 'ai' | 'updates' | 'account';

export interface settings_view_state {
    open: boolean;
    pane: settings_pane;
}

export const settings_view = writable<settings_view_state>({ open: false, pane: 'general' });

export function close_settings(): void {
    settings_view.update((current) => ({ ...current, open: false }));
}
