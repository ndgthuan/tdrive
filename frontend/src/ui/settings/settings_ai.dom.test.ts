// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, tick, unmount } from 'svelte';
import {
    add_model,
    delete_model,
    list_models,
    Provider,
    set_default_model,
    test_connection,
    update_model,
    type ModelView,
} from '../../api/ai';
import AiModelsPane from './ai_models_pane.svelte';
import { set_locale } from '../../i18n/i18n';

vi.mock('../../api/ai', async (importOriginal) => {
    const actual = await importOriginal<typeof import('../../api/ai')>();
    return {
        ...actual,
        list_models: vi.fn(),
        add_model: vi.fn(),
        update_model: vi.fn(),
        delete_model: vi.fn(),
        set_default_model: vi.fn(),
        test_connection: vi.fn(),
    };
});

const openai_model: ModelView = {
    id: 'm-openai',
    display_name: 'Team OpenAI',
    provider: Provider.ProviderOpenAI,
    base_url: '',
    model: 'gpt-4.1-mini',
    vision_enabled: true,
    is_default: true,
    has_api_key: true,
};

const gemini_model: ModelView = {
    id: 'm-gemini',
    display_name: 'Backup Gemini',
    provider: Provider.ProviderGemini,
    base_url: '',
    model: 'gemini-2.5-flash',
    vision_enabled: false,
    is_default: false,
    has_api_key: false,
};

let component: Record<string, unknown> | null = null;
let host: HTMLElement | null = null;

/** Lets the pane's onMount refresh and any pending promise settle. */
async function settle(): Promise<void> {
    for (let i = 0; i < 4; i += 1) {
        await Promise.resolve();
        await tick();
    }
    flushSync();
}

function buttonByText(text: string): HTMLButtonElement {
    const found = Array.from(host?.querySelectorAll<HTMLButtonElement>('button') ?? [])
        .find((button) => button.textContent?.trim() === text);
    if (!found) throw new Error(`missing button "${text}"`);
    return found;
}

// The provider is a grid of brand buttons, not a select: pick it by its name.
function providerButton(name: string): HTMLButtonElement {
    const found = Array.from(host?.querySelectorAll<HTMLButtonElement>('.ai-provider') ?? [])
        .find((item) => item.textContent?.trim() === name);
    if (!found) throw new Error(`missing provider "${name}"`);
    return found;
}

function field(label: string): HTMLInputElement | HTMLSelectElement {
    const wrapper = Array.from(host?.querySelectorAll<HTMLElement>('.settings-field') ?? [])
        .find((item) => item.querySelector('.settings-field-label')?.textContent?.trim() === label);
    const control = wrapper?.querySelector<HTMLInputElement | HTMLSelectElement>('input, select');
    if (!control) throw new Error(`missing field "${label}"`);
    return control;
}

function typeInto(input: HTMLInputElement | HTMLSelectElement, value: string): void {
    if (input instanceof HTMLSelectElement) {
        // Select the option itself: the binding reads the checked option, and
        // setting only .value leaves that state behind in the DOM test runtime.
        const option = Array.from(input.options).find((item) => item.value === value);
        if (!option) throw new Error(`no option ${value}`);
        option.selected = true;
        input.dispatchEvent(new Event('change', { bubbles: true }));
    } else {
        input.value = value;
        input.dispatchEvent(new Event('input', { bubbles: true }));
    }
    flushSync();
}

function mountPane(): void {
    host = document.createElement('div');
    document.body.appendChild(host);
    component = mount(AiModelsPane, { target: host });
    flushSync();
}

beforeEach(async () => {
    await set_locale('en');
    vi.mocked(list_models).mockResolvedValue([openai_model, gemini_model]);
    vi.mocked(add_model).mockResolvedValue({ ...gemini_model, id: 'm-new', display_name: 'Internal', provider: Provider.ProviderCustom, model: 'my-model', base_url: 'https://llm.internal/v1', has_api_key: true });
    vi.mocked(update_model).mockImplementation(async (id, input) => ({ ...openai_model, id, display_name: input.display_name, model: input.model }));
    vi.mocked(delete_model).mockResolvedValue(undefined);
    vi.mocked(set_default_model).mockResolvedValue(undefined);
    vi.mocked(test_connection).mockResolvedValue({ ok: true, message: 'Connected in 412 ms.' });
});

afterEach(async () => {
    if (component) await unmount(component);
    host?.remove();
    component = null;
    host = null;
    vi.clearAllMocks();
});

describe('AI models pane', () => {
    it('lists each model with its name, provider, model, default badge and vision state', async () => {
        mountPane();
        await settle();

        expect(list_models).toHaveBeenCalled();
        expect(host?.textContent).not.toContain('Models for the AI assistant');
        const cards = host?.querySelectorAll('.ai-card') ?? [];
        expect(cards).toHaveLength(2);

        const first = cards[0];
        expect(first.querySelector('.ai-card-name')?.textContent).toBe('Team OpenAI');
        expect(first.querySelector('.ai-badge')?.textContent).toBe('Default');
        expect(first.querySelector('.ai-card-sub')?.textContent).toContain('gpt-4.1-mini');
        expect(first.querySelector('.ai-card-meta')?.textContent).toContain('Reads images');

        const second = cards[1];
        expect(second.querySelector('.ai-badge')).toBeNull();
        expect(second.querySelector('.ai-card-sub')?.textContent).toContain('Google Gemini');
        expect(second.querySelector('.ai-card-meta')?.textContent).toContain('Text only');
        expect(second.querySelector('.ai-card-meta')?.textContent).toContain('No API key saved');
    });

    it('shows the empty state when no model is configured', async () => {
        vi.mocked(list_models).mockResolvedValue([]);
        mountPane();
        await settle();

        expect(host?.querySelectorAll('.ai-card')).toHaveLength(0);
        expect(host?.textContent).toContain('No models yet.');
    });

    it('tests a new custom endpoint draft, then saves it and makes it the default', async () => {
        mountPane();
        await settle();
        buttonByText('Add model').click();
        flushSync();

        typeInto(field('Display name'), 'Internal');
        providerButton('Custom (OpenAI-compatible)').click();
        flushSync();
        typeInto(field('Base URL'), 'https://llm.internal/v1');
        typeInto(field('API key'), 'sk-test-value');
        typeInto(field('Model'), 'my-model');
        const defaultSwitch = host?.querySelector<HTMLButtonElement>('[aria-labelledby="settings-ai-default-label"]');
        defaultSwitch?.click();
        flushSync();

        buttonByText('Test connection').click();
        await settle();
        expect(test_connection).toHaveBeenCalledWith('', expect.objectContaining({
            display_name: 'Internal',
            provider: Provider.ProviderCustom,
            base_url: 'https://llm.internal/v1',
            model: 'my-model',
            api_key: 'sk-test-value',
            clear_api_key: false,
        }));
        expect(host?.textContent).toContain('Connected in 412 ms.');

        buttonByText('Save').click();
        await settle();
        expect(add_model).toHaveBeenCalledTimes(1);
        expect(set_default_model).toHaveBeenCalledWith('m-new');
    });

    it('reports a failed connection test inline without throwing', async () => {
        vi.mocked(test_connection).mockResolvedValue({ ok: false, message: 'Invalid API key.' });
        mountPane();
        await settle();
        buttonByText('Edit').click();
        flushSync();

        buttonByText('Test connection').click();
        await settle();
        const alert = host?.querySelector('.test-result.fail');
        expect(alert?.textContent).toContain('Invalid API key.');
    });

    it('edits a stored model: an empty key keeps the saved one and the default cannot be unset', async () => {
        mountPane();
        await settle();
        buttonByText('Edit').click();
        flushSync();

        expect(host?.textContent).toContain('Key saved. Leave empty to keep it.');
        expect(field('API key')).toHaveProperty('value', '');
        const defaultSwitch = host?.querySelector<HTMLButtonElement>('[aria-labelledby="settings-ai-default-label"]');
        expect(defaultSwitch?.disabled).toBe(true);

        typeInto(field('Model'), 'gpt-4.1');
        buttonByText('Save').click();
        await settle();

        expect(update_model).toHaveBeenCalledWith('m-openai', expect.objectContaining({
            model: 'gpt-4.1',
            api_key: '',
            clear_api_key: false,
        }));
        expect(set_default_model).not.toHaveBeenCalled();
    });

    it('asks before deleting a model and deletes it only after confirming', async () => {
        mountPane();
        await settle();

        const more = host?.querySelectorAll<HTMLButtonElement>('[aria-haspopup="menu"]')[1];
        more?.click();
        flushSync();
        Array.from(host?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])
            .find((item) => item.textContent?.trim() === 'Delete')?.click();
        flushSync();

        expect(delete_model).not.toHaveBeenCalled();
        expect(host?.textContent).toContain('Delete Backup Gemini? Its saved API key is removed too.');

        buttonByText('Delete model').click();
        await settle();
        expect(delete_model).toHaveBeenCalledWith('m-gemini');
    });

    it('sets a non-default model as the default from its more menu', async () => {
        mountPane();
        await settle();

        const more = host?.querySelectorAll<HTMLButtonElement>('[aria-haspopup="menu"]')[1];
        more?.click();
        flushSync();
        Array.from(host?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])
            .find((item) => item.textContent?.trim() === 'Set as default')?.click();
        await settle();

        expect(set_default_model).toHaveBeenCalledWith('m-gemini');
    });
});
