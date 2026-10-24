# i18n

TDrive ships English (`en`) and Vietnamese (`vi`). English is bundled eagerly and is the fallback for everything. Vietnamese loads on first use.

## Files

- `i18n.ts`: the store, `t`, `t_store`, `set_locale`, and the formatters.
- `../locales/en/<namespace>.json`: English source strings (eager).
- `../locales/vi/<namespace>.json`: Vietnamese strings (lazy chunk).
- `i18n.test.ts`: behavior tests and the en/vi key and placeholder parity check.

## Keys

Each JSON file is one namespace. Keys inside it are flat and snake_case. Reference them as `<namespace>.<key>`:

```json
// locales/en/common.json
{ "cancel": "Cancel", "items_one": "{count} item", "items_other": "{count} items" }
```

`common.cancel` and `common.items` are the keys. `items` resolves to `items_one` or `items_other` when `count` is passed.

Rules:
- Every `en/<ns>.json` needs a `vi/<ns>.json` with exactly the same keys. The test fails otherwise.
- `{name}` placeholders must match between languages (same set of names).
- Plural forms come in pairs (`_one` and `_other`) in both languages, even where Vietnamese reads the same.
- Adding a namespace means adding the file in both folders. Translators can each own a namespace without merge conflicts.

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

await set_locale('vi');       // or 'en'
await set_locale('system');   // follow navigator.language (vi* -> vi, anything else -> en)
```

`set_locale` persists the choice under `localStorage['tdrive_locale']` (`'system' | 'en' | 'vi'`), loads the dictionary if needed, updates `current_locale`, and sets `<html lang>`. If the Vietnamese chunk fails to load, it keeps English and logs a warning. Overlapping calls resolve last-wins.

Read the user's choice (including `system`) from `locale_preference`, for example to highlight the selected option in settings.

## Startup

`main.ts` calls `await set_locale(resolve_initial_preference())` at the start of the lifecycle. `resolve_initial_locale()` returns the effective language (`'en' | 'vi'`) without changing anything.

## Formatting

- `format_date(date, style)`: `style` is `'full' | 'long' | 'medium' | 'short'`. Uses `Intl.DateTimeFormat`.
- `format_number(n)`: locale grouping (`1,234,567` / `1.234.567`).
- `format_bytes(n)`: matches the app's existing size style (base 1024, `B/KB/MB/GB/TB`, at most one decimal, no grouping). Only the decimal separator follows the locale (`1.5 KB` / `1,5 KB`).

## Adding a string

1. Add the key to `en/<ns>.json`.
2. Add the same key with the Vietnamese text to `vi/<ns>.json`. Keep `{placeholders}` identical.
3. Run `npm test` in `frontend/`.
