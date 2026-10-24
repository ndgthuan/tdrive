<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import {
        current_locale,
        locale_info,
        locale_preference,
        set_locale,
        SUPPORTED_LOCALES,
        t_store,
        type LocalePreference,
    } from '../../i18n/i18n';

    // "System" first, then every supported language in the order of SUPPORTED_LOCALES.
    const options: readonly LocalePreference[] = ['system', ...SUPPORTED_LOCALES.map((info) => info.code)];

    // The resolved language, named in its own words; it is what "System" means right now.
    const resolvedName = $derived(locale_info($current_locale).native_name);
    const systemLabel = $derived($t_store('settings_language.option_system', { language: resolvedName }));

    // Re-derived with the locale so the example follows a language switch.
    const example = $derived.by(() => {
        const now = new Date();
        const day = new Intl.DateTimeFormat($current_locale, { day: '2-digit', month: '2-digit', year: 'numeric' }).format(now);
        const time = now.toLocaleTimeString($current_locale, { hour: '2-digit', minute: '2-digit' });
        return `${day} · ${time}`;
    });

    function optionLabel(option: LocalePreference): string {
        if (option === 'system') return systemLabel;
        return locale_info(option).native_name;
    }

    // English name under the native one; omitted when they are identical (English itself) or for System.
    function optionEnglishName(option: LocalePreference): string | null {
        if (option === 'system') return null;
        const info = locale_info(option);
        return info.english_name === info.native_name ? null : info.english_name;
    }

    function choose(option: LocalePreference): void {
        void set_locale(option);
    }
</script>

<section class="settings-pane" aria-labelledby="settings-language-title">
    <header class="settings-pane-header">
        <h3 id="settings-language-title" class="settings-pane-title">{$t_store('settings_language.title')}</h3>
    </header>

    <div class="settings-section">
        <div class="settings-label" id="settings-language-group">{$t_store('settings_language.interface_label')}</div>
        <div class="language-options" role="radiogroup" aria-labelledby="settings-language-group">
            {#each options as option (option)}
                <label class="language-option" class:on={$locale_preference === option}>
                    <input
                        type="radio"
                        name="settings-locale"
                        value={option}
                        checked={$locale_preference === option}
                        onchange={() => choose(option)}
                    />
                    <span class="language-names">
                        <span class="language-native">{optionLabel(option)}</span>
                        {#if optionEnglishName(option)}
                            <span class="language-english">{optionEnglishName(option)}</span>
                        {/if}
                    </span>
                </label>
            {/each}
        </div>
        <p class="settings-note">{$t_store('settings_language.note')}</p>
    </div>

    <div class="settings-section">
        <div class="settings-label">{$t_store('settings_language.date_format_label')}</div>
        <div class="date-example" aria-describedby="settings-date-note">{example}</div>
        <p id="settings-date-note" class="settings-note">{$t_store('settings_language.date_format_note')}</p>
    </div>
</section>

<style>
    .language-options {
        display: flex;
        flex-direction: column;
        max-height: min(360px, 50vh);
        overflow-y: auto;
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-1);
    }

    .language-option {
        display: flex;
        align-items: center;
        gap: 10px;
        min-height: 40px;
        padding: 6px var(--space-4);
        flex-shrink: 0;
        color: var(--color-text-muted);
        font-size: var(--type-base);
        white-space: nowrap;
        cursor: pointer;
    }

    .language-option + .language-option { border-top: 1px solid var(--color-border-soft); }
    .language-option.on { color: var(--color-text); }

    .language-names {
        display: flex;
        flex-direction: column;
        min-width: 0;
        line-height: 1.25;
    }

    .language-english {
        color: var(--color-text-muted);
        font-size: 12px;
    }
    .language-option:hover { background: var(--overlay-white-1); }
    .language-option:focus-within { box-shadow: inset 0 0 0 1px var(--color-accent); }

    .language-option input {
        width: 16px;
        height: 16px;
        margin: 0;
        accent-color: var(--color-accent);
    }

    .date-example {
        display: flex;
        align-items: center;
        min-height: var(--control-md);
        padding: 0 10px;
        color: var(--color-text);
        background: var(--color-canvas);
        border: 1px solid var(--color-border);
        border-radius: var(--radius-sm);
        font-size: var(--type-base);
        white-space: nowrap;
    }
</style>
