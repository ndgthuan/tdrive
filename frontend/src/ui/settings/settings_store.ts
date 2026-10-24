import { writable } from 'svelte/store';

export type settings_pane = 'general' | 'appearance' | 'language' | 'ai' | 'account';

// Window event any component dispatches to open Settings on a pane. A missing
// section means the General pane. settings_host listens for it.
export const OPEN_SETTINGS_EVENT = 'tdrive:open_settings';

export interface settings_view_state {
    open: boolean;
    pane: settings_pane;
}

export const settings_view = writable<settings_view_state>({ open: false, pane: 'general' });

export function close_settings(): void {
    settings_view.update((current) => ({ ...current, open: false }));
}
