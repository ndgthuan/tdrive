// Display helpers for the phone row: the two-line layout has room for one
// meta line, so size and age share it, and a long name keeps its extension by
// giving up its middle instead of its end.

import { get } from 'svelte/store';
import { current_locale, t } from '../../i18n/i18n';
import { formatBytes, formatDate } from '../../utils';
import type { FileListFileRow, FolderListRow } from '../file_list/types';

const TAIL_CHARS = 4;
// The tail is the half that never shrinks -- keeping it is the whole point of
// middle truncation -- so what counts as an extension has to be bounded here.
// "Report 2026.09-final-annotated" has a 19-character "extension", and a tail
// that long walks straight over the status badge and the overflow button on a
// narrow screen. Six is the same limit utils.splitNameAndExt puts on the type
// label, so both halves of the row agree on what an extension is.
const MAX_EXT_CHARS = 6;

const relative_formatters = new Map<string, Intl.RelativeTimeFormat>();

/** "3 days ago" in the current language, from Intl rather than hand-written plurals. */
function relative_phrase(value: number, unit: Intl.RelativeTimeFormatUnit, numeric: 'always' | 'auto' = 'always'): string {
    const locale = get(current_locale);
    const cache_key = `${locale}|${numeric}`;
    let formatter = relative_formatters.get(cache_key);
    if (!formatter) {
        formatter = new Intl.RelativeTimeFormat(locale, { numeric });
        relative_formatters.set(cache_key, formatter);
    }
    return formatter.format(value, unit);
}

function capitalize(text: string): string {
    return text.charAt(0).toUpperCase() + text.slice(1);
}

/** "Just now", "3 minutes ago", "Yesterday", then the calendar date past a week. */
export function relativeTimeLabel(unixSec: number, nowMs = Date.now()): string {
    if (!unixSec) return '';
    const diff = Math.max(0, Math.floor(nowMs / 1000) - unixSec);
    if (diff < 60) return t('files.just_now');
    const minutes = Math.floor(diff / 60);
    if (minutes < 60) return relative_phrase(-minutes, 'minute');
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return relative_phrase(-hours, 'hour');
    const days = Math.floor(hours / 24);
    if (days === 1) return capitalize(relative_phrase(-1, 'day', 'auto'));
    if (days < 7) return relative_phrase(-days, 'day');
    return formatDate(unixSec);
}

/**
 * The kind of thing this row is, in the words a reader would use: "PDF",
 * "Folder". It leads the meta line because it is the one field that is always
 * known -- a folder's size and a fresh file's timestamp can both be missing,
 * and a meta line that sometimes starts with a size and sometimes with a date
 * gives the eye no fixed column to scan down.
 */
export function rowTypeLabel(row: FolderListRow | FileListFileRow): string {
    if (row.kind === 'folder') return t('files.folder');
    return row.ext ? row.ext.toUpperCase() : t('files.file');
}

/** "PDF · 2 MB · 2 hours ago", dropping whichever parts are not known yet. */
export function rowMetaLine(row: FolderListRow | FileListFileRow, nowMs = Date.now()): string {
    const type = rowTypeLabel(row);
    if (row.kind === 'folder') {
        const parts = [
            type,
            row.size > 0 ? formatBytes(row.size) : '',
            row.timeLabel ?? relativeTimeLabel(row.modifiedTime, nowMs),
        ];
        return parts.filter(Boolean).join(' · ');
    }
    return [type, formatBytes(row.size), row.timeLabel ?? relativeTimeLabel(row.uploadTime, nowMs)]
        .filter(Boolean)
        .join(' · ');
}

/**
 * Splits a name so the tail (the last few characters of the base plus the
 * extension) can stay while the head ellipsises. Names without an extension --
 * or with a trailing run too long to be one -- keep the whole string in the
 * head and give way from the end like any other label.
 */
export function splitRowLabel(name: string): { head: string; tail: string } {
    const dot = name.lastIndexOf('.');
    if (dot <= 0 || dot === name.length - 1) return { head: name, tail: '' };
    if (name.length - dot - 1 > MAX_EXT_CHARS) return { head: name, tail: '' };
    const cut = Math.max(0, dot - TAIL_CHARS);
    return { head: name.slice(0, cut), tail: name.slice(cut) };
}
