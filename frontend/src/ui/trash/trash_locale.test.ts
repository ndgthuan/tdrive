import { afterEach, describe, expect, it } from 'vitest';
import { set_locale } from '../../i18n/i18n';
import type { TrashEntry } from '../../api/trash';
import { purgeCountdown, trashSummary } from './trash_view';

const HOUR = 60 * 60_000;
const DAY = 24 * HOUR;
const NOW = new Date(2026, 5, 10, 12, 0, 0).getTime();

function entry(size: number): TrashEntry {
    return { objectId: `f:${size}`, kind: 'file', name: 'a.pdf', parentPath: '', size, deletedAt: NOW, purgeAfter: NOW + DAY } as TrashEntry;
}

describe('trash time-left and summary in Vietnamese', () => {
    afterEach(async () => {
        await set_locale('en');
    });

    it('counts hours and days left in vi', async () => {
        await set_locale('vi');
        expect(purgeCountdown(NOW + 30 * 60_000, NOW).label).toBe('Còn chưa đến 1 giờ');
        expect(purgeCountdown(NOW + 5 * HOUR, NOW).label).toBe('Còn 5 giờ');
        expect(purgeCountdown(NOW + 3 * DAY + HOUR, NOW)).toEqual({ label: 'Còn 3 ngày', urgent: false });
    });

    it('keeps the English countdown wording by default', () => {
        expect(purgeCountdown(NOW + 5 * HOUR, NOW).label).toBe('5 hours left');
        expect(purgeCountdown(NOW + 3 * DAY + HOUR, NOW).label).toBe('3 days left');
    });

    it('counts items in the dialog summary in vi', async () => {
        await set_locale('vi');
        expect(trashSummary([])).toBe('');
        expect(trashSummary([entry(0)])).toBe('1 mục');
        expect(trashSummary([entry(0), entry(0)])).toBe('2 mục');
        expect(trashSummary([entry(1536)])).toBe('1 mục · 1,5 KB');
    });
});
