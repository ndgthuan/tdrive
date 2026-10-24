import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import CountryCodePicker from './country_code_picker.svelte';
import { normalizePhoneNumber } from './phone_number';
import { set_locale } from '../../i18n/i18n';

vi.mock('../../api', () => ({ isMobilePlatform: () => true }));

let host: HTMLFormElement;
let app: Record<string, unknown>;

beforeEach(async () => {
    await set_locale('vi');
    host = document.createElement('form');
    document.body.appendChild(host);
    app = mount(CountryCodePicker, {
        target: host,
        props: { country: undefined, disabled: false, onSelect: vi.fn() },
    });
    flushSync();
});

afterEach(async () => {
    try {
        await unmount(app);
    } finally {
        host.remove();
        await set_locale('en');
    }
});

describe('auth screens in Vietnamese', () => {
    it('renders the country trigger with Vietnamese text and accessible name', () => {
        const trigger = host.querySelector<HTMLButtonElement>('#telegram-country');
        expect(trigger?.textContent).toContain('Quốc gia');
        expect(trigger?.getAttribute('aria-label')).toBe('Chọn mã quốc gia');
    });

    it('returns phone validation errors in Vietnamese', () => {
        expect(normalizePhoneNumber('')).toEqual({ error: 'Nhập số điện thoại của tài khoản Telegram của bạn.' });
    });
});
