// Mounts the settings dialog once, into a body-level overlay of its own. A
// body-level host keeps the dialog out of the account menu's popover, which is
// positioned inside the header and must not clip or trap a full-window dialog.
import { get } from 'svelte/store';
import { mount } from 'svelte';
import SettingsDialog from './settings_dialog.svelte';
import { settings_view, type settings_pane } from './settings_store';
import { updatesPanelRequest } from '../updates/update_store';

const HOST_ID = 'settings-modal';

let mounted = false;

function ensure_host(): void {
    if (mounted || typeof document === 'undefined') return;
    let host = document.getElementById(HOST_ID);
    if (!host) {
        host = document.createElement('div');
        host.id = HOST_ID;
        host.className = 'modal-overlay';
        host.style.display = 'none';
        host.setAttribute('aria-hidden', 'true');
        document.body.appendChild(host);
    }
    mount(SettingsDialog, { target: host });
    mounted = true;
}

/** Opens the settings dialog on a pane. Safe to call repeatedly. */
export function open_settings(pane: settings_pane = 'general'): void {
    ensure_host();
    settings_view.set({ open: true, pane });
}

// The macOS "Check for Updates", the login-screen link and the explicit
// updater entry point all bump this nonce; each one lands on the Updates pane.
let seen_request = get(updatesPanelRequest);
updatesPanelRequest.subscribe((nonce) => {
    if (nonce === seen_request) return;
    seen_request = nonce;
    open_settings('updates');
});

// Anything outside the settings tree (the AI composer's "add a model" call to
// action, for one) opens the dialog by dispatching this event instead of
// importing the host. A missing section means the General pane.
export const OPEN_SETTINGS_EVENT = 'tdrive:open_settings';

function on_open_settings_event(event: Event): void {
    const detail = (event as CustomEvent<{ section?: settings_pane } | undefined>).detail;
    open_settings(detail?.section ?? 'general');
}

if (typeof window !== 'undefined') {
    window.addEventListener(OPEN_SETTINGS_EVENT, on_open_settings_event);
}
