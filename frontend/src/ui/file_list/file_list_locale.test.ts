import { afterEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { set_locale } from '../../i18n/i18n';
import { FILE_LIST_COLUMN_LABEL, file_list_column_label, fileListColumnMode } from './column_mode_store';
import { relativeTimeLabel } from './row_meta';

const NOW_MS = 1_800_000_000_000;
const NOW_SEC = Math.floor(NOW_MS / 1000);

describe('file list header and row labels in Vietnamese', () => {
    afterEach(async () => {
        fileListColumnMode.set('date');
        await set_locale('en');
    });

    it('names the second column for each mode in vi', async () => {
        await set_locale('vi');
        expect(FILE_LIST_COLUMN_LABEL.date).toBe('Ngày');
        expect(FILE_LIST_COLUMN_LABEL.location).toBe('Vị trí');
        expect(FILE_LIST_COLUMN_LABEL.purge).toBe('Thời gian còn lại');
    });

    it('updates the reactive header label with the mode and the language', async () => {
        await set_locale('vi');
        fileListColumnMode.set('purge');
        expect(get(file_list_column_label)).toBe('Thời gian còn lại');

        await set_locale('en');
        expect(get(file_list_column_label)).toBe('Time left');
    });

    it('phrases relative times in vi from the current locale', async () => {
        await set_locale('vi');
        expect(relativeTimeLabel(NOW_SEC - 20, NOW_MS)).toBe('Vừa xong');
        expect(relativeTimeLabel(NOW_SEC - 5 * 60, NOW_MS)).toBe('5 phút trước');
        expect(relativeTimeLabel(NOW_SEC - 2 * 3600, NOW_MS)).toBe('2 giờ trước');
        expect(relativeTimeLabel(NOW_SEC - 26 * 3600, NOW_MS)).toBe('Hôm qua');
        expect(relativeTimeLabel(NOW_SEC - 3 * 86_400, NOW_MS)).toBe('3 ngày trước');
    });
});
