// Copyright (c) 2026 Nguyễn Dương Gia Thuận

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import { get } from 'svelte/store';
import { Provider, type ModelView } from '../../api/ai';
import { state } from '../../state';
import { sidebarState } from '../sidebar/sidebar_store';
import { breadcrumbPath } from '../chrome/breadcrumb_store';
import { configured_models, chat_model_choices, selected_model_id } from './ai_models_store';
import { active_session_id } from './ai_store';
import { scope_override, build_scope } from './ai_scope';
import { OPEN_SETTINGS_EVENT } from '../settings/settings_store';
import AiComposer from './ai_composer.svelte';
import AiModelPicker from './ai_model_picker.svelte';
import { set_locale } from '../../i18n/i18n';

vi.mock('../../modules/file_list', () => ({ refreshFiles: vi.fn() }));
vi.mock('../../modules/notifications', () => ({ notify: vi.fn(() => 'toast') }));
vi.mock('./ai_chat', () => ({ send_text: vi.fn(async () => {}), refresh_models: vi.fn(async () => {}) }));

const personal = { id: 1, title: 'TDrive', kind: 'personal' as const, isActive: true, inviteLink: '' };
const temp = { id: 77, title: 'temp', kind: 'shared' as const, isActive: false, inviteLink: '' };
const other = { id: 78, title: 'Read only drive', kind: 'shared' as const, isActive: false, inviteLink: '' };

function model(id: string, overrides: Partial<ModelView> = {}): ModelView {
    return {
        id,
        display_name: `Model ${id}`,
        provider: Provider.ProviderGemini,
        base_url: '',
        model: `${id}-model`,
        vision_enabled: false,
        is_default: false,
        has_api_key: true,
        ...overrides,
    };
}

let host: HTMLElement | null = null;
let component: ReturnType<typeof mount> | null = null;

// Mounts a component into a fresh host. Typed loosely because the two components have different props.
function render(target: unknown): void {
    host = document.createElement('div');
    document.body.appendChild(host);
    component = mount(target as never, { target: host });
    flushSync();
}

function click(selector: string): void {
    const element = host?.querySelector<HTMLElement>(selector);
    if (!element) throw new Error(`missing ${selector}`);
    element.click();
    flushSync();
}

function menuItems(): HTMLElement[] {
    return Array.from(host?.querySelectorAll<HTMLElement>('[role="menuitemradio"], [role="menuitem"]') ?? []);
}

beforeEach(async () => {
    await set_locale('en');
    state.activeChannel = { id: 1, title: 'TDrive', kind: 'personal' };
    state.currentFolderId = '';
    breadcrumbPath.set([]);
    sidebarState.set({ personal: [personal], shared: [temp, other], pending: [], activeChannelId: 1, virtualView: null });
    scope_override.set(null);
    active_session_id.set(null);
    chat_model_choices.set({});
    configured_models.set([
        model('a', { display_name: 'Gemini Flash', is_default: true, vision_enabled: true }),
        model('b', { display_name: 'Gemini Pro' }),
    ]);
});

afterEach(async () => {
    if (component) await unmount(component);
    host?.remove();
    component = null;
    host = null;
    configured_models.set([]);
    scope_override.set(null);
});

describe('composer model picker', () => {
    it('preselects the default model and lists every configured model', () => {
        render(AiModelPicker);
        expect(host?.querySelector('.ai-model-btn')?.textContent).toContain('Gemini Flash');

        click('.ai-model-btn');
        const names = menuItems().map((item) => item.textContent?.trim() ?? '');
        expect(names[0]).toContain('Auto');
        expect(names.some((text) => text.includes('Gemini Pro'))).toBe(true);
        expect(host?.querySelectorAll('.ai-model-badge')).toHaveLength(1);
    });

    it('marks vision-capable models', () => {
        render(AiModelPicker);
        click('.ai-model-btn');
        expect(host?.querySelectorAll('.ai-model-vision')).toHaveLength(1);
    });

    it('chooses one model for the open chat and closes the menu', () => {
        render(AiModelPicker);
        click('.ai-model-btn');
        menuItems().find((item) => item.textContent?.includes('Gemini Pro'))?.click();
        flushSync();

        expect(get(selected_model_id)).toBe('b');
        expect(host?.querySelector('.ai-model-menu')).toBeNull();
        expect(host?.querySelector('.ai-model-btn')?.textContent).toContain('Gemini Pro');
    });

    it('keeps the choice to the chat it was made in', () => {
        render(AiModelPicker);
        click('.ai-model-btn');
        menuItems().find((item) => item.textContent?.includes('Gemini Pro'))?.click();
        active_session_id.set('other-chat');
        flushSync();
        expect(get(selected_model_id)).toBe('');
    });

    it('returns to Auto, which is the default model', () => {
        render(AiModelPicker);
        click('.ai-model-btn');
        menuItems().find((item) => item.textContent?.includes('Gemini Pro'))?.click();
        click('.ai-model-btn');
        menuItems()[0]?.click();
        flushSync();
        expect(get(selected_model_id)).toBe('');
    });

    it('ends with Manage models, which opens Settings on the AI pane', () => {
        const opened = vi.fn();
        window.addEventListener(OPEN_SETTINGS_EVENT, opened);
        try {
            render(AiModelPicker);
            click('.ai-model-btn');
            click('.ai-model-manage');
            expect(opened).toHaveBeenCalledOnce();
            expect((opened.mock.calls[0][0] as CustomEvent).detail).toEqual({ section: 'ai' });
        } finally {
            window.removeEventListener(OPEN_SETTINGS_EVENT, opened);
        }
    });

    it('shows the add-model call to action only when no model exists', () => {
        configured_models.set([]);
        render(AiModelPicker);
        expect(host?.querySelector('.ai-model-btn')).toBeNull();
        expect(host?.querySelector('.ai-cta')?.textContent).toContain('Add a model in Settings');
    });
});

describe('composer context chip', () => {
    it('shows the open shared drive by name and follows a drive switch', () => {
        render(AiComposer);
        expect(host?.querySelector('.ai-scope .ai-chip')?.textContent).toContain('My Drive');

        state.activeChannel = { id: 77, title: 'temp', kind: 'shared' };
        sidebarState.update((current) => ({ ...current, activeChannelId: 77 }));
        flushSync();
        expect(host?.querySelector('.ai-scope .ai-chip')?.textContent).toContain('temp');
        expect(build_scope()).toEqual({ channel_id: 77, folder_id: '' });
    });

    it('lists the drives the user can change, plus the drive root choice', () => {
        render(AiComposer);
        click('.ai-scope .ai-chip');
        const names = menuItems().map((item) => item.textContent?.trim() ?? '');
        expect(names.some((text) => text.includes('Drive root'))).toBe(true);
        expect(names.some((text) => text.includes('temp'))).toBe(true);
        expect(names.some((text) => text.includes('Read only drive'))).toBe(true);
    });

    it('sends the chosen shared drive as the scope', () => {
        render(AiComposer);
        click('.ai-scope .ai-chip');
        menuItems().find((item) => item.textContent?.includes('temp'))?.click();
        flushSync();
        expect(build_scope()).toEqual({ channel_id: 77, folder_id: '' });
        expect(host?.querySelector('.ai-scope .ai-chip')?.textContent).toContain('temp');
    });

    it('offers the current folder for the open drive and returns to the drive root', () => {
        state.currentFolderId = 'folder-9';
        breadcrumbPath.set([{ id: 'folder-9', name: 'Trips' }]);
        render(AiComposer);
        click('.ai-scope .ai-chip');
        expect(menuItems().some((item) => item.textContent?.includes('Trips'))).toBe(true);
        menuItems().find((item) => item.textContent?.includes('Drive root'))?.click();
        flushSync();
        expect(build_scope()).toEqual({ channel_id: 1, folder_id: '' });
    });
});
