// Pure helpers for the link downloader. They do no I/O and read no locale, so
// they can be tested directly. Words the user sees come from the i18n keys the
// dialog picks; this module only decides which key and which number.

export const STAGES = ['downloading', 'merging', 'uploading', 'done', 'error', 'cancelled'] as const;
export type download_stage = (typeof STAGES)[number];

const SIZE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const;

/** Decimal byte size, "512 B", "4.2 MB", "1.3 GB". Empty for unknown or zero sizes. */
export function human_size(bytes: number): string {
    if (!Number.isFinite(bytes) || bytes <= 0) return '';
    let value = bytes;
    let unit = 0;
    while (value >= 1000 && unit < SIZE_UNITS.length - 1) {
        value /= 1000;
        unit += 1;
    }
    const digits = unit === 0 || value >= 10 ? 0 : 1;
    return `${value.toFixed(digits)} ${SIZE_UNITS[unit]}`;
}

/** Transfer speed per second, "2.3 MB/s". Empty when the speed is unknown. */
export function format_speed(bytes_per_second: number): string {
    const size = human_size(bytes_per_second);
    return size ? `${size}/s` : '';
}

/** Remaining time as a clock: "0:42", "4:05", "1:02:09". Empty when unknown (-1). */
export function format_eta(seconds: number): string {
    if (!Number.isFinite(seconds) || seconds < 0) return '';
    return clock(seconds);
}

/** Video length, "4:05" or "1:02:09". Zero or unknown lengths read "0:00". */
export function format_clock(seconds: number): string {
    return clock(Number.isFinite(seconds) && seconds > 0 ? seconds : 0);
}

function clock(seconds: number): string {
    const total = Math.floor(seconds);
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const secs = total % 60;
    const pad = (value: number) => String(value).padStart(2, '0');
    return hours > 0 ? `${hours}:${pad(minutes)}:${pad(secs)}` : `${minutes}:${pad(secs)}`;
}

/** Whole percent 0-100 for a bar. Unknown (-1) or invalid values read 0. */
export function clamp_percent(percent: number): number {
    if (!Number.isFinite(percent) || percent < 0) return 0;
    return Math.min(100, Math.round(percent));
}

/** True for a link the backend will accept: absolute http or https, no credentials. */
export function is_valid_link(raw: string): boolean {
    const value = raw.trim();
    if (!value || value.length > 2048) return false;
    let parsed: URL;
    try {
        parsed = new URL(value);
    } catch {
        return false;
    }
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return false;
    if (!parsed.hostname) return false;
    return parsed.username === '' && parsed.password === '';
}

/** The format fields the dialog reads. Mirrors the Go ProbeFormat JSON. */
export interface format_facts {
    height: number;
    ext: string;
    approx_size: number;
    has_audio: boolean;
    has_video: boolean;
    note: string;
}

export type format_kind = 'combined' | 'video_only' | 'audio_only';

export interface format_description {
    kind: format_kind;
    resolution: string;
    ext: string;
    size: string;
}

/**
 * Splits one format into the parts the option shows. Resolution is "1080p" for
 * video, the site's note for audio, and the extension is upper-cased.
 */
export function describe_format(format: format_facts): format_description {
    const kind: format_kind = format.has_video ? (format.has_audio ? 'combined' : 'video_only') : 'audio_only';
    const resolution = format.height > 0 ? `${format.height}p` : format.note.trim();
    return {
        kind,
        resolution,
        ext: format.ext.toUpperCase(),
        size: human_size(format.approx_size),
    };
}

/** Joins the non-empty option parts with a middle dot. */
export function join_parts(parts: Array<string | undefined>): string {
    return parts.filter((part): part is string => Boolean(part && part.trim())).join(' · ');
}

/** Stage that still has work in flight, so Cancel applies. */
export function is_active_stage(stage: string): boolean {
    return stage === 'downloading' || stage === 'merging' || stage === 'uploading';
}

/** Stage with no more updates. */
export function is_terminal_stage(stage: string): boolean {
    return stage === 'done' || stage === 'error' || stage === 'cancelled';
}

/** Translation key for a stage label. Unknown stages fall back to the generic label. */
export function stage_key(stage: string): string {
    return STAGES.includes(stage as download_stage) ? `downloader.stage_${stage}` : 'downloader.stage_error';
}

/** Setup stages the install progress reports, mapped to their translation key. */
export function setup_stage_key(stage: string): string {
    switch (stage) {
        case 'downloading':
            return 'downloader.setup_downloading';
        case 'verifying':
            return 'downloader.setup_verifying';
        case 'extracting':
            return 'downloader.setup_extracting';
        case 'done':
            return 'downloader.setup_done';
        default:
            return 'downloader.setup_error';
    }
}

/** Pulls the host of a link for a compact label, "" when it does not parse. */
export function link_host(raw: string): string {
    try {
        return new URL(raw.trim()).hostname.replace(/^www\./, '');
    } catch {
        return '';
    }
}
