import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { Provider, type ModelView } from '../../api/ai';
import {
    adopt_new_chat_choice,
    chat_model_choices,
    choose_model,
    configured_models,
    default_model,
    refresh_configured_models,
    reset_new_chat_choice,
    resolve_model,
    selected_model_id,
} from './ai_models_store';
import { active_session_id } from './ai_store';

vi.mock('../../api/ai', async (importOriginal) => {
    const actual = await importOriginal<typeof import('../../api/ai')>();
    return { ...actual, list_models: vi.fn() };
});

import { list_models } from '../../api/ai';

function model(id: string, overrides: Partial<ModelView> = {}): ModelView {
    return {
        id,
        display_name: `Model ${id}`,
        provider: Provider.ProviderOpenAI,
        base_url: '',
        model: `${id}-v1`,
        vision_enabled: false,
        is_default: false,
        has_api_key: true,
        ...overrides,
    };
}

beforeEach(() => {
    vi.mocked(list_models).mockReset();
    configured_models.set([]);
    chat_model_choices.set({});
    active_session_id.set(null);
});

describe('shared model list', () => {
    it('loads the configured models into the shared store', async () => {
        vi.mocked(list_models).mockResolvedValue([model('a'), model('b', { is_default: true })]);
        await refresh_configured_models();
        expect(get(configured_models).map((item) => item.id)).toEqual(['a', 'b']);
    });

    it('drops a choice whose model was deleted, and keeps the others', async () => {
        chat_model_choices.set({ '': 'gone', s1: 'a', s2: '' });
        vi.mocked(list_models).mockResolvedValue([model('a')]);
        await refresh_configured_models();
        expect(get(chat_model_choices)).toEqual({ s1: 'a', s2: '' });
    });

    it('leaves the store unchanged when the reload fails', async () => {
        configured_models.set([model('a')]);
        vi.mocked(list_models).mockRejectedValue(new Error('store locked'));
        await expect(refresh_configured_models()).rejects.toThrow('store locked');
        expect(get(configured_models).map((item) => item.id)).toEqual(['a']);
    });
});

describe('model resolution', () => {
    it('treats Auto as the default model, falling back to the first model', () => {
        const models = [model('a'), model('b', { is_default: true })];
        expect(default_model(models)?.id).toBe('b');
        expect(resolve_model(models, '')?.id).toBe('b');
        expect(default_model([model('a'), model('c')])?.id).toBe('a');
    });

    it('returns the chosen model when it exists, and the default otherwise', () => {
        const models = [model('a'), model('b', { is_default: true })];
        expect(resolve_model(models, 'a')?.id).toBe('a');
        expect(resolve_model(models, 'missing')?.id).toBe('b');
        expect(resolve_model([], 'a')).toBeNull();
    });
});

describe('per-chat model choice', () => {
    it('keeps a separate model for each chat session', () => {
        active_session_id.set('s1');
        choose_model('a');
        active_session_id.set('s2');
        expect(get(selected_model_id)).toBe('');
        choose_model('b');
        active_session_id.set('s1');
        expect(get(selected_model_id)).toBe('a');
        active_session_id.set('s2');
        expect(get(selected_model_id)).toBe('b');
    });

    it('lets a new chat pick a model before its session exists, then keeps it on that session', () => {
        active_session_id.set(null);
        choose_model('b');
        expect(get(selected_model_id)).toBe('b');

        adopt_new_chat_choice('s9');
        active_session_id.set('s9');
        expect(get(selected_model_id)).toBe('b');
        expect(get(chat_model_choices)['']).toBeUndefined();
    });

    it('starts a fresh new chat on Auto', () => {
        active_session_id.set(null);
        choose_model('a');
        reset_new_chat_choice();
        expect(get(selected_model_id)).toBe('');
    });
});
