import { afterEach, describe, expect, it } from 'vitest';
import { set_locale, t } from '../../i18n/i18n';

describe('modal and message strings in Vietnamese', () => {
    afterEach(async () => {
        await set_locale('en');
    });

    it('translates a message key once the locale switches to vi', async () => {
        expect(t('messages.preview_default_title')).toBe('Preview');
        await set_locale('vi');
        expect(t('messages.preview_default_title')).toBe('Xem trước');
        expect(t('messages.notify_login_failed')).toBe('Đăng nhập thất bại');
    });

    it('uses the plural form and interpolation for vi', async () => {
        await set_locale('vi');
        expect(t('messages.upload_batch_failed', { count: 2 })).toBe('Không thể tải lên 2 tệp');
        expect(t('messages.source_removed', { title: 'Lab' })).toBe('Đã xóa Lab');
    });
});
