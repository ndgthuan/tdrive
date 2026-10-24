// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { describe, expect, it } from 'vitest';
import { Provider, type ModelView } from '../../../bindings/TDrive/backend/ai/models';
import {
    base_url_placeholder,
    draft_from_model,
    empty_draft,
    model_suggestions,
    provider_requires_base_url,
    provider_takes_base_url,
    to_model_input,
    validate_draft,
} from './ai_model_draft';

const stored: ModelView = {
    id: 'm1',
    display_name: 'Team OpenAI',
    provider: Provider.ProviderOpenAI,
    base_url: 'https://ignored.example/v1',
    model: 'gpt-4.1-mini',
    vision_enabled: true,
    is_default: true,
    has_api_key: true,
};

describe('provider rules', () => {
    it('shows a base URL only for custom and OpenRouter, and requires it only for custom', () => {
        expect(provider_takes_base_url(Provider.ProviderOpenAI)).toBe(false);
        expect(provider_takes_base_url(Provider.ProviderGemini)).toBe(false);
        expect(provider_takes_base_url(Provider.ProviderOpenRouter)).toBe(true);
        expect(provider_takes_base_url(Provider.ProviderCustom)).toBe(true);
        expect(provider_requires_base_url(Provider.ProviderOpenRouter)).toBe(false);
        expect(provider_requires_base_url(Provider.ProviderCustom)).toBe(true);
    });

    it('offers suggestions and a base URL placeholder per provider', () => {
        expect(model_suggestions(Provider.ProviderOpenAI)).toContain('gpt-4.1-mini');
        expect(model_suggestions(Provider.ProviderAnthropic)).toContain('claude-sonnet-4-5');
        expect(model_suggestions(Provider.ProviderGemini)).toContain('gemini-2.5-flash');
        expect(model_suggestions(Provider.ProviderCustom)).toEqual([]);
        expect(base_url_placeholder(Provider.ProviderCustom)).toBe('https://api.example.com/v1');
    });
});

describe('validate_draft', () => {
    it('names the first missing field', () => {
        const draft = empty_draft({ is_default: false });
        expect(validate_draft(draft)).toBe('err_name');

        draft.display_name = 'Internal';
        expect(validate_draft(draft)).toBe('err_model');

        draft.model = 'my-model';
        draft.provider = Provider.ProviderCustom;
        expect(validate_draft(draft)).toBe('err_base_url');

        draft.base_url = 'https://llm.internal/v1';
        expect(validate_draft(draft)).toBeNull();
    });
});

describe('to_model_input', () => {
    it('trims text and drops a base URL the provider does not use', () => {
        const draft = {
            ...empty_draft({ is_default: false }),
            display_name: '  Team  ',
            model: ' gpt-4.1-mini ',
            base_url: 'https://not-used.example',
            api_key: ' sk-live ',
        };
        expect(to_model_input(draft)).toEqual({
            display_name: 'Team',
            provider: Provider.ProviderOpenAI,
            base_url: '',
            model: 'gpt-4.1-mini',
            vision_enabled: true,
            api_key: 'sk-live',
            clear_api_key: false,
        });
    });

    it('sends no key when the user asks to remove the saved one', () => {
        const draft = { ...draft_from_model(stored), api_key: 'typed-by-mistake', clear_api_key: true };
        expect(to_model_input(draft)).toMatchObject({ api_key: '', clear_api_key: true });
    });

    it('keeps the stored key when no new one is typed', () => {
        const draft = draft_from_model(stored);
        expect(to_model_input(draft)).toMatchObject({ api_key: '', clear_api_key: false });
    });

    it('keeps a base URL for OpenRouter', () => {
        const draft = { ...draft_from_model(stored), provider: Provider.ProviderOpenRouter, base_url: ' https://openrouter.ai/api/v1 ' };
        expect(to_model_input(draft).base_url).toBe('https://openrouter.ai/api/v1');
    });
});

describe('draft_from_model', () => {
    it('starts an edit with an empty key field and the stored default flag', () => {
        const draft = draft_from_model(stored);
        expect(draft).toMatchObject({ display_name: 'Team OpenAI', model: 'gpt-4.1-mini', is_default: true, api_key: '', clear_api_key: false });
    });
});
