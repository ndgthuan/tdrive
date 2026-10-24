// Chat turns: sessions, messages, the streamed reply and inline errors. Organize
// requests are handed to ai_organize.ts.
import { get } from 'svelte/store';
import * as api from './ai_api';
import type { ChatMessage, MessageDeltaPayload, SessionSummary } from './ai_api';
import { classify_ai_error, subscribe_ai_event } from './ai_api';
import { build_scope } from './ai_scope';
import { is_organize_request, title_from_text } from './ai_intent';
import { start_organize } from './ai_organize';
import {
    active_session_id,
    busy,
    include_folder,
    mode,
    models,
    next_item_id,
    selected_model_id,
    sessions,
    sessions_loading,
    streaming_text,
    transcript,
    view,
    type TranscriptItem,
} from './ai_store';

function sort_sessions(list: SessionSummary[]): SessionSummary[] {
    return [...list].sort((a, b) => b.updated_at - a.updated_at);
}

function message_item(message: ChatMessage): TranscriptItem {
    return {
        kind: 'text',
        id: message.id || next_item_id('msg'),
        role: message.role === 'user' ? 'user' : 'assistant',
        text: message.text,
        created_at: message.created_at,
    };
}

function push(item: TranscriptItem): void {
    transcript.update((list) => [...list, item]);
}

export async function refresh_models(): Promise<void> {
    try {
        const list = await api.list_models();
        models.set(list);
        const current = get(selected_model_id);
        if (current && !list.some((model) => model.id === current)) selected_model_id.set('');
    } catch {
        models.set([]);
    }
}

export async function refresh_sessions(): Promise<void> {
    sessions_loading.set(true);
    try {
        sessions.set(sort_sessions(await api.list_sessions()));
    } catch {
        sessions.set([]);
    } finally {
        sessions_loading.set(false);
    }
}

export function start_new_chat(): void {
    active_session_id.set(null);
    transcript.set([]);
    streaming_text.set('');
    view.set('chat');
}

export async function open_session(id: string): Promise<void> {
    view.set('chat');
    active_session_id.set(id);
    transcript.set([]);
    try {
        const session = await api.get_session(id);
        if (get(active_session_id) !== id) return;
        transcript.set((session.messages ?? []).map(message_item));
    } catch (error) {
        if (get(active_session_id) !== id) return;
        transcript.set([{ kind: 'error', id: next_item_id('err'), error: classify_ai_error(error), retry_text: '' }]);
    }
}

/** One entry point for the composer and the suggestion chips. */
export async function send_text(raw: string): Promise<void> {
    const text = raw.trim();
    if (!text || get(busy)) return;
    if (get(mode) === 'agent' && is_organize_request(text)) {
        await start_organize(text);
        return;
    }
    await send_chat_turn(text);
}

async function send_chat_turn(text: string): Promise<void> {
    busy.set(true);
    streaming_text.set('');
    push({ kind: 'text', id: next_item_id('user'), role: 'user', text, created_at: Date.now() });
    const scope = build_scope(get(include_folder));
    let unsubscribe: () => void = () => {};
    try {
        let session_id = get(active_session_id);
        if (!session_id) {
            const session = await api.create_session(title_from_text(text), scope);
            session_id = session.id;
            active_session_id.set(session_id);
        }
        const active = session_id;
        unsubscribe = subscribe_ai_event<MessageDeltaPayload>('ai:message_delta', (payload) => {
            if (payload.session_id !== active) return;
            streaming_text.update((value) => value + payload.delta);
        });
        const reply = await api.send_message({ session_id: active, text, scope, model_id: get(selected_model_id) });
        push({
            kind: 'text',
            id: reply.id || next_item_id('reply'),
            role: 'assistant',
            text: reply.text,
            created_at: reply.created_at,
        });
        void refresh_sessions();
    } catch (error) {
        push({ kind: 'error', id: next_item_id('err'), error: classify_ai_error(error), retry_text: text });
    } finally {
        unsubscribe();
        streaming_text.set('');
        busy.set(false);
    }
}

/** Drops a failed turn's error line and sends its text again. */
export async function retry_error(item_id: string): Promise<void> {
    const item = get(transcript).find((entry) => entry.id === item_id);
    if (!item || item.kind !== 'error' || !item.retry_text) return;
    transcript.update((list) => list.filter((entry) => entry.id !== item_id));
    await send_text(item.retry_text);
}

/** Suggestion chips: organize chips start the organize flow in Agent mode, others send as chat. */
export async function run_suggestion(text: string, organize: boolean): Promise<void> {
    if (organize && get(mode) === 'agent') {
        await start_organize(text);
        return;
    }
    await send_text(text);
}
