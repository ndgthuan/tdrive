import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import * as aiservice from '../../../bindings/TDrive/internal/app/aiservice';
import { Provider } from '../../../bindings/TDrive/backend/ai/models';
import AiPanel from './ai_panel.svelte';
import AiPanelToggle from './ai_panel_toggle.svelte';
import { active_session_id, mode, organize_preview, panel_open, sessions, transcript, view, busy } from './ai_store';
import { notify } from '../../modules/notifications';
import { refreshFiles } from '../../modules/file_list';

vi.mock('../../../bindings/TDrive/internal/app/aiservice', () => ({
    ListModels: vi.fn(),
    ListSessions: vi.fn(),
    GetSession: vi.fn(),
    CreateSession: vi.fn(),
    DeleteSession: vi.fn(),
    SendMessage: vi.fn(),
    PlanOrganize: vi.fn(),
    ApplyPlan: vi.fn(),
    UndoApply: vi.fn(),
    ResolveAction: vi.fn(),
    UndoAction: vi.fn(),
}));

vi.mock('@wailsio/runtime', () => ({
    Browser: {},
    System: {},
    Window: {},
    Events: { On: vi.fn(() => () => {}) },
}));

vi.mock('../../modules/file_list', () => ({ refreshFiles: vi.fn() }));
vi.mock('../../modules/notifications', () => ({ notify: vi.fn(() => 'toast') }));

const mocked = vi.mocked(aiservice);
const notify_mock = vi.mocked(notify);
const refresh_mock = vi.mocked(refreshFiles);

const model = {
    id: 'm1',
    display_name: 'GPT Test',
    provider: Provider.ProviderOpenAI,
    base_url: '',
    model: 'gpt-test',
    vision_enabled: true,
    is_default: true,
    has_api_key: true,
};

const plan = {
    plan_id: 'plan-1',
    scope: { channel_id: 0, folder_id: '' },
    total: 3,
    skipped: 0,
    notes: null,
    folders: [
        {
            name: 'Goku',
            is_new: true,
            files: [
                { file_id: 11, name: 'goku_01.png', from_path: 'Photos/goku_01.png', to_path: 'Goku/goku_01.png' },
                { file_id: 12, name: 'goku_02.png', from_path: 'Photos/goku_02.png', to_path: 'Goku/goku_02.png' },
            ],
        },
        {
            name: 'Frieren',
            is_new: false,
            files: [{ file_id: 21, name: 'frieren_01.jpeg', from_path: 'Photos/frieren_01.jpeg', to_path: 'Frieren/frieren_01.jpeg' }],
        },
    ],
};

let host: HTMLElement | null = null;
let components: Array<Record<string, unknown>> = [];

function render_panel(): void {
    host = document.createElement('div');
    document.body.appendChild(host);
    const toggle_target = document.createElement('div');
    host.appendChild(toggle_target);
    const panel_target = document.createElement('div');
    host.appendChild(panel_target);
    components = [
        mount(AiPanelToggle, { target: toggle_target }),
        mount(AiPanel, { target: panel_target }),
    ];
    flushSync();
}

function reset_stores(): void {
    panel_open.set(false);
    view.set('list');
    sessions.set([]);
    transcript.set([]);
    active_session_id.set(null);
    organize_preview.set(null);
    busy.set(false);
    mode.set('agent');
}

async function type_and_send(text: string): Promise<void> {
    const input = document.querySelector<HTMLTextAreaElement>('.ai-input');
    if (!input) throw new Error('composer missing');
    input.value = text;
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }));
    flushSync();
    await Promise.resolve();
}

function button_with_text(root: ParentNode, text: string): HTMLButtonElement {
    const found = Array.from(root.querySelectorAll('button')).find((b) => b.textContent?.trim() === text);
    if (!found) throw new Error(`button "${text}" not found`);
    return found;
}

beforeEach(() => {
    vi.clearAllMocks();
    mocked.ListModels.mockResolvedValue([model]);
    mocked.ListSessions.mockResolvedValue([]);
    reset_stores();
});

afterEach(async () => {
    for (const component of components) await unmount(component);
    components = [];
    host?.remove();
    host = null;
});

describe('AI panel', () => {
    it('toggles from the top-bar button and from Ctrl+I', async () => {
        render_panel();
        const toggle = document.querySelector<HTMLButtonElement>('.ai-toggle');
        expect(toggle?.getAttribute('aria-label')).toBe('AI assistant');
        expect(document.getElementById('ai-panel')).toBeNull();

        toggle!.click();
        flushSync();
        expect(document.getElementById('ai-panel')).not.toBeNull();
        expect(toggle!.getAttribute('aria-pressed')).toBe('true');

        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'i', ctrlKey: true }));
        flushSync();
        expect(document.getElementById('ai-panel')).toBeNull();

        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'i', ctrlKey: true }));
        flushSync();
        expect(document.getElementById('ai-panel')).not.toBeNull();
    });

    it('sends a message and renders the markdown reply', async () => {
        mocked.ListModels.mockResolvedValue([]);
        mocked.CreateSession.mockResolvedValue({ id: 's1', title: 'Hi', scope: { channel_id: 0, folder_id: '' }, created_at: 1, updated_at: 1, messages: [] });
        mocked.SendMessage.mockResolvedValue({
            id: 'r1',
            role: 'assistant',
            text: 'Hello **there**\n\n- first\n- second',
            created_at: 2,
        });
        panel_open.set(true);
        view.set('chat');
        render_panel();

        await type_and_send('Hi');
        await vi.waitFor(() => {
            const reply = document.querySelector('.ai-reply');
            expect(reply?.querySelector('strong')?.textContent).toBe('there');
            expect(reply?.querySelectorAll('li')).toHaveLength(2);
        });
        expect(mocked.CreateSession).toHaveBeenCalledWith('Hi', { channel_id: 0, folder_id: '' });
        expect(mocked.SendMessage).toHaveBeenCalledWith(expect.objectContaining({ session_id: 's1', text: 'Hi', model_id: '' }));
        expect(document.querySelector('.ai-bubble')?.textContent).toBe('Hi');
    });

    it('shows an inline error with retry, and retry sends the text again', async () => {
        mocked.ListModels.mockResolvedValue([]);
        mocked.CreateSession.mockResolvedValue({ id: 's1', title: 'Hi', scope: { channel_id: 0, folder_id: '' }, created_at: 1, updated_at: 1, messages: [] });
        mocked.SendMessage
            .mockRejectedValueOnce(new Error('rate limit exceeded (429)'))
            .mockResolvedValueOnce({ id: 'r2', role: 'assistant', text: 'Back again', created_at: 3 });
        panel_open.set(true);
        view.set('chat');
        render_panel();

        await type_and_send('Hi');
        await vi.waitFor(() => expect(document.querySelector('.ai-error')).not.toBeNull());
        expect(document.querySelector('.ai-error')?.textContent).toContain('busy or rate limited');

        button_with_text(document.querySelector('.ai-error')!, 'Retry').click();
        flushSync();
        await vi.waitFor(() => expect(document.querySelector('.ai-reply')?.textContent).toContain('Back again'));
        expect(document.querySelector('.ai-error')).toBeNull();
        expect(mocked.SendMessage).toHaveBeenCalledTimes(2);
    });

    it('shows the add-model call to action when no model exists', async () => {
        mocked.ListModels.mockResolvedValue([]);
        panel_open.set(true);
        view.set('chat');
        render_panel();
        await vi.waitFor(() => expect(document.querySelector('.ai-cta')).not.toBeNull());

        const seen: CustomEvent[] = [];
        const listener = (event: Event) => seen.push(event as CustomEvent);
        window.addEventListener('tdrive:open_settings', listener);
        document.querySelector<HTMLButtonElement>('.ai-cta')!.click();
        window.removeEventListener('tdrive:open_settings', listener);
        expect(seen[0]?.detail).toEqual({ section: 'ai' });
    });

    it('plans an organize request, then previews and applies it with exclusions', async () => {
        mocked.PlanOrganize.mockResolvedValue(plan);
        mocked.ApplyPlan.mockResolvedValue({ plan_id: 'plan-1', moved: 2, created_folders: ['Goku'], failed: null, undo_token: 'tok-1' });
        panel_open.set(true);
        view.set('chat');
        render_panel();

        // The organize chip starts the plan; typed text no longer does (the model decides).
        button_with_text(document.body, 'Organize photos by topic').click();
        flushSync();
        await vi.waitFor(() => expect(document.querySelector('.ai-plan')?.textContent).toContain('Goku'));
        const card = document.querySelector<HTMLElement>('.ai-plan')!;
        expect(mocked.PlanOrganize).toHaveBeenCalledWith(expect.objectContaining({ instruction: 'Organize photos by topic', model_id: '' }));
        expect(card.textContent).toContain('Goku');
        expect(card.textContent).toContain('New');

        button_with_text(card, 'Preview changes').click();
        flushSync();
        await vi.waitFor(() => expect(document.querySelector('.ai-row')).not.toBeNull());
        const boxes = Array.from(document.querySelectorAll<HTMLInputElement>('.ai-check'));
        expect(boxes).toHaveLength(3);
        boxes[1].checked = false;
        boxes[1].dispatchEvent(new Event('change', { bubbles: true }));
        flushSync();

        const apply = Array.from(document.querySelectorAll('button')).find((b) => b.classList.contains('is-accent') && b.closest('.modal-card'));
        apply!.click();
        flushSync();
        await vi.waitFor(() => expect(mocked.ApplyPlan).toHaveBeenCalled());
        expect(mocked.ApplyPlan).toHaveBeenCalledWith('plan-1', [12]);
        expect(refresh_mock).toHaveBeenCalled();
    });

    it('undoes an applied plan through UndoApply', async () => {
        mocked.PlanOrganize.mockResolvedValue(plan);
        mocked.ApplyPlan.mockResolvedValue({ plan_id: 'plan-1', moved: 3, created_folders: [], failed: null, undo_token: 'tok-9' });
        mocked.UndoApply.mockResolvedValue({ restored: 3, skipped: null, failed: null });
        panel_open.set(true);
        view.set('chat');
        render_panel();

        button_with_text(document.body, 'Organize photos by topic').click();
        flushSync();
        await vi.waitFor(() => expect(document.querySelector('.ai-plan')?.textContent).toContain('Goku'));
        button_with_text(document.querySelector('.ai-plan')!, 'Apply').click();
        flushSync();
        await vi.waitFor(() => expect(mocked.ApplyPlan).toHaveBeenCalledWith('plan-1', []));

        await vi.waitFor(() => expect(document.querySelector('.ai-plan')?.textContent).toContain('Undo'));
        button_with_text(document.querySelector('.ai-plan')!, 'Undo').click();
        await vi.waitFor(() => expect(mocked.UndoApply).toHaveBeenCalledWith('tok-9'));
        expect(notify_mock).toHaveBeenCalledWith(expect.objectContaining({ level: 'success' }));
    });

    it('sends typed organize text as chat in Agent mode; the model decides on tools', async () => {
        mocked.CreateSession.mockResolvedValue({ id: 's2', title: 'x', scope: { channel_id: 0, folder_id: '' }, created_at: 1, updated_at: 1, messages: [] });
        mocked.SendMessage.mockResolvedValue({ id: 'r3', role: 'assistant', text: 'ok', created_at: 4 });
        panel_open.set(true);
        view.set('chat');
        render_panel();

        await type_and_send('Organize my photos');
        await vi.waitFor(() => expect(mocked.SendMessage).toHaveBeenCalled());
        expect(mocked.SendMessage).toHaveBeenCalledWith(expect.objectContaining({ mode: 'agent' }));
        expect(mocked.PlanOrganize).not.toHaveBeenCalled();
    });

    it('passes Ask mode to the backend and never plans from chat text', async () => {
        mocked.CreateSession.mockResolvedValue({ id: 's3', title: 'x', scope: { channel_id: 0, folder_id: '' }, created_at: 1, updated_at: 1, messages: [] });
        mocked.SendMessage.mockResolvedValue({ id: 'r4', role: 'assistant', text: 'ok', created_at: 4 });
        mode.set('ask');
        panel_open.set(true);
        view.set('chat');
        render_panel();

        await type_and_send('Organize my photos');
        await vi.waitFor(() => expect(mocked.SendMessage).toHaveBeenCalled());
        expect(mocked.SendMessage).toHaveBeenCalledWith(expect.objectContaining({ mode: 'ask' }));
        expect(mocked.PlanOrganize).not.toHaveBeenCalled();
    });
});
