import { afterEach, describe, expect, it } from 'vitest';
import { set_locale, t } from '../../i18n/i18n';

describe('selection count in Vietnamese', () => {
    afterEach(async () => {
        await set_locale('en');
    });

    it('uses the one and other plural forms for the count', async () => {
        await set_locale('vi');
        expect(t('selection.count', { count: 1 })).toBe('1 đã chọn');
        expect(t('selection.count', { count: 12 })).toBe('12 đã chọn');
    });

    it('keeps the English plural wording by default', () => {
        expect(t('selection.count', { count: 1 })).toBe('1 selected');
        expect(t('selection.count', { count: 3 })).toBe('3 selected');
    });

    it('translates the bar actions', async () => {
        await set_locale('vi');
        expect(t('selection.download')).toBe('Tải xuống');
        expect(t('selection.move')).toBe('Di chuyển');
        expect(t('selection.delete')).toBe('Xóa');
        expect(t('selection.done')).toBe('Xong');
    });
});
