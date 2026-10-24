import { current_locale, format_date, t } from '../../i18n/i18n';
import { get } from 'svelte/store';
// Builds the lightbox info panel (the Apple-style "ⓘ" card). Pure rendering:
// given the active item, the loaded full-image data URL, and the displayed
// dimensions, it parses EXIF and returns the panel's inner HTML. Every dynamic
// string is escaped; sections that have no data are omitted.

import { escapeHtml, formatBytes, splitNameAndExt } from '../../utils';
import { state } from '../../state';
import { parseExif, dataUrlToBytes, type ImageExif } from '../exif';

export interface PreviewInfoItem {
    name?: string;
    size?: number;
    encrypted?: boolean;
    uploaderId?: number;
    uploadTime?: number;
}

export interface InfoInput {
    item: PreviewInfoItem;
    fullSrc: string; // data URL of the full image, or "" if not loaded yet
    naturalWidth: number;
    naturalHeight: number;
}

export function renderImageInfoHTML(input: InfoInput): string {
    const { item, fullSrc, naturalWidth, naturalHeight } = input;
    const exif: ImageExif = fullSrc ? exifFromDataUrl(fullSrc) : {};

    const when = exif.takenTime || Number(item.uploadTime || 0);
    const width = exif.width || naturalWidth || 0;
    const height = exif.height || naturalHeight || 0;

    const sections: string[] = [];

    if (when > 0) {
        sections.push(`<div class="info-date">${escapeHtml(formatWhen(when))}</div>`);
    }

    const details: string[] = [];
    if (width > 0 && height > 0) {
        details.push(row(t('messages.info_dimensions'), `${width} × ${height}${megapixels(width, height)}`));
    }
    if (Number(item.size || 0) > 0) {
        details.push(row(t('messages.info_size'), formatBytes(Number(item.size))));
    }
    const ext = splitNameAndExt(String(item.name || '')).ext;
    if (ext && ext !== 'FILE') {
        details.push(row(t('messages.info_type'), ext));
    }
    const uploader = uploaderName(item);
    if (uploader) {
        details.push(row(t('messages.info_uploaded_by'), uploader));
    }
    if (item.encrypted) {
        details.push(row(t('messages.info_encrypted'), t('messages.info_yes')));
    }
    if (details.length) {
        sections.push(section(t('messages.info_details'), details.join('')));
    }

    const camera = cameraBlock(exif);
    if (camera) sections.push(section(t('messages.info_camera'), camera));

    if (exif.gps) {
        const { lat, lon } = exif.gps;
        const open = `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(`${lat},${lon}`)}`;
        const body = row(t('messages.info_coordinates'), escapeHtml(`${lat}, ${lon}`))
            + `<button class="info-map-link" type="button" data-map-url="${escapeHtml(open)}" aria-label="${t('messages.info_open_maps_aria')}">${t('messages.info_open_maps')}</button>`;
        sections.push(section(t('messages.info_location'), body));
    }

    if (!sections.length) {
        return `<div class="info-empty">${t('messages.info_empty')}</div>`;
    }
    return sections.join('');
}

function exifFromDataUrl(dataUrl: string): ImageExif {
    const bytes = dataUrlToBytes(dataUrl);
    return bytes ? parseExif(bytes) : {};
}

function cameraBlock(exif: ImageExif): string {
    const rows: string[] = [];

    const device = [exif.make, exif.model].filter(Boolean).join(' ').trim();
    if (device) rows.push(row(t('messages.info_camera'), escapeHtml(dedupeMake(exif.make, exif.model) || device)));
    if (exif.lens) rows.push(row(t('messages.info_lens'), escapeHtml(exif.lens)));

    const settings = [
        exif.fNumber ? `ƒ${exif.fNumber}` : '',
        exif.exposureTime || '',
        exif.focalLength ? `${exif.focalLength} mm` : '',
        exif.iso ? `ISO ${exif.iso}` : '',
    ].filter(Boolean);
    if (settings.length) rows.push(row(t('messages.info_exposure'), escapeHtml(settings.join(' · '))));

    return rows.join('');
}

// Camera make is often a prefix of the model ("Apple" + "Apple iPhone 14"),
// so collapse the redundancy the way Apple Photos does.
function dedupeMake(make?: string, model?: string): string {
    const mk = (make || '').trim();
    const md = (model || '').trim();
    if (!mk) return md;
    if (!md) return mk;
    if (md.toLowerCase().startsWith(mk.toLowerCase())) return md;
    return `${mk} ${md}`;
}

function uploaderName(item: PreviewInfoItem): string {
    if (state.activeChannel?.kind !== 'shared') return '';
    const id = Number(item.uploaderId || 0);
    if (id <= 0) return '';
    return escapeHtml(state.userNames.get(String(id)) || '');
}

function megapixels(w: number, h: number): string {
    const mp = (w * h) / 1_000_000;
    if (mp < 0.1) return '';
    return ` · ${Math.round(mp * 10) / 10} MP`;
}

function formatWhen(unixSec: number): string {
    const d = new Date(unixSec * 1000);
    const date = format_date(d, 'full');
    const time = new Intl.DateTimeFormat(get(current_locale), { hour: 'numeric', minute: '2-digit' }).format(d);
    return `${date} · ${time}`;
}

function section(title: string, body: string): string {
    return `<div class="info-section"><div class="info-section-title">${escapeHtml(title)}</div>${body}</div>`;
}

// row builds a label/value line. The value is inserted as-is, so callers escape
// any dynamic content before passing it in.
function row(label: string, value: string): string {
    return `<div class="info-row"><span class="info-label">${escapeHtml(label)}</span><span class="info-value">${value}</span></div>`;
}
