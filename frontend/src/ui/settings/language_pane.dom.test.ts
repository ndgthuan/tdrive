import { afterEach, describe, expect, it } from 'vitest';
import { flushSync, mount, tick, unmount } from 'svelte';
import LanguagePane from './language_pane.svelte';
import { current_locale, locale_preference, set_locale } from '../../i18n/i18n';

let component: Record<string, unknown> | null = null;
let host: HTMLElement | null = null;

async function settle(): Promise<void> {
    for (let i = 0; i < 6; i += 1) {
        await Promise.resolve();
        await tick();
    }
    flushSync();
}

function radio(value: string): HTMLInputElement {
    const found = host?.querySelector<HTMLInputElement>(`input[type="radio"][value="${value}"]`);
    if (!found) throw new Error(`missing ${value} radio`);
    return found;
}

afterEach(async () => {
    if (component) await unmount(component);
    host?.remove();
    component = null;
    host = null;
    await set_locale('en');
});

describe('language pane', () => {
    it('marks the saved preference and names the resolved language in System', async () => {
        await set_locale('vi');
        host = document.createElement('div');
        document.body.appendChild(host);
        component = mount(LanguagePane, { target: host });
        await settle();

        expect(radio('vi').checked).toBe(true);
        expect(radio('en').checked).toBe(false);
        expect(host.textContent).toContain('Theo hệ thống (Tiếng Việt)');
    });

    it('switches the interface language at once when a different option is chosen', async () => {
        await set_locale('en');
        host = document.createElement('div');
        document.body.appendChild(host);
        component = mount(LanguagePane, { target: host });
        await settle();

        expect(host.textContent).toContain('Interface language');
        radio('vi').click();
        await settle();

        expect(locale_preference).toBeDefined();
        let preference = '';
        const stop = locale_preference.subscribe((value) => { preference = value; });
        stop();
        expect(preference).toBe('vi');
        let locale = '';
        const stop_locale = current_locale.subscribe((value) => { locale = value; });
        stop_locale();
        expect(locale).toBe('vi');
        expect(document.documentElement.lang).toBe('vi');
        expect(host.textContent).toContain('Ngôn ngữ giao diện');
    });

    it('offers System, Tiếng Việt and English, and shows a date example', async () => {
        await set_locale('en');
        host = document.createElement('div');
        document.body.appendChild(host);
        component = mount(LanguagePane, { target: host });
        await settle();

        const values = Array.from(host.querySelectorAll<HTMLInputElement>('input[type="radio"]')).map((input) => input.value);
        expect(values).toEqual(['system', 'vi', 'en']);
        expect(host.querySelector('.date-example')?.textContent?.trim()).not.toBe('');
    });
});
