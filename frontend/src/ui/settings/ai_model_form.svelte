<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeftIcon';
    import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircleIcon';
    import EyeIcon from 'phosphor-svelte/lib/EyeIcon';
    import EyeSlashIcon from 'phosphor-svelte/lib/EyeSlashIcon';
    import BrandIcon from '../ai/brand_icon.svelte';
    import WarningCircleIcon from 'phosphor-svelte/lib/WarningCircleIcon';
    import {
        add_model,
        set_default_model,
        test_connection,
        update_model,
        type ModelView,
        type TestResult,
        Provider,
    } from '../../api/ai';
    import { humanizeBackendError } from '../../modules/errors';
    import { t, t_store } from '../../i18n/i18n';
    import {
        base_url_placeholder,
        draft_from_model,
        empty_draft,
        model_suggestions,
        PROVIDER_OPTIONS,
        provider_brand_label,
        provider_requires_base_url,
        provider_takes_base_url,
        to_model_input,
        validate_draft,
        type ModelDraft,
    } from './ai_model_draft';

    interface Props {
        model: ModelView | null;
        firstModel: boolean;
        onDone: () => void | Promise<void>;
        onCancel: () => void;
    }

    let { model, firstModel, onDone, onCancel }: Props = $props();

    // The form edits a copy; the list only changes after a successful save.
    // svelte-ignore state_referenced_locally
    let draft = $state<ModelDraft>(model ? draft_from_model(model) : empty_draft({ is_default: firstModel }));
    let showKey = $state(false);
    let saving = $state(false);
    let testing = $state(false);
    let error = $state('');
    let testResult = $state<TestResult | null>(null);

    const suggestions = $derived(model_suggestions(draft.provider));
    const takesBaseUrl = $derived(provider_takes_base_url(draft.provider));
    const baseUrlRequired = $derived(provider_requires_base_url(draft.provider));
    const placeholder = $derived(base_url_placeholder(draft.provider));
    // The default cannot be unset here: another model has to be made default.
    const defaultLocked = $derived(Boolean(model?.is_default));
    const keyStored = $derived(Boolean(model?.has_api_key));

    function providerName(provider: Provider): string {
        return provider_brand_label(provider) || $t_store('settings_ai.provider_custom');
    }

    function setProvider(value: Provider): void {
        draft.provider = value;
    }

    function toggle(field: 'vision_enabled' | 'is_default'): void {
        if (field === 'is_default' && defaultLocked) return;
        draft[field] = !draft[field];
    }

    async function runTest(): Promise<void> {
        testing = true;
        testResult = null;
        try {
            testResult = await test_connection(model?.id ?? '', to_model_input(draft));
        } catch (cause) {
            testResult = { ok: false, message: humanizeBackendError(cause) };
        } finally {
            testing = false;
        }
    }

    async function save(): Promise<void> {
        if (saving) return;
        const problem = validate_draft(draft);
        if (problem) {
            error = t(`settings_ai.${problem}`);
            return;
        }
        saving = true;
        error = '';
        try {
            const input = to_model_input(draft);
            const saved = model ? await update_model(model.id, input) : await add_model(input);
            if (draft.is_default && !model?.is_default) {
                await set_default_model(saved.id);
            }
            await onDone();
        } catch (cause) {
            error = humanizeBackendError(cause) || t('settings_ai.save_failed');
        } finally {
            saving = false;
        }
    }
</script>

<section class="settings-pane ai-form" aria-labelledby="settings-ai-form-title">
    <header class="ai-form-header">
        <button type="button" class="settings-btn ghost ai-back" onclick={onCancel}>
            <ArrowLeftIcon size={16} aria-hidden="true" />
            {$t_store('settings_ai.back')}
        </button>
    </header>

    <h3 id="settings-ai-form-title" class="ai-form-title">
        {model ? $t_store('settings_ai.form_edit_title') : $t_store('settings_ai.form_add_title')}
    </h3>

    <div class="ai-form-grid">
        <label class="settings-field">
            <span class="settings-field-label">{$t_store('settings_ai.field_name')}</span>
            <input class="settings-input" type="text" autocomplete="off" bind:value={draft.display_name} />
        </label>

        <div class="settings-field">
            <span id="ai-provider-label" class="settings-field-label">{$t_store('settings_ai.field_provider')}</span>
            <div class="ai-provider-grid" role="radiogroup" aria-labelledby="ai-provider-label">
                {#each PROVIDER_OPTIONS as provider (provider)}
                    {@const checked = draft.provider === provider}
                    <button
                        type="button"
                        class="ai-provider"
                        class:is-on={checked}
                        role="radio"
                        aria-checked={checked ? 'true' : 'false'}
                        onclick={() => setProvider(provider)}
                    >
                        <BrandIcon {provider} size={20} />
                        <span class="ai-provider-name">{providerName(provider)}</span>
                    </button>
                {/each}
            </div>
        </div>

        {#if takesBaseUrl}
            <label class="settings-field">
                <span class="settings-field-label">
                    {baseUrlRequired ? $t_store('settings_ai.field_base_url') : $t_store('settings_ai.field_base_url_optional')}
                </span>
                <input class="settings-input" type="url" autocomplete="off" placeholder={placeholder} bind:value={draft.base_url} />
            </label>
        {/if}

        <div class="settings-field">
            <span class="settings-field-label">{$t_store('settings_ai.field_api_key')}</span>
            <div class="key-row">
                <input
                    class="settings-input"
                    type={showKey ? 'text' : 'password'}
                    autocomplete="off"
                    spellcheck="false"
                    disabled={draft.clear_api_key}
                    placeholder={keyStored ? '••••••••••••' : ''}
                    bind:value={draft.api_key}
                />
                <button
                    type="button"
                    class="settings-icon-btn"
                    aria-label={showKey ? $t_store('settings_ai.hide_key') : $t_store('settings_ai.show_key')}
                    aria-pressed={showKey}
                    onclick={() => (showKey = !showKey)}
                >
                    {#if showKey}
                        <EyeSlashIcon size={16} aria-hidden="true" />
                    {:else}
                        <EyeIcon size={16} aria-hidden="true" />
                    {/if}
                </button>
            </div>
            <p class="settings-note">
                {#if model}
                    {keyStored ? $t_store('settings_ai.key_saved') : $t_store('settings_ai.key_none')}
                {/if}
                {$t_store('settings_ai.key_note')}
            </p>
            {#if keyStored}
                <label class="clear-key">
                    <input type="checkbox" bind:checked={draft.clear_api_key} />
                    <span>{$t_store('settings_ai.key_clear')}</span>
                </label>
            {/if}
        </div>

        <label class="settings-field">
            <span class="settings-field-label">{$t_store('settings_ai.field_model')}</span>
            <input
                class="settings-input"
                type="text"
                autocomplete="off"
                list="settings-ai-model-suggestions"
                bind:value={draft.model}
            />
            <datalist id="settings-ai-model-suggestions">
                {#each suggestions as suggestion (suggestion)}
                    <option value={suggestion}></option>
                {/each}
            </datalist>
        </label>

        <div class="switch-stack">
            <div class="settings-switch-row">
                <span id="settings-ai-vision-label">{$t_store('settings_ai.toggle_vision')}</span>
                <button
                    type="button"
                    class="settings-switch"
                    role="switch"
                    aria-checked={draft.vision_enabled}
                    aria-labelledby="settings-ai-vision-label"
                    onclick={() => toggle('vision_enabled')}
                ></button>
            </div>
            <div class="settings-switch-row">
                <span id="settings-ai-default-label">{$t_store('settings_ai.toggle_default')}</span>
                <button
                    type="button"
                    class="settings-switch"
                    role="switch"
                    aria-checked={draft.is_default}
                    aria-labelledby="settings-ai-default-label"
                    aria-describedby={defaultLocked ? 'settings-ai-default-note' : undefined}
                    disabled={defaultLocked}
                    onclick={() => toggle('is_default')}
                ></button>
            </div>
            {#if defaultLocked}
                <p id="settings-ai-default-note" class="settings-note">{$t_store('settings_ai.default_locked')}</p>
            {/if}
        </div>
    </div>

    <div class="ai-form-footer">
        <div class="test-row">
            <button type="button" class="settings-btn" disabled={testing} onclick={() => void runTest()}>
                {testing ? $t_store('settings_ai.testing') : $t_store('settings_ai.test')}
            </button>
            {#if testResult}
                {#if testResult.ok}
                    <span class="test-result ok" role="status">
                        <CheckCircleIcon size={16} aria-hidden="true" />
                        {testResult.message || $t_store('settings_ai.test_ok')}
                    </span>
                {:else}
                    <span class="test-result fail" role="alert">
                        <WarningCircleIcon size={16} aria-hidden="true" />
                        {testResult.message || $t_store('settings_ai.test_failed')}
                    </span>
                {/if}
            {/if}
        </div>

        {#if error}
            <p class="settings-note error" role="alert">{error}</p>
        {/if}

        <div class="ai-form-actions">
            <button type="button" class="settings-btn" onclick={onCancel}>{$t_store('settings_ai.cancel')}</button>
            <button type="button" class="settings-btn primary" disabled={saving} onclick={() => void save()}>
                {saving ? $t_store('settings_ai.saving') : $t_store('settings_ai.save')}
            </button>
        </div>
    </div>
</section>

<style>
    .ai-form { min-height: 0; }
    .ai-form-header { display: flex; align-items: center; min-height: var(--control-md); padding-right: 40px; }
    .ai-back { padding: 0 8px; margin-left: -8px; }
    .ai-form-title {
        margin-top: calc(-1 * var(--space-2));
        color: var(--color-text);
        font-size: var(--type-xl);
        font-weight: var(--weight-semibold);
        letter-spacing: -0.01em;
        line-height: var(--leading-tight);
    }

    .ai-form-grid { display: flex; flex-direction: column; gap: var(--space-3); min-width: 0; }

    .key-row { display: flex; align-items: center; gap: var(--space-1); min-width: 0; }
    .key-row .settings-input { flex: 1; }

    .clear-key {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        color: var(--color-text-muted);
        font-size: var(--type-xs);
        cursor: pointer;
    }

    .switch-stack { display: flex; flex-direction: column; gap: var(--space-2); }

    .ai-form-footer {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        margin-top: auto;
        padding-top: var(--space-3);
        border-top: 1px solid var(--color-border-soft);
    }

    .test-row { display: flex; align-items: center; gap: var(--space-3); min-width: 0; }

    .test-result {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        min-width: 0;
        font-size: var(--type-sm);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    .test-result.ok { color: var(--color-success); }
    .test-result.fail { color: var(--color-danger); }

    .ai-form-actions { display: flex; justify-content: flex-end; gap: var(--space-2); }

    /* The form sits in the same fixed-height pane, so its controls keep to a
       single column and the footer stays where the eye expects it. */
    .ai-form :global(select.settings-input) { appearance: auto; }

    .ai-provider-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
        gap: var(--space-2);
    }

    .ai-provider {
        min-width: 0;
        min-height: var(--control-md);
        padding: 0 var(--space-2-5);
        display: flex;
        align-items: center;
        gap: var(--space-2);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-2);
        color: var(--color-text);
        font: inherit;
        font-size: var(--type-sm);
        text-align: left;
        cursor: pointer;
    }

    .ai-provider.is-on {
        border-color: var(--color-accent);
        box-shadow: inset 0 0 0 1px var(--color-accent);
    }

    .ai-provider:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    .ai-provider-name {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
</style>
