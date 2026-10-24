// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Pure helpers for the add/edit model form. No DOM, no backend calls.
import { Provider, type ModelInput, type ModelView } from '../../../bindings/TDrive/backend/ai/models';

export interface ModelDraft {
    display_name: string;
    provider: Provider;
    base_url: string;
    model: string;
    vision_enabled: boolean;
    is_default: boolean;
    api_key: string;
    clear_api_key: boolean;
}

export const PROVIDER_OPTIONS: readonly Provider[] = [
    Provider.ProviderOpenAI,
    Provider.ProviderAnthropic,
    Provider.ProviderGemini,
    Provider.ProviderOpenRouter,
    Provider.ProviderCustom,
];

const BRAND_LABELS: Partial<Record<Provider, string>> = {
    [Provider.ProviderOpenAI]: 'OpenAI',
    [Provider.ProviderAnthropic]: 'Anthropic',
    [Provider.ProviderGemini]: 'Google Gemini',
    [Provider.ProviderOpenRouter]: 'OpenRouter',
};

// A few starting points per provider, offered as suggestions only.
const MODEL_SUGGESTIONS: Partial<Record<Provider, readonly string[]>> = {
    [Provider.ProviderOpenAI]: ['gpt-4.1-mini', 'gpt-4.1'],
    [Provider.ProviderAnthropic]: ['claude-sonnet-4-5', 'claude-haiku-4-5'],
    [Provider.ProviderGemini]: ['gemini-2.5-flash', 'gemini-2.5-pro'],
    [Provider.ProviderOpenRouter]: ['openai/gpt-4.1-mini', 'anthropic/claude-sonnet-4.5'],
};

const BASE_URL_PLACEHOLDERS: Partial<Record<Provider, string>> = {
    [Provider.ProviderOpenRouter]: 'https://openrouter.ai/api/v1',
    [Provider.ProviderCustom]: 'https://api.example.com/v1',
};

/** Brand names stay as they are; only the custom entry is translated by the caller. */
export function provider_brand_label(provider: Provider): string {
    return BRAND_LABELS[provider] ?? '';
}

/** Base URL is shown for the two providers where it is a real choice. */
export function provider_takes_base_url(provider: Provider): boolean {
    return provider === Provider.ProviderCustom || provider === Provider.ProviderOpenRouter;
}

/** A custom endpoint cannot work without its own base URL. */
export function provider_requires_base_url(provider: Provider): boolean {
    return provider === Provider.ProviderCustom;
}

export function model_suggestions(provider: Provider): readonly string[] {
    return MODEL_SUGGESTIONS[provider] ?? [];
}

export function base_url_placeholder(provider: Provider): string {
    return BASE_URL_PLACEHOLDERS[provider] ?? '';
}

export function empty_draft(defaults: { is_default: boolean }): ModelDraft {
    return {
        display_name: '',
        provider: Provider.ProviderOpenAI,
        base_url: '',
        model: '',
        vision_enabled: true,
        is_default: defaults.is_default,
        api_key: '',
        clear_api_key: false,
    };
}

export function draft_from_model(model: ModelView): ModelDraft {
    return {
        display_name: model.display_name,
        provider: model.provider,
        base_url: model.base_url,
        model: model.model,
        vision_enabled: model.vision_enabled,
        is_default: model.is_default,
        api_key: '',
        clear_api_key: false,
    };
}

/** Error keys (under settings_ai.) for the first problem with the draft, or null. */
export function validate_draft(draft: ModelDraft): 'err_name' | 'err_model' | 'err_base_url' | null {
    if (!draft.display_name.trim()) return 'err_name';
    if (!draft.model.trim()) return 'err_model';
    if (provider_requires_base_url(draft.provider) && !draft.base_url.trim()) return 'err_base_url';
    return null;
}

/** The wire payload. Fields the provider does not use are sent empty. */
export function to_model_input(draft: ModelDraft): ModelInput {
    return {
        display_name: draft.display_name.trim(),
        provider: draft.provider,
        base_url: provider_takes_base_url(draft.provider) ? draft.base_url.trim() : '',
        model: draft.model.trim(),
        vision_enabled: draft.vision_enabled,
        api_key: draft.clear_api_key ? '' : draft.api_key.trim(),
        clear_api_key: draft.clear_api_key,
    };
}
