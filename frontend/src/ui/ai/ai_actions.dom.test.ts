import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import * as aiservice from '../../../bindings/TDrive/internal/app/aiservice';
import AiPanel from './ai_panel.svelte';
import { open_session } from './ai_chat';
import { reset_action_state } from './ai_actions';
import { active_session_id, busy, panel_open, sessions, transcript, view } from './ai_store';
import { refreshFiles } from '../../modules/file_list';

// The binding predates ResolveAction and UndoAction; the wrapper reads them by name.
const action_service = aiservice as unknown as {
    ResolveAction: ReturnType<typeof vi.fn>;
    UndoAction: ReturnType<typeof vi.fn>;
};

const handlers = vi.hoisted(() => new Map<string, (event: { data: unknown[] }) => void>());

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
    Events: {
        On: vi.fn((name: string, handler: (event: { data: unknown[] }) => void) => {
            handlers.set(name, handler);
            return () => handlers.delete(name);
        }),
    },
}));

vi.mock('../../modules/file_list', () => ({ refreshFiles: vi.fn() }));
vi.mock('../../modules/notifications', () => ({ notify: vi.fn(() => 'toast') }));

const mocked = vi.mocked(aiservice);
const refresh_mock = vi.mocked(refreshFiles);

let host: HTMLElement | null = null;
let component: Record<string, unknown> | null = null;

function emit(name: string, payload: unknown): void {
    const handler = handlers.get(name);
    if (!handler) throw new Error(`no subscriber for ${name}`);
    handler({ data: [payload] });
    flushSync();
}

function render_panel(): void {
    host = document.createElement('div');
    document.body.appendChild(host);
    component = mount(AiPanel, { target: host });
    flushSync();
}

function reset_stores(): void {
    panel_open.set(false);
    view.set('list');
    sessions.set([]);
    transcript.set([]);
    active_session_id.set(null);
    busy.set(false);
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

/** Starts a turn whose reply the test releases by hand, so events can arrive mid-turn. */
function hold_reply(): () => void {
    let release: (value: unknown) => void = () => {};
    mocked.SendMessage.mockReturnValue(new Promise((resolve) => { release = resolve; }) as never);
    return () => release({ id: 'reply-1', role: 'assistant', text: 'All set.', created_at: 9 });
}

async function start_turn(text: string): Promise<() => void> {
    mocked.CreateSession.mockResolvedValue({ id: 's1', title: text, scope: { channel_id: 0, folder_id: '' }, created_at: 1, updated_at: 1, messages: [] } as never);
    const finish = hold_reply();
    panel_open.set(true);
    view.set('chat');
    render_panel();
    await type_and_send(text);
    await vi.waitFor(() => expect(mocked.SendMessage).toHaveBeenCalled());
    return finish;
}

beforeEach(() => {
    vi.clearAllMocks();
    handlers.clear();
    mocked.ListModels.mockResolvedValue([]);
    mocked.ListSessions.mockResolvedValue([]);
    action_service.ResolveAction.mockResolvedValue({ ok: true, message: '' });
    action_service.UndoAction.mockResolvedValue({ ok: true, message: '' });
    reset_stores();
});

afterEach(async () => {
    reset_action_state();
    if (component) await unmount(component);
    component = null;
    host?.remove();
    host = null;
});

describe('AI action timeline', () => {
    it('adds a row per tool call while the model works, with undo and a working label', async () => {
        const finish = await start_turn('Rename and move the report');
        emit('ai:action_done', { session_id: 's1', tool: 'rename_file', summary: 'Renamed report.pdf to Q3 report.pdf', ok: true, undo_token: 'tok-rename' });
        emit('ai:action_done', { session_id: 's1', tool: 'move_file', summary: 'Moved Q3 report.pdf to Reports', ok: true, undo_token: 'tok-move' });

        await vi.waitFor(() => expect(document.querySelectorAll('.ai-action')).toHaveLength(2));
        expect(document.querySelector('.ai-action-summary')?.textContent).toBe('Renamed report.pdf to Q3 report.pdf');
        expect(document.querySelector('.ai-working')?.textContent).toBe('Working…');

        finish();
        await vi.waitFor(() => expect(document.querySelector('.ai-reply')?.textContent).toContain('All set.'));
        expect(document.querySelector('.ai-working')).toBeNull();
    });

    it('undoes one row through UndoAction and marks it Undone', async () => {
        const finish = await start_turn('Rename it');
        emit('ai:action_done', { session_id: 's1', tool: 'rename_file', summary: 'Renamed a.txt', ok: true, undo_token: 'tok-rename' });
        await vi.waitFor(() => expect(document.querySelector('.ai-action')).not.toBeNull());

        button_with_text(document.querySelector('.ai-action')!, 'Undo').click();
        await vi.waitFor(() => expect(action_service.UndoAction).toHaveBeenCalledWith('tok-rename'));
        await vi.waitFor(() => expect(document.querySelector('.ai-action')?.textContent).toContain('Undone'));
        expect(document.querySelector('.ai-action-undo')).toBeNull();
        finish();
    });

    it('shows a failed call in red without an undo button', async () => {
        const finish = await start_turn('Move it');
        emit('ai:action_done', { session_id: 's1', tool: 'move_file', summary: 'Move report.pdf', ok: false, error: 'Destination is read-only' });
        await vi.waitFor(() => expect(document.querySelector('.ai-action-error')).not.toBeNull());
        expect(document.querySelector('.ai-action-error')?.textContent).toBe('Destination is read-only');
        expect(document.querySelector('.ai-action-undo')).toBeNull();
        finish();
    });

    it('refreshes the file list after a change, but not after a search', async () => {
        const finish = await start_turn('Find and rename');
        emit('ai:action_done', { session_id: 's1', tool: 'search_files', summary: 'Found 3 files', ok: true });
        await new Promise((resolve) => setTimeout(resolve, 380));
        expect(refresh_mock).not.toHaveBeenCalled();

        emit('ai:action_done', { session_id: 's1', tool: 'create_folder', summary: 'Created Reports', ok: true });
        await vi.waitFor(() => expect(refresh_mock).toHaveBeenCalledWith({ background: true }));
        expect(refresh_mock).toHaveBeenCalledTimes(1);
        finish();
    });

    it('renders persisted actions from a reopened chat, with undo', async () => {
        mocked.GetSession.mockResolvedValue({
            id: 's5',
            title: 'Old chat',
            scope: { channel_id: 0, folder_id: '' },
            created_at: 1,
            updated_at: 2,
            messages: [
                { id: 'm1', role: 'user', text: 'Rename a', created_at: 1 },
                {
                    id: 'm2',
                    role: 'assistant',
                    text: 'Done.',
                    created_at: 2,
                    actions: [
                        { tool: 'rename_file', summary: 'Renamed a.txt', ok: true, undo_token: 'tok-old' },
                        { tool: 'delete_file', summary: 'Delete b.txt', ok: false, error: 'Permission denied' },
                    ],
                },
            ],
        } as never);
        panel_open.set(true);
        render_panel();

        await open_session('s5');
        flushSync();
        await vi.waitFor(() => expect(document.querySelectorAll('.ai-action')).toHaveLength(2));
        expect(document.querySelector('.ai-action-error')?.textContent).toBe('Permission denied');
        expect(document.querySelector('.ai-reply')?.textContent).toContain('Done.');

        button_with_text(document.querySelector('.ai-action')!, 'Undo').click();
        await vi.waitFor(() => expect(action_service.UndoAction).toHaveBeenCalledWith('tok-old'));
    });
});

describe('AI approval card', () => {
    const delete_request = {
        session_id: 's1',
        request_id: 'req-1',
        tool: 'delete_files',
        summary: 'Delete 2 files?',
        items: [
            { id: 1, name: 'a.txt', kind: 'file' },
            { id: 2, name: 'Photos', kind: 'folder' },
        ],
    };

    it('shows the request and approves it through ResolveAction', async () => {
        const finish = await start_turn('Delete the drafts');
        emit('ai:action_request', delete_request);

        const card = await vi.waitFor(() => {
            const found = document.querySelector<HTMLElement>('.ai-approval');
            expect(found).not.toBeNull();
            return found!;
        });
        expect(card.querySelector('.ai-approval-title')?.textContent).toBe('Delete 2 files?');
        expect(card.textContent).toContain('a.txt');
        const confirm = button_with_text(card, 'Move to trash');
        expect(confirm.classList.contains('is-danger')).toBe(true);

        confirm.click();
        await vi.waitFor(() => expect(action_service.ResolveAction).toHaveBeenCalledWith('req-1', true));
        await vi.waitFor(() => expect(document.querySelector('.ai-approval')).toBeNull());
        expect(document.querySelector('.ai-approval-outcome')?.textContent).toContain('Approved');
        finish();
    });

    it('denies a request through ResolveAction and collapses it', async () => {
        const finish = await start_turn('Move everything');
        emit('ai:action_request', {
            ...delete_request,
            request_id: 'req-2',
            tool: 'move_files',
            summary: 'Move 7 files?',
            items: Array.from({ length: 7 }, (_, index) => ({ id: index, name: `f${index}.jpg`, kind: 'file' })),
        });

        const card = await vi.waitFor(() => {
            const found = document.querySelector<HTMLElement>('.ai-approval');
            expect(found).not.toBeNull();
            return found!;
        });
        expect(card.querySelectorAll('.ai-approval-item')).toHaveLength(6);
        expect(card.querySelector('.ai-approval-more')?.textContent).toBe('+1 more');

        button_with_text(card, 'Cancel').click();
        await vi.waitFor(() => expect(action_service.ResolveAction).toHaveBeenCalledWith('req-2', false));
        await vi.waitFor(() => expect(document.querySelector('.ai-approval-outcome')?.textContent).toContain('Cancelled'));
        finish();
    });

    it('shows an approval left open when the session is reopened as expired', async () => {
        const finish = await start_turn('Delete it');
        emit('ai:action_request', delete_request);
        await vi.waitFor(() => expect(document.querySelector('.ai-approval')).not.toBeNull());

        mocked.GetSession.mockResolvedValue({ id: 's1', title: 'Chat', scope: { channel_id: 0, folder_id: '' }, created_at: 1, updated_at: 1, messages: [] } as never);
        await open_session('s1');
        flushSync();
        await vi.waitFor(() => expect(document.querySelector('.ai-approval-outcome')?.textContent).toContain('This request expired.'));
        expect(action_service.ResolveAction).not.toHaveBeenCalled();
        finish();
    });
});
