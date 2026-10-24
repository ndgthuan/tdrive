# i18n

TDrive ships English (`en`) and Vietnamese (`vi`) and lists 17 more languages in `SUPPORTED_LOCALES`. English is bundled eagerly and is the fallback for everything. Every other language loads on first use.

## Files

- `i18n.ts`: `SUPPORTED_LOCALES`, the store, `t`, `t_store`, `set_locale`, the system-language matching and the formatters.
- `../locales/en/<namespace>.json`: English source strings (eager). This is the reference for every other locale.
- `../locales/<code>/<namespace>.json`: one folder per locale, named by its BCP-47 code (`vi`, `zh-CN`, `pt-BR`, ...). Loaded lazily.
- `i18n.test.ts`: behavior tests and the parity check for every locale folder that exists.

## Supported locales

`SUPPORTED_LOCALES` in `i18n.ts` lists each language with `code`, `native_name` and `english_name`. `Locale` is derived from it. The list is already complete for the current target set:

`en`, `vi`, `zh-CN`, `zh-TW`, `ja`, `ko`, `es`, `pt-BR`, `fr`, `de`, `it`, `ru`, `uk`, `pl`, `nl`, `tr`, `id`, `th`, `hi`

A listed locale without a folder is fine. It shows English until its folder arrives, with no warning. A folder that exists must be complete (see the parity test below).

## Keys

Each JSON file is one namespace. Keys inside it are flat and snake_case. Reference them as `<namespace>.<key>`:

```json
// locales/en/common.json
{ "cancel": "Cancel", "items_one": "{count} item", "items_other": "{count} items" }
```

`common.cancel` and `common.items` are the keys. `items` resolves to a plural form when `count` is passed.

## Plurals

Plural forms use the CLDR categories that `Intl.PluralRules` returns for the locale:

- `key_zero`, `key_one`, `key_two`, `key_few`, `key_many`, `key_other`.
- `key_other` is required in every locale. It is the fallback when a category is missing.
- For each plural group in English (`key_other` exists there), the locale must provide every category its language uses, plus `_other`.

Categories per language family:

| Languages | Required forms |
|---|---|
| `ru`, `uk`, `pl` | `_one`, `_few`, `_many`, `_other` |
| `ja`, `zh-CN`, `zh-TW`, `ko`, `th`, `id`, `vi` | `_other` only (`_one` optional, it is only used if present) |
| `en`, `de`, `it`, `nl`, `tr`, `hi`, `es`, `pt-BR`, `fr` | `_one`, `_other` in most cases. Check the list for your code with `new Intl.PluralRules('<code>').resolvedOptions().pluralCategories` and add any category it prints. |

`fr` and `pt-BR` use the `one` form for 0 as well as 1, so the `_one` string must read correctly for zero.

Pass a number as `count` to select the form. Use `count` in the string as `{count}`.

## Rules for translators

1. Create `src/locales/<code>/` with exactly the same file names as `src/locales/en/`. Do not add or drop namespace files.
2. Translate every key in each file. Keys are the same as English. Do not rename or reorder keys.
3. Keep every `{placeholder}` exactly as in English, including its name (`{count}`, `{name}`, `{language}`). The same set of names must appear in the translation. Translate the words around them, not the names.
4. Plural groups: provide every category your language uses (table above), plus `_other`. A language with only `_other` does not need `_one`.
5. Keep brand and product names as they are in English: TDrive, Google, Apple, Microsoft, Dropbox, OneDrive, GitHub, Telegram, Wi-Fi, PDF, JPEG, MP4, and similar proper nouns.
6. Keep HTML, markdown, and emoji in the text as they appear in English. Keep `·` separators and punctuation style idiomatic for your language.
7. Partial translations are allowed. A missing key falls back to English. Prefer a complete file over a half-done one, because a folder that exists must pass the parity test.
8. Run `npm test` in `frontend/` before handing off. The parity test names every missing key, extra key, and placeholder mismatch.

## Usage in `.svelte`

```svelte
<script lang="ts">
    import { t_store, format_bytes } from '../i18n/i18n';
</script>

<button>{$t_store('common.cancel')}</button>
<span>{$t_store('common.items', { count: files.length })}</span>
<span>{format_bytes(size)}</span>
```

`$t_store` is reactive. The template re-renders when the language changes.

## Usage in plain `.ts`

```ts
import { t, format_date } from '../i18n/i18n';

const label = t('settings_language.title');        // "Language" / "Ngôn ngữ"
const message = t('common.items', { count: 3 });   // "3 items" / "3 mục"
const when = format_date(new Date(), 'medium');
```

`t()` reads the current language when called. It does not update on its own. For text that must update, call it from a `.svelte` file through `$t_store`, or subscribe to `current_locale`.

## Changing the language

```ts
import { set_locale } from '../i18n/i18n';

await set_locale('vi');       // any code from SUPPORTED_LOCALES, e.g. 'zh-CN'
await set_locale('system');   // follow navigator.languages
```

`set_locale` persists the choice under `localStorage['tdrive_locale']` (`'system'` or a locale code), loads the folder if needed, updates `current_locale`, and sets `<html lang>` to the effective code. If the folder is missing or fails to load, it keeps English (a warning is logged only for load failures). Overlapping calls resolve last-wins.

Read the user's choice (including `system`) from `locale_preference`, for example to highlight the selected option in settings. Use `locale_info(code)` for `native_name` and `english_name`.

## System language

`system` resolves `navigator.languages` in order, using `match_locale_tag`:

1. Exact match (case-insensitive): `pt-BR`, `zh-TW`.
2. Chinese: a `Hant` script, or region `TW`, `HK`, `MO`, gives `zh-TW`. Otherwise (including `Hans` or plain `zh`) it gives `zh-CN`.
3. Portuguese: any `pt-*` gives `pt-BR`.
4. Same language subtag: `es-MX` gives `es`, `en-GB` gives `en`.
5. Nothing matched: the next entry in `navigator.languages`, then English.

## Startup

`main.ts` calls `await set_locale(resolve_initial_preference())` at the start of the lifecycle. `resolve_initial_locale()` returns the effective code without changing anything.

## Formatting

- `format_date(date, style)`: `style` is `'full' | 'long' | 'medium' | 'short'`. Uses `Intl.DateTimeFormat` with the locale code.
- `format_number(n)`: locale grouping (`1,234,567` / `1.234.567`).
- `format_bytes(n)`: matches the app's existing size style (base 1024, `B/KB/MB/GB/TB`, at most one decimal, no grouping). Only the decimal separator follows the locale (`1.5 KB` / `1,5 KB`).

## Adding a string

1. Add the key to `en/<ns>.json`.
2. Add the same key, with the translated text and the required plural categories, to every locale folder that exists. Keep `{placeholders}` identical.
3. Run `npm test` in `frontend/`.
