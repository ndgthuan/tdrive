// Lightweight diagnostics for errors that only happen on a real device.
//
// The phone build hit uncaught errors on taps that the browser preview never
// reproduced. This keeps the last errors in memory and in localStorage so a
// person can copy them from Account > About and send them, and it turns an
// uncaught error into a small toast rather than a screen that stops answering.

import { recentBackendLog } from '../api/session';
import { getAppVersion } from '../api/updates';
import { t } from '../i18n/i18n';
import { notify } from './notifications';

export interface DiagnosticEntry {
    kind: 'error' | 'rejection' | 'handler';
    message: string;
    stack: string;
    time: number;
    route: string;
}

const STORAGE_KEY = 'tdrive.diagnostics.v1';
const MAX_ENTRIES = 100;
const BACKEND_LOG_LINES = 300;
// The report is pasted into a chat or an issue, so the backend section is capped.
const BACKEND_LOG_MAX_CHARS = 64 * 1024;
const TOAST_GAP_MS = 15_000;

let entries: DiagnosticEntry[] = read_stored();
let last_toast_at = 0;

function read_stored(): DiagnosticEntry[] {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        const parsed: unknown = raw ? JSON.parse(raw) : [];
        return Array.isArray(parsed) ? (parsed as DiagnosticEntry[]).slice(-MAX_ENTRIES) : [];
    } catch {
        return [];
    }
}

function persist(): void {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
    } catch {
        // Private windows and blocked storage: memory still holds the list.
    }
}

function route(): string {
    try {
        return `${location.pathname}${location.hash}`;
    } catch {
        return '';
    }
}

function describe(value: unknown): { message: string; stack: string } {
    if (value instanceof Error) return { message: value.message || value.name, stack: value.stack ?? '' };
    if (typeof value === 'string') return { message: value, stack: '' };
    try {
        return { message: JSON.stringify(value) ?? String(value), stack: '' };
    } catch {
        return { message: String(value), stack: '' };
    }
}

/** Records one error. Called by the global listeners and by guarded handlers. */
export function record_diagnostic(kind: DiagnosticEntry['kind'], value: unknown): void {
    const { message, stack } = describe(value);
    entries = [...entries, { kind, message: message.slice(0, 500), stack: stack.slice(0, 2000), time: Date.now(), route: route() }]
        .slice(-MAX_ENTRIES);
    persist();
}

export function diagnostic_entries(): readonly DiagnosticEntry[] {
    return entries;
}

/** Copy action on the toast. Errors arriving in a burst share one toast. */
function announce(): void {
    const now = Date.now();
    if (now - last_toast_at < TOAST_GAP_MS) return;
    last_toast_at = now;
    notify({
        level: 'error',
        title: t('mobile.diag_error_title'),
        action: { label: t('mobile.diag_copy_details'), run: () => { void copy_and_report(); } },
    });
}

/**
 * Runs a UI handler so that a thrown error is recorded and shown as a toast
 * instead of escaping and leaving the control dead. The handler's own result
 * is returned unchanged.
 */
export function guard_handler<A extends unknown[]>(where: string, handler: (...args: A) => void): (...args: A) => void {
    return (...args: A) => {
        try {
            handler(...args);
        } catch (error) {
            record_diagnostic('handler', `${where}: ${describe(error).message}`);
            console.error(`[${where}]`, error);
            announce();
        }
    };
}

let installed = false;

/** Installs the global listeners once. Returns the teardown for tests. */
export function install_diagnostics(): () => void {
    if (installed || typeof window === 'undefined') return () => {};
    installed = true;
    const on_error = (event: ErrorEvent): void => {
        record_diagnostic('error', event.error ?? event.message);
        announce();
    };
    const on_rejection = (event: PromiseRejectionEvent): void => {
        record_diagnostic('rejection', event.reason);
        announce();
    };
    window.addEventListener('error', on_error);
    window.addEventListener('unhandledrejection', on_rejection);
    return () => {
        window.removeEventListener('error', on_error);
        window.removeEventListener('unhandledrejection', on_rejection);
        installed = false;
    };
}

function device_model(): string {
    const ua = typeof navigator === 'undefined' ? '' : navigator.userAgent;
    const ios = /iPhone|iPad|iPod/.test(ua);
    const android = /Android/.test(ua);
    const model = /\(([^)]*)\)/.exec(ua)?.[1] ?? '';
    return ios ? `iOS device (${model})` : android ? `Android device (${model})` : model || 'unknown';
}

/** The text placed on the clipboard: app version, platform, device, then the recorded errors. */
export async function build_diagnostics_report(): Promise<string> {
    let version = 'unknown';
    let platform = 'unknown';
    try {
        const info = await getAppVersion();
        version = `${info.version || 'unknown'} (${info.os}/${info.arch})`;
        platform = info.os || platform;
    } catch {
        // Offline or before the bridge is ready: the report still goes out without it.
    }
    const header = [
        `TDrive diagnostics`,
        `Generated: ${new Date().toISOString()}`,
        `App version: ${version}`,
        `Platform: ${platform}`,
        `Device: ${device_model()}`,
        `User agent: ${typeof navigator === 'undefined' ? '' : navigator.userAgent}`,
        '',
    ];
    const lines = entries.length === 0
        ? ['No errors recorded.']
        : entries.map((entry) => [
            `[${new Date(entry.time).toISOString()}] ${entry.kind} at ${entry.route || '/'}`,
            entry.message,
            entry.stack,
        ].filter(Boolean).join('\n'));
    const backend = await build_backend_log_section();
    return [...header, ...lines, '', ...backend].join('\n');
}

/**
 * The backend log section of the report. The newest lines are kept within the
 * size cap. When the backend cannot be asked, the section says why, and the
 * rest of the report is still produced.
 */
export async function build_backend_log_section(): Promise<string[]> {
    const title = `----- Backend log (last ${BACKEND_LOG_LINES} lines) -----`;
    let body: string;
    try {
        body = await recentBackendLog(BACKEND_LOG_LINES);
    } catch (error) {
        return [title, `(backend log unavailable: ${describe(error).message})`];
    }
    if (body.trim() === '') return [title, '(no backend log lines recorded)'];
    return [title, keep_newest(body, BACKEND_LOG_MAX_CHARS)];
}

/** Keeps the newest characters, starting at a whole line, and says what was dropped. */
function keep_newest(text: string, max: number): string {
    if (text.length <= max) return text;
    const tail = text.slice(text.length - max);
    const first_break = tail.indexOf('\n');
    const whole_lines = first_break >= 0 ? tail.slice(first_break + 1) : tail;
    return `(older lines omitted)\n${whole_lines}`;
}

/** Copies the report and says so, in a toast that does not block the screen. */
export async function copy_and_report(): Promise<void> {
    const ok = await copy_diagnostics();
    notify({ level: ok ? 'success' : 'error', title: t(ok ? 'mobile.diag_copied' : 'mobile.diag_copy_failed'), durationMs: 3000 });
}

/** Copies the report to the clipboard. Returns false when the clipboard is unavailable. */
export async function copy_diagnostics(): Promise<boolean> {
    const report = await build_diagnostics_report();
    try {
        await navigator.clipboard.writeText(report);
        return true;
    } catch {
        return false;
    }
}
