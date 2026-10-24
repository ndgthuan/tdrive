// Time labels for the session list and messages. Timestamps from the backend
// are Unix milliseconds.
import { get } from 'svelte/store';
import { current_locale, format_date } from '../../i18n/i18n';

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

export function format_relative_time(timestamp_ms: number, now: number = Date.now()): string {
    const elapsed = now - timestamp_ms;
    const formatter = new Intl.RelativeTimeFormat(get(current_locale), { numeric: 'auto' });
    if (elapsed < HOUR) return formatter.format(-Math.max(0, Math.round(elapsed / MINUTE)), 'minute');
    if (elapsed < DAY) return formatter.format(-Math.round(elapsed / HOUR), 'hour');
    if (elapsed < 7 * DAY) return formatter.format(-Math.round(elapsed / DAY), 'day');
    return format_date(timestamp_ms, 'medium');
}
