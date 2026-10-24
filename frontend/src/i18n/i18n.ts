import { derived, get, writable, type Readable } from 'svelte/store';

export const SUPPORTED_LOCALES = ['en', 'vi'] as const;
export type Locale = (typeof SUPPORTED_LOCALES)[number];
/** What the user chose. `system` follows navigator.language and is not itself a language. */
export type LocalePreference = 'system' | Locale;
export type TranslationParams = Record<string, string | number>;
export type Translator = (key: string, params?: TranslationParams) => string;
export type DateStyle = 'full' | 'long' | 'medium' | 'short';
type Dictionary = Record<string, string>;

export const LOCALE_STORAGE_KEY = 'tdrive_locale';
const DEFAULT_LOCALE: Locale = 'en';
const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'] as const;

// English ships in the main bundle; every other language is a separate chunk.
const english_modules = import.meta.glob('../locales/en/*.json', {
    eager: true,
    import: 'default',
}) as Record<string, Dictionary>;

const lazy_locale_modules: Partial<Record<Locale, Record<string, () => Promise<Dictionary>>>> = {
    vi: import.meta.glob('../locales/vi/*.json', { import: 'default' }) as Record<string, () => Promise<Dictionary>>,
};

function namespace_of(module_path: string): string {
    const match = /([^/]+)\.json$/.exec(module_path);
    if (!match) throw new Error(`Locale file has no namespace: ${module_path}`);
    return match[1];
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

async function load_dictionary(locale: Locale): Promise<Dictionary> {
    if (locale === DEFAULT_LOCALE) return english_messages;
    const loaders = lazy_locale_modules[locale];
    if (!loaders) throw new Error(`No dictionaries registered for locale ${locale}`);
    const entries = await Promise.all(
        Object.entries(loaders).map(async ([path, load]) => [namespace_of(path), await load()] as [string, Dictionary]),
    );
    return flatten_namespaces(entries);
}

function is_locale(value: unknown): value is Locale {
    return typeof value === 'string' && (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

function is_preference(value: unknown): value is LocalePreference {
    return value === 'system' || is_locale(value);
}

/** Effective language for a preference. `system` maps navigator.language to vi or en. */
export function resolve_locale(preference: LocalePreference): Locale {
    if (preference !== 'system') return preference;
    const language = typeof navigator !== 'undefined' ? (navigator.language ?? '') : '';
    return language.toLowerCase().startsWith('vi') ? 'vi' : 'en';
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

/** Effective language at startup: the saved choice, with `system` resolved against navigator.language. */
export function resolve_initial_locale(): Locale {
    return resolve_locale(read_saved_preference());
}

const plural_rules_cache = new Map<Locale, Intl.PluralRules>();

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
 * `key_one` / `key_other` are chosen with Intl.PluralRules when `params.count` is a number.
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

/** Effective language ('en' | 'vi'). Read-only from outside; change it with set_locale. */
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
 * Keeps English if the dictionary cannot load, and sets <html lang>.
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
            dictionary_store.update((current) => ({ ...current, [locale]: dictionary }));
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
