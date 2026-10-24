import { writable } from 'svelte/store';
import { t } from '../../i18n/i18n';

/** The name a new folder row starts with, selected so typing replaces it. */
export function default_new_folder_name(): string {
    return t('files.new_folder');
}

/**
 * The name typed into the inline "new folder" row at the top of the desktop
 * list, or null while no such row is open. Only the desktop list reads it; the
 * phone still asks in its modal.
 */
export const new_folder_draft = writable<string | null>(null);

/**
 * Opens the inline new folder row at the top of the desktop list. It scrolls the
 * list to the top first, because the row is the first thing in it.
 */
export function open_inline_folder_row(): void {
    const list = document.getElementById('file-list') as HTMLElement | null;
    if (list) list.scrollTop = 0;
    new_folder_draft.set(default_new_folder_name());
}
