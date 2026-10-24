import { readdirSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

type I18nModule = typeof import('./i18n');

const locales_dir = fileURLToPath(new URL('../locales', import.meta.url));

function load_json(path: string): Record<string, string> {
    return JSON.parse(readFileSync(path, 'utf8')) as Record<string, string>;
}

function namespaces_in(locale: string): string[] {
    return readdirSync(`${locales_dir}/${locale}`)
        .filter((name) => name.endsWith('.json'))
        .map((name) => name.replace(/\.json$/, ''))
        .sort();
}

function placeholders_of(text: string): string[] {
    return [...text.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort();
}

const PLURAL_KEY = /^(.+)_(zero|one|two|few|many|other)$/;

/**
 * Compares one namespace of a locale against English and returns human-readable problems.
 * Plural groups are keys `base_other` in English; the locale must have `base_<category>` for every
 * category Intl.PluralRules lists for it, plus `_other`. Extra keys are only allowed as plural variants.
 */
function parity_problems(locale: string, en: Record<string, string>, target: Record<string, string>): string[] {
    const problems: string[] = [];
    const plural_bases = new Set(Object.keys(en).filter((key) => key.endsWith('_other')).map((key) => key.slice(0, -'_other'.length)));
    const is_en_plural_variant = (key: string): boolean => {
        const match = PLURAL_KEY.exec(key);
        return match !== null && plural_bases.has(match[1]);
    };
    const categories = [...new Set([...new Intl.PluralRules(locale).resolvedOptions().pluralCategories, 'other'])];

    for (const key of Object.keys(en)) {
        if (is_en_plural_variant(key)) continue;
        if (!(key in target)) problems.push(`missing ${key}`);
    }
    for (const base of plural_bases) {
        for (const category of categories) {
            if (!(`${base}_${category}` in target)) problems.push(`missing ${base}_${category}`);
        }
    }
    for (const [key, value] of Object.entries(target)) {
        if (typeof value !== 'string' || value.trim() === '') {
            problems.push(`empty or non-string ${key}`);
            continue;
        }
        if (!(key in en) && !is_en_plural_variant(key)) {
            problems.push(`extra key ${key}`);
            continue;
        }
        const english_key = key in en ? key : `${PLURAL_KEY.exec(key)?.[1]}_other`;
        if (english_key in en && JSON.stringify(placeholders_of(value)) !== JSON.stringify(placeholders_of(en[english_key]))) {
            problems.push(`placeholders differ for ${key}`);
        }
    }
    return problems;
}

let storage: Map<string, string>;
let i18n: I18nModule;

function stub_storage(options: { broken?: boolean } = {}): void {
    storage = new Map();
    vi.stubGlobal('localStorage', {
        getItem: (key: string) => {
            if (options.broken) throw new Error('storage blocked');
            return storage.get(key) ?? null;
        },
        setItem: (key: string, value: string) => {
            if (options.broken) throw new Error('storage blocked');
            storage.set(key, value);
        },
    });
}

function stub_navigator(language: string): void {
    vi.stubGlobal('navigator', { language, languages: [language] });
}

beforeEach(async () => {
    vi.resetModules();
    stub_storage();
    stub_navigator('en-US');
    vi.stubGlobal('document', { documentElement: { lang: '' } });
    i18n = await import('./i18n');
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('interpolation', () => {
    it('replaces named placeholders', () => {
        const message = i18n.translate_message(
            { en: { 'demo.greet': 'Hello {name}, you have {count} new' } },
            'en',
            'demo.greet',
            { name: 'Thuận', count: 3 },
        );
        expect(message).toBe('Hello Thuận, you have 3 new');
    });

    it('keeps placeholders that have no matching param visible', () => {
        const message = i18n.translate_message({ en: { 'demo.x': 'Hi {name} {missing}' } }, 'en', 'demo.x', { name: 'A' });
        expect(message).toBe('Hi A {missing}');
    });

    it('works through the non-reactive t() helper', () => {
        expect(i18n.t('common.items', { count: 1 })).toBe('1 item');
        expect(i18n.t('common.items', { count: 5 })).toBe('5 items');
    });
});

describe('plurals', () => {
    it('selects _one and _other with Intl.PluralRules when count is given', () => {
        const dictionaries = { en: { 'demo.files_one': '{count} file', 'demo.files_other': '{count} files' } };
        expect(i18n.translate_message(dictionaries, 'en', 'demo.files', { count: 1 })).toBe('1 file');
        expect(i18n.translate_message(dictionaries, 'en', 'demo.files', { count: 0 })).toBe('0 files');
        expect(i18n.translate_message(dictionaries, 'en', 'demo.files', { count: 2 })).toBe('2 files');
    });

    it('uses the _other form for Vietnamese, which has no plural distinction', () => {
        const dictionaries = { vi: { 'demo.files_one': '{count} tệp', 'demo.files_other': '{count} tệp' } };
        expect(i18n.translate_message(dictionaries, 'vi', 'demo.files', { count: 1 })).toBe('1 tệp');
        expect(i18n.translate_message(dictionaries, 'vi', 'demo.files', { count: 7 })).toBe('7 tệp');
    });
});

describe('fallback', () => {
    it('falls back to English when the locale lacks the key', () => {
        const dictionaries = { en: { 'demo.only_en': 'English text' }, vi: {} };
        expect(i18n.translate_message(dictionaries, 'vi', 'demo.only_en')).toBe('English text');
    });

    it('falls back to the key itself when no language has it', () => {
        expect(i18n.translate_message({ en: {}, vi: {} }, 'vi', 'demo.nowhere')).toBe('demo.nowhere');
    });

    it('falls back to English plural forms when the locale has none', () => {
        const dictionaries = { en: { 'demo.items_one': '{count} item', 'demo.items_other': '{count} items' }, vi: {} };
        expect(i18n.translate_message(dictionaries, 'vi', 'demo.items', { count: 1 })).toBe('1 item');
    });
});

describe('set_locale', () => {
    it('switches the effective locale, the dictionary and <html lang>', async () => {
        expect(get(i18n.current_locale)).toBe('en');
        expect(i18n.t('common.cancel')).toBe('Cancel');

        await i18n.set_locale('vi');

        expect(get(i18n.current_locale)).toBe('vi');
        expect(i18n.t('common.cancel')).toBe('Hủy');
        expect(document.documentElement.lang).toBe('vi');

        await i18n.set_locale('en');
        expect(i18n.t('common.cancel')).toBe('Cancel');
        expect(document.documentElement.lang).toBe('en');
    });

    it('notifies t_store subscribers when the language changes', async () => {
        const seen: string[] = [];
        const unsubscribe = i18n.t_store.subscribe((translate) => seen.push(translate('common.save')));

        await i18n.set_locale('vi');
        unsubscribe();

        expect(seen[0]).toBe('Save');
        expect(seen[seen.length - 1]).toBe('Lưu');
    });

    it('resolves the system preference to the navigator language and keeps it distinct', async () => {
        stub_navigator('vi-VN');
        await i18n.set_locale('system');

        expect(get(i18n.locale_preference)).toBe('system');
        expect(get(i18n.current_locale)).toBe('vi');
    });

    it('resolves a regional system language to its translated folder', async () => {
        stub_navigator('de-AT');
        await i18n.set_locale('system');

        expect(i18n.resolve_locale('system')).toBe('de');
        expect(get(i18n.current_locale)).toBe('de');
        expect(document.documentElement.lang).toBe('de');
    });

    it('shows English for a language that is not supported at all', async () => {
        stub_navigator('klingon-QO');
        await i18n.set_locale('system');
        expect(get(i18n.current_locale)).toBe('en');
    });

    it('persists the preference under tdrive_locale', async () => {
        await i18n.set_locale('vi');
        expect(storage.get('tdrive_locale')).toBe('vi');

        await i18n.set_locale('system');
        expect(storage.get('tdrive_locale')).toBe('system');
    });

    it('applies the last of overlapping calls', async () => {
        const first = i18n.set_locale('vi');
        const second = i18n.set_locale('en');
        await Promise.all([first, second]);

        expect(get(i18n.current_locale)).toBe('en');
        expect(i18n.t('common.cancel')).toBe('Cancel');
    });
});

describe('resolve_initial_locale', () => {
    it('uses a saved explicit locale', () => {
        storage.set('tdrive_locale', 'vi');
        expect(i18n.resolve_initial_locale()).toBe('vi');
        expect(i18n.resolve_initial_preference()).toBe('vi');
    });

    it('resolves a saved system preference against navigator.language', () => {
        storage.set('tdrive_locale', 'system');
        stub_navigator('vi');
        expect(i18n.resolve_initial_locale()).toBe('vi');
        expect(i18n.resolve_initial_preference()).toBe('system');
    });

    it('defaults to system when nothing is saved, and ignores invalid values', () => {
        stub_navigator('en-GB');
        expect(i18n.resolve_initial_locale()).toBe('en');

        storage.set('tdrive_locale', 'klingon');
        expect(i18n.resolve_initial_preference()).toBe('system');
    });

    it('survives blocked storage by following the system language', async () => {
        stub_storage({ broken: true });
        stub_navigator('vi-VN');
        expect(i18n.resolve_initial_locale()).toBe('vi');

        await expect(i18n.set_locale('en')).resolves.toBeUndefined();
        expect(get(i18n.current_locale)).toBe('en');
    });
});

describe('formatting', () => {
    it('formats bytes in the app style for English', async () => {
        await i18n.set_locale('en');
        expect(i18n.format_bytes(0)).toBe('0 B');
        expect(i18n.format_bytes(512)).toBe('512 B');
        expect(i18n.format_bytes(1024)).toBe('1 KB');
        expect(i18n.format_bytes(1536)).toBe('1.5 KB');
        expect(i18n.format_bytes(1048576)).toBe('1 MB');
        expect(i18n.format_bytes(1000 * 1024)).toBe('1000 KB');
    });

    it('uses the locale decimal separator for bytes in Vietnamese', async () => {
        await i18n.set_locale('vi');
        expect(i18n.format_bytes(1536)).toBe('1,5 KB');
    });

    it('formats numbers with the current locale grouping', async () => {
        await i18n.set_locale('en');
        expect(i18n.format_number(1234567)).toBe('1,234,567');
        await i18n.set_locale('vi');
        expect(i18n.format_number(1234567)).toBe('1.234.567');
    });

    it('formats dates with the current locale', async () => {
        const date = new Date(2026, 0, 5);
        await i18n.set_locale('en');
        expect(i18n.format_date(date, 'medium')).toContain('2026');
        expect(i18n.format_date(date, 'medium')).toContain('Jan');
        await i18n.set_locale('vi');
        expect(i18n.format_date(date, 'medium')).toContain('2026');
    });
});

describe('system resolution', () => {
    it.each<[string[], string]>([
        [['zh-HK'], 'zh-TW'],
        [['zh-MO'], 'zh-TW'],
        [['zh-Hant-SG'], 'zh-TW'],
        [['zh-TW'], 'zh-TW'],
        [['zh-SG'], 'zh-CN'],
        [['zh-Hans-HK'], 'zh-CN'],
        [['zh'], 'zh-CN'],
        [['pt-PT'], 'pt-BR'],
        [['pt'], 'pt-BR'],
        [['pt-BR'], 'pt-BR'],
        [['es-MX'], 'es'],
        [['en-GB'], 'en'],
        [['fr-CA'], 'fr'],
        [['ja-JP'], 'ja'],
        [['klingon', 'vi'], 'vi'],
        [['xx-YY'], 'en'],
        [[], 'en'],
    ])('maps %j to %s', (languages, expected) => {
        expect(i18n.resolve_system_locale(languages)).toBe(expected);
    });

    it('walks navigator.languages in preference order', () => {
        vi.stubGlobal('navigator', { language: 'de-AT', languages: ['de-AT', 'vi'] });
        expect(i18n.resolve_locale('system')).toBe('de');
        vi.stubGlobal('navigator', { language: 'xx', languages: ['xx', 'ko-KR'] });
        expect(i18n.resolve_locale('system')).toBe('ko');
    });

    it('exposes metadata for every supported locale', () => {
        expect(i18n.SUPPORTED_LOCALES.map((info) => info.code)).toContain('zh-CN');
        expect(i18n.locale_info('pt-BR').native_name).toBe('Português (Brasil)');
        expect(i18n.locale_info('vi').english_name).toBe('Vietnamese');
    });
});

describe('plural categories', () => {
    it('uses few and many for Russian counts', () => {
        const dictionaries = {
            ru: { 'demo.x_one': 'one', 'demo.x_few': 'few', 'demo.x_many': 'many', 'demo.x_other': 'other' },
        };
        const form = (count: number) => i18n.translate_message(dictionaries, 'ru', 'demo.x', { count });
        expect(form(1)).toBe('one');
        expect(form(21)).toBe('one');
        expect(form(3)).toBe('few');
        expect(form(22)).toBe('few');
        expect(form(5)).toBe('many');
        expect(form(11)).toBe('many');
    });

    it('uses only _other for Japanese, which has no plural distinction', () => {
        const dictionaries = { ja: { 'demo.x_other': '{count}件' } };
        expect(i18n.translate_message(dictionaries, 'ja', 'demo.x', { count: 1 })).toBe('1件');
        expect(i18n.translate_message(dictionaries, 'ja', 'demo.x', { count: 9 })).toBe('9件');
    });
});

describe('locale dictionaries', () => {
    const en_namespaces = namespaces_in('en');
    const locale_folders = readdirSync(locales_dir, { withFileTypes: true })
        .filter((entry) => entry.isDirectory() && entry.name !== 'en')
        .map((entry) => entry.name)
        .sort();

    it('has English namespaces and only supported locale folders', () => {
        expect(en_namespaces.length).toBeGreaterThan(0);
        const supported = new Set<string>(i18n.SUPPORTED_LOCALES.map((info) => info.code));
        for (const folder of [...locale_folders, 'en']) {
            expect(supported.has(folder), `folder ${folder} is not in SUPPORTED_LOCALES`).toBe(true);
        }
    });

    it('lists only valid BCP-47 codes', () => {
        for (const info of i18n.SUPPORTED_LOCALES) {
            expect(() => Intl.getCanonicalLocales(info.code), info.code).not.toThrow();
        }
    });

    it('keeps every existing locale folder complete against en', () => {
        const problems: string[] = [];
        for (const locale of locale_folders) {
            expect(namespaces_in(locale), `namespaces of ${locale}`).toEqual(en_namespaces);
            for (const namespace of en_namespaces) {
                const en = load_json(`${locales_dir}/en/${namespace}.json`);
                const target = load_json(`${locales_dir}/${locale}/${namespace}.json`);
                for (const problem of parity_problems(locale, en, target)) {
                    problems.push(`${locale}/${namespace}: ${problem}`);
                }
            }
        }
        expect(problems).toEqual([]);
    });

    it('catches missing plural categories and placeholder drift in the checker itself', () => {
        const en = { 'a_one': '{count} file', 'a_other': '{count} files', 'b': 'Hi {name}' };
        const ok_ru = { 'a_one': '{count} f', 'a_few': '{count} f', 'a_many': '{count} f', 'a_other': '{count} f', 'b': 'Hi {name}' };
        expect(parity_problems('ru', en, ok_ru)).toEqual([]);
        expect(parity_problems('ru', en, { 'a_one': '{count} f', 'a_other': '{count} f', 'b': 'Hi {name}' }).sort())
            .toEqual(['missing a_few', 'missing a_many']);
        expect(parity_problems('ja', en, { 'a_other': '{count}', 'b': 'Hi {name}' })).toEqual([]);
        expect(parity_problems('ja', en, { 'a_other': '{count}', 'b': 'Hi {user}' })).toEqual(['placeholders differ for b']);
        expect(parity_problems('ja', en, { 'a_other': '{count}', 'b': 'Hi {name}', 'c': 'x' })).toEqual(['extra key c']);
    });
});
