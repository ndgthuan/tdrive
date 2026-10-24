// Pure utility functions for TDrive frontend

import { format_bytes, format_date } from './i18n/i18n';

/** macOS and iOS: the command key is the accelerator and Delete is Backspace. */
export function isApplePlatform(): boolean {
    if (typeof navigator === 'undefined') return false;
    return /Mac|iPhone|iPad|iPod/i.test(navigator.platform || navigator.userAgent || '');
}

export function escapeHtml(input: unknown): string {
    return String(input ?? "")
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#039;");
}

export function splitNameAndExt(filename: string): { base: string; ext: string } {
    const name = typeof filename === "string" ? filename : "";
    const lastDot = name.lastIndexOf(".");
    if (lastDot <= 0 || lastDot === name.length - 1) {
        return { base: name, ext: "FILE" };
    }
    const base = name.slice(0, lastDot);
    const rawExt = name.slice(lastDot + 1);
    const ext = rawExt.replace(/[^a-z0-9]/gi, "").toUpperCase().slice(0, 6) || "FILE";
    return { base, ext };
}

/** A short calendar date in the current language, e.g. "Jan 5, 2026". */
export function formatDate(unixTimestamp: number): string {
    if (!unixTimestamp) return "-";
    return format_date(new Date(unixTimestamp * 1000), 'medium');
}

/** "4:05" or "1:02:09", the way a player's clock reads. */
export function formatDuration(value: number): string {
    if (!Number.isFinite(value) || value < 0) return '0:00';
    const total = Math.floor(value);
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const seconds = total % 60;
    if (hours > 0) {
        return `${hours}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
    }
    return `${minutes}:${String(seconds).padStart(2, '0')}`;
}

/** "1.5 KB", or "1,5 KB" in Vietnamese. Shares the i18n formatter so the app has one size style. */
export function formatBytes(bytes: number): string {
    return format_bytes(bytes);
}
