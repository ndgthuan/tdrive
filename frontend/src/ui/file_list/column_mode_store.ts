/**
 * What the file list's second column is currently reporting.
 *
 * Normally it is the date a file was added, and the header carries the Date
 * sort control. A search spans every folder in the drive, so the date stops
 * being the useful fact and the same column reports where each result lives
 * instead -- search puts the path in `metaLabel`, which is the field that
 * column draws.
 *
 * This exists because the header used to be relabelled by writing to the
 * column's textContent from the search module. That column is not a text node:
 * it wraps the Date sort button and its indicator, so assigning textContent
 * deleted the control outright and left Svelte updating nodes that were no
 * longer in the document. The button never came back, because what got restored
 * afterwards was the text captured from the whole subtree.
 *
 * A one-word label is not worth reaching across a module boundary for. The
 * header reads this, the search module sets it, and neither needs to know how
 * the other is built.
 */

import { derived, writable } from 'svelte/store';
import { t, t_store } from '../../i18n/i18n';

export type FileListColumnMode = 'date' | 'location' | 'purge';

const COLUMN_LABEL_KEY: Record<FileListColumnMode, string> = {
    date: 'files.column_date',
    location: 'files.column_location',
    // The trash reports how long each item has left, which is the fact it
    // exists to tell you -- not the date it was added to the drive.
    purge: 'files.column_time_left',
};

/**
 * What each mode calls the column, read at access time so it follows the
 * language. Non-reactive: .svelte code should use `file_list_column_label`.
 */
export const FILE_LIST_COLUMN_LABEL: Record<FileListColumnMode, string> = {
    get date() { return t(COLUMN_LABEL_KEY.date); },
    get location() { return t(COLUMN_LABEL_KEY.location); },
    get purge() { return t(COLUMN_LABEL_KEY.purge); },
};

export const fileListColumnMode = writable<FileListColumnMode>('date');

/** The column label for the current mode, updating with the language. */
export const file_list_column_label = derived(
    [t_store, fileListColumnMode],
    ([translate, mode]) => translate(COLUMN_LABEL_KEY[mode]),
);

/** Leaving search or the trash returns the column to the drive's own ordering. */
export function resetFileListColumnMode(): void {
    fileListColumnMode.set('date');
}
