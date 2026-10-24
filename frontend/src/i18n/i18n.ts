import { derived, get, writable, type Readable } from 'svelte/store';

export interface LocaleInfo {
    /** BCP-47 tag, also the folder name under src/locales/. */
    readonly code: string;
    /** The language named in its own words, shown in the language pane. */
    readonly native_name: string;
    /** The language named in English. */
    readonly english_name: string;
}

/**
 * Every language the app can show. Listing a locale here does not require its folder:
 * a locale without src/locales/<code>/ silently shows English.
 */
export const SUPPORTED_LOCALES = [
    { code: 'en', native_name: 'English', english_name: 'English' },
    { code: 'vi', native_name: 'Tiếng Việt', english_name: 'Vietnamese' },
    { code: 'zh-CN', native_name: '简体中文', english_name: 'Chinese (Simplified)' },
    { code: 'zh-TW', native_name: '繁體中文', english_name: 'Chinese (Traditional)' },
    { code: 'ja', native_name: '日本語', english_name: 'Japanese' },
    { code: 'ko', native_name: '한국어', english_name: 'Korean' },
    { code: 'es', native_name: 'Español', english_name: 'Spanish' },
    { code: 'pt-BR', native_name: 'Português (Brasil)', english_name: 'Portuguese (Brazil)' },
    { code: 'fr', native_name: 'Français', english_name: 'French' },
    { code: 'de', native_name: 'Deutsch', english_name: 'German' },
    { code: 'it', native_name: 'Italiano', english_name: 'Italian' },
    { code: 'ru', native_name: 'Русский', english_name: 'Russian' },
    { code: 'uk', native_name: 'Українська', english_name: 'Ukrainian' },
    { code: 'pl', native_name: 'Polski', english_name: 'Polish' },
    { code: 'nl', native_name: 'Nederlands', english_name: 'Dutch' },
    { code: 'tr', native_name: 'Türkçe', english_name: 'Turkish' },
    { code: 'id', native_name: 'Bahasa Indonesia', english_name: 'Indonesian' },
    { code: 'th', native_name: 'ไทย', english_name: 'Thai' },
    { code: 'hi', native_name: 'हिन्दी', english_name: 'Hindi' },
] as const satisfies readonly LocaleInfo[];

export type Locale = (typeof SUPPORTED_LOCALES)[number]['code'];
/** What the user chose. `system` follows navigator.languages and is not itself a language. */
export type LocalePreference = 'system' | Locale;
export type TranslationParams = Record<string, string | number>;
export type Translator = (key: string, params?: TranslationParams) => string;
export type DateStyle = 'full' | 'long' | 'medium' | 'short';
type Dictionary = Record<string, string>;
type NamespaceLoader = () => Promise<Dictionary>;

export const LOCALE_STORAGE_KEY = 'tdrive_locale';
const DEFAULT_LOCALE: Locale = 'en';
const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const;

/** Metadata for a supported locale. */
export function locale_info(locale: Locale): (typeof SUPPORTED_LOCALES)[number] {
    const found = SUPPORTED_LOCALES.find((info) => info.code === locale);
    if (!found) throw new Error(`Unsupported locale ${locale}`);
    return found;
}

// English ships in the main bundle; every other locale is split into lazy chunks per namespace.
const english_modules = import.meta.glob('../locales/en/*.json', {
    eager: true,
    import: 'default',
}) as Record<string, Dictionary>;

// Every non-English folder. English is excluded because it is already in the main bundle.
const all_lazy_modules = import.meta.glob(['../locales/*/*.json', '!../locales/en/*.json'], {
    import: 'default',
}) as Record<string, NamespaceLoader>;

function namespace_of(module_path: string): string {
    const match = /([^/]+)\.json$/.exec(module_path);
    if (!match) throw new Error(`Locale file has no namespace: ${module_path}`);
    return match[1];
}

function folder_of(module_path: string): string {
    const match = /locales\/([^/]+)\/[^/]+\.json$/.exec(module_path);
    if (!match) throw new Error(`Locale file is not inside a locale folder: ${module_path}`);
    return match[1];
}

/** Namespace loaders grouped by locale folder, e.g. `zh-CN -> [['common', load], ...]`. */
const lazy_loaders_by_locale = new Map<string, Array<[string, NamespaceLoader]>>();
for (const [path, load] of Object.entries(all_lazy_modules)) {
    const folder = folder_of(path);
    const list = lazy_loaders_by_locale.get(folder) ?? [];
    list.push([namespace_of(path), load]);
    lazy_loaders_by_locale.set(folder, list);
}

/** Flattens `{ namespace: { key: text } }` into `{ 'namespace.key': text }`. */
function flatten_namespaces(entries: Array<[string, Dictionary]>): Dictionary {
    const flat: Dictionary = {};
    for (const [namespace, dictionary] of entries) {
        for (const [key, text] of Object.entries(dictionary)) {
            flat[`${namespace}.${key}`] = text;
        }
    }
    return flat;
}

const english_messages = flatten_namespaces(
    Object.entries(english_modules).map(([path, dictionary]) => [namespace_of(path), dictionary]),
);

/**
 * Loads every namespace of a locale folder. Returns undefined when the folder does not exist,
 * which callers treat as "use English" without warning.
 */
async function load_dictionary(locale: Locale): Promise<Dictionary | undefined> {
    if (locale === DEFAULT_LOCALE) return english_messages;
    const loaders = lazy_loaders_by_locale.get(locale);
    if (!loaders || loaders.length === 0) return undefined;
    const entries = await Promise.all(
        loaders.map(async ([namespace, load]) => [namespace, await load()] as [string, Dictionary]),
    );
    return flatten_namespaces(entries);
}

function is_locale(value: unknown): value is Locale {
    return typeof value === 'string' && SUPPORTED_LOCALES.some((info) => info.code === value);
}

function is_preference(value: unknown): value is LocalePreference {
    return value === 'system' || is_locale(value);
}

/**
 * Maps one BCP-47 tag to a supported locale: exact match first, then the rules for
 * Chinese (script or region decides Traditional vs Simplified) and Portuguese (Brazil),
 * then any supported locale with the same language subtag. Undefined when none fits.
 */
export function match_locale_tag(tag: string): Locale | undefined {
    const normalized = tag.trim().toLowerCase();
    if (!normalized) return undefined;

    const exact = SUPPORTED_LOCALES.find((info) => info.code.toLowerCase() === normalized);
    if (exact) return exact.code;

    const parts = normalized.split('-');
    const language = parts[0];
    if (language === 'zh') {
        // An explicit script wins; otherwise Taiwan, Hong Kong and Macau mean Traditional.
        if (parts.includes('hant')) return 'zh-TW';
        if (parts.includes('hans')) return 'zh-CN';
        return parts.slice(1).some((part) => ['tw', 'hk', 'mo'].includes(part)) ? 'zh-TW' : 'zh-CN';
    }
    if (language === 'pt') return 'pt-BR';

    const same_language = SUPPORTED_LOCALES.find((info) => info.code.split('-')[0].toLowerCase() === language);
    return same_language?.code;
}

/** The best supported locale for languages in preference order (navigator.languages). */
export function resolve_system_locale(languages: readonly string[]): Locale {
    for (const tag of languages) {
        const matched = match_locale_tag(tag);
        if (matched) return matched;
    }
    return DEFAULT_LOCALE;
}

/** Effective language for a preference. `system` maps navigator.languages to a supported locale. */
export function resolve_locale(preference: LocalePreference): Locale {
    if (preference !== 'system') return preference;
    const nav = (typeof navigator !== 'undefined' ? navigator : {}) as Partial<Navigator>;
    const languages = [...(nav.languages ?? []), nav.language ?? ''];
    return resolve_system_locale(languages);
}

/** The stored preference, or `system` when nothing valid is stored or storage is unavailable. */
export function read_saved_preference(): LocalePreference {
    try {
        const saved = localStorage.getItem(LOCALE_STORAGE_KEY);
        if (is_preference(saved)) return saved;
    } catch {
        // Storage can be blocked in hardened webviews; follow the system language.
    }
    return 'system';
}

export function resolve_initial_preference(): LocalePreference {
    return read_saved_preference();
}

/** Effective language at startup: the saved choice, with `system` resolved against navigator.languages. */
export function resolve_initial_locale(): Locale {
    return resolve_locale(read_saved_preference());
}

const plural_rules_cache = new Map<Locale, Intl.PluralRules>();

/**
 * Finds `key` in one dictionary. With a numeric count, the plural form Intl.PluralRules picks
 * (`key_zero|one|two|few|many|other`) wins, and `key_other` is the fallback. Then the plain key.
 */
function lookup(dictionary: Dictionary | undefined, locale: Locale, key: string, count: number | undefined): string | undefined {
    if (!dictionary) return undefined;
    if (count !== undefined) {
        let rules = plural_rules_cache.get(locale);
        if (!rules) {
            rules = new Intl.PluralRules(locale);
            plural_rules_cache.set(locale, rules);
        }
        const form = rules.select(count);
        const plural_hit = dictionary[`${key}_${form}`] ?? dictionary[`${key}_other`];
        if (plural_hit !== undefined) return plural_hit;
    }
    return dictionary[key];
}

function interpolate(template: string, params: TranslationParams | undefined): string {
    if (!params) return template;
    return template.replace(/\{(\w+)\}/g, (placeholder, name: string) =>
        Object.prototype.hasOwnProperty.call(params, name) ? String(params[name]) : placeholder,
    );
}

/**
 * Pure lookup: locale dictionary, then English, then the key itself.
 * A missing key or missing locale folder falls back to English per key.
 * `key_<category>` is chosen with Intl.PluralRules when `params.count` is a number.
 */
export function translate_message(
    dictionaries: Partial<Record<Locale, Dictionary>>,
    locale: Locale,
    key: string,
    params?: TranslationParams,
): string {
    const count = typeof params?.count === 'number' ? params.count : undefined;
    const template =
        lookup(dictionaries[locale], locale, key, count) ??
        lookup(dictionaries[DEFAULT_LOCALE], DEFAULT_LOCALE, key, count) ??
        key;
    return interpolate(template, params);
}

// --- Reactive state -------------------------------------------------------

const locale_store = writable<Locale>(DEFAULT_LOCALE);
const preference_store = writable<LocalePreference>('system');
const dictionary_store = writable<Partial<Record<Locale, Dictionary>>>({ [DEFAULT_LOCALE]: english_messages });

/** Effective language (a code from SUPPORTED_LOCALES). Read-only from outside; change it with set_locale. */
export const current_locale: Readable<Locale> = { subscribe: locale_store.subscribe };
/** The user's preference, including 'system'. Use it to show the selected option in settings. */
export const locale_preference: Readable<LocalePreference> = { subscribe: preference_store.subscribe };

/**
 * Reactive translator. In .svelte files: `{$t_store('common.ok')}`.
 * It re-derives when the locale or a dictionary changes, so the template updates.
 */
export const t_store: Readable<Translator> = derived(
    [locale_store, dictionary_store],
    ([locale, dictionaries]) => (key: string, params?: TranslationParams) =>
        translate_message(dictionaries, locale, key, params),
);

/** Non-reactive translator for plain .ts modules. Reads the current locale at call time. */
export function t(key: string, params?: TranslationParams): string {
    return translate_message(get(dictionary_store), get(locale_store), key, params);
}

function persist_preference(preference: LocalePreference): void {
    try {
        localStorage.setItem(LOCALE_STORAGE_KEY, preference);
    } catch {
        // The choice still applies for this session if storage is unavailable.
    }
}

let latest_request = 0;

/**
 * Applies a language preference. Non-English dictionaries load on first use.
 * Keeps English when the locale has no folder or its dictionary cannot load, and sets <html lang>.
 * Overlapping calls are resolved last-wins.
 */
export async function set_locale(preference: LocalePreference): Promise<void> {
    const request = ++latest_request;
    const locale = resolve_locale(preference);
    preference_store.set(preference);
    persist_preference(preference);

    if (!get(dictionary_store)[locale]) {
        try {
            const dictionary = await load_dictionary(locale);
            if (request !== latest_request) return;
            if (dictionary) dictionary_store.update((current) => ({ ...current, [locale]: dictionary }));
        } catch (error) {
            console.warn(`Could not load the ${locale} translations; using English.`, error);
        }
    }
    if (request !== latest_request) return;

    const effective: Locale = get(dictionary_store)[locale] ? locale : DEFAULT_LOCALE;
    locale_store.set(effective);
    if (typeof document !== 'undefined') {
        document.documentElement.lang = effective;
    }
}

// --- Formatting (uses the current effective locale) -----------------------

/** Formats a date in the current locale, e.g. `format_date(new Date(), 'medium')`. */
export function format_date(date: Date | number, style: DateStyle = 'medium'): string {
    return new Intl.DateTimeFormat(get(locale_store), { dateStyle: style }).format(date);
}

export function format_number(value: number): string {
    return new Intl.NumberFormat(get(locale_store)).format(value);
}

/**
 * Byte sizes in the app's existing style: base 1024, units B/KB/MB/GB/TB,
 * at most one decimal, no grouping ("1.5 KB", "1000 KB"). Only the decimal
 * separator follows the locale ("1,5 KB" in Vietnamese).
 */
export function format_bytes(bytes: number): string {
    if (!(bytes > 0)) return '0 B';
    const unit_index = Math.min(Math.max(Math.floor(Math.log(bytes) / Math.log(1024)), 0), BYTE_UNITS.length - 1);
    const value = bytes / 1024 ** unit_index;
    const number_text = new Intl.NumberFormat(get(locale_store), {
        maximumFractionDigits: 1,
        useGrouping: false,
    }).format(value);
    return `${number_text} ${BYTE_UNITS[unit_index]}`;
}
