// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Chat turns: sessions, messages, the streamed reply, tool-call activity and
// inline errors. Organize suggestion chips are handed to ai_organize.ts; the
// model decides on its own when a typed message needs a tool.
import { get } from 'svelte/store';
import * as api from './ai_api';
import type { ActionDonePayload, ActionRequestPayload, ChatMessage, MessageDeltaPayload, SessionSummary } from './ai_api';
import { classify_ai_error, subscribe_ai_event } from './ai_api';
import { build_scope } from './ai_scope';
import { adopt_new_chat_choice, refresh_configured_models, reset_new_chat_choice, selected_model_id } from './ai_models_store';
import { title_from_text } from './ai_intent';
import { start_organize } from './ai_organize';
import {
    add_approval,
    expired_approval_items,
    record_action_done,
    rows_from_records,
    timeline_for_reply,
    type TurnActions,
} from './ai_actions';
import {
    active_session_id,
    busy,
    mode,
    next_item_id,
    sessions,
    sessions_loading,
    streaming_text,
    tools_running,
    transcript,
    view,
    type TranscriptItem,
} from './ai_store';

function sort_sessions(list: SessionSummary[]): SessionSummary[] {
    return [...list].sort((a, b) => b.updated_at - a.updated_at);
}

/** A stored message becomes its timeline (when it has tool calls) followed by its text. */
function message_items(message: ChatMessage): TranscriptItem[] {
    const text_item: TranscriptItem = {
        kind: 'text',
        id: message.id || next_item_id('msg'),
        role: message.role === 'user' ? 'user' : 'assistant',
        text: message.text,
        created_at: message.created_at,
    };
    const rows = rows_from_records(message.actions);
    if (message.role === 'user' || rows.length === 0) return [text_item];
    return [{ kind: 'actions', id: next_item_id('actions'), rows }, text_item];
}

function push(item: TranscriptItem): void {
    transcript.update((list) => [...list, item]);
}

/** Reloads the shared model list; a failed load leaves the list empty rather than stale. */
export async function refresh_models(): Promise<void> {
    try {
        await refresh_configured_models();
    } catch {
        // The Settings pane reports load failures; the composer just offers no models.
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
    reset_new_chat_choice();
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
        const items = (session.messages ?? []).flatMap(message_items);
        transcript.set([...items, ...expired_approval_items(id)]);
    } catch (error) {
        if (get(active_session_id) !== id) return;
        transcript.set([{ kind: 'error', id: next_item_id('err'), error: classify_ai_error(error), retry_text: '' }]);
    }
}

/** One entry point for the composer and the suggestion chips. */
export async function send_text(raw: string): Promise<void> {
    const text = raw.trim();
    if (!text || get(busy)) return;
    await send_chat_turn(text);
}

async function send_chat_turn(text: string): Promise<void> {
    busy.set(true);
    streaming_text.set('');
    push({ kind: 'text', id: next_item_id('user'), role: 'user', text, created_at: Date.now() });
    const scope = build_scope();
    const turn_mode = get(mode);
    const turn: TurnActions = { timeline_id: null };
    const unsubscribers: Array<() => void> = [];
    try {
        let session_id = get(active_session_id);
        if (!session_id) {
            const session = await api.create_session(title_from_text(text), scope);
            session_id = session.id;
            adopt_new_chat_choice(session_id);
            active_session_id.set(session_id);
        }
        const active = session_id;
        unsubscribers.push(subscribe_ai_event<MessageDeltaPayload>('ai:message_delta', (payload) => {
            if (payload.session_id !== active) return;
            streaming_text.update((value) => value + payload.delta);
        }));
        unsubscribers.push(subscribe_ai_event<ActionDonePayload>('ai:action_done', (payload) => {
            if (payload.session_id !== active) return;
            record_action_done(turn, payload);
        }));
        unsubscribers.push(subscribe_ai_event<ActionRequestPayload>('ai:action_request', (payload) => {
            if (payload.session_id !== active) return;
            add_approval(payload);
        }));
        const reply = await api.send_message({
            session_id: active,
            text,
            scope,
            model_id: get(selected_model_id),
            mode: turn_mode,
        });
        for (const item of timeline_for_reply(turn, reply)) push(item);
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
        unsubscribers.forEach((unsubscribe) => unsubscribe());
        streaming_text.set('');
        tools_running.set(false);
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
