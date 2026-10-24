// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Typed boundary between the AI panel and the Wails AIService binding. Every
// call goes through here so components never touch the generated bridge, and
// tests can replace one module.
import { Events } from '@wailsio/runtime';
import * as ai_service from '../../../bindings/TDrive/internal/app/aiservice';
import type {
    ActionItem,
    ApplyResult,
    ChatAction,
    ChatMessage,
    ChatSession,
    ModelView,
    Plan,
    PlanRequest,
    Scope,
    SendMessageRequest,
    SessionSummary,
    UndoActionResult,
    UndoResult,
} from '../../../bindings/TDrive/backend/ai/models';

export type { ActionItem, ApplyResult, ChatMessage, ChatSession, ModelView, Plan, Scope, SessionSummary, UndoResult };

/** One tool call the assistant made during a turn, as stored with the reply. */
export type ActionRecord = ChatAction;

export type AiEventName =
    | 'ai:message_delta'
    | 'ai:message_done'
    | 'ai:organize_progress'
    | 'ai:apply_progress'
    | 'ai:action_request'
    | 'ai:action_done';

export interface MessageDeltaPayload {
    session_id: string;
    delta: string;
}

export interface ProgressPayload {
    analyzed?: number;
    done?: number;
    total: number;
}

export interface ActionRequestPayload {
    session_id: string;
    request_id: string;
    tool: string;
    summary: string;
    items: ActionItem[];
}

export interface ActionDonePayload {
    session_id: string;
    tool: string;
    summary: string;
    ok: boolean;
    error?: string;
    undo_token?: string;
    items?: Array<Pick<ActionItem, 'id' | 'name'>>;
}

/** Result of an undo: ok=false carries the reason in message. */
export type ActionOutcome = UndoActionResult;

/** Why a call failed, in the terms the panel can act on. */
export type AiErrorKind = 'no_model' | 'invalid_key' | 'rate_limit' | 'generic';

export function list_models(): Promise<ModelView[]> {
    return Promise.resolve(ai_service.ListModels()).then((value) => value ?? []);
}

export function list_sessions(): Promise<SessionSummary[]> {
    return Promise.resolve(ai_service.ListSessions()).then((value) => value ?? []);
}

export function get_session(id: string): Promise<ChatSession> {
    return Promise.resolve(ai_service.GetSession(id));
}

export function create_session(title: string, scope: Scope): Promise<ChatSession> {
    return Promise.resolve(ai_service.CreateSession(title, scope));
}

/** Agent mode lets the model call file tools; Ask mode answers only. */
export function send_message(request: SendMessageRequest): Promise<ChatMessage> {
    return Promise.resolve(ai_service.SendMessage(request));
}

export function plan_organize(request: PlanRequest): Promise<Plan> {
    return Promise.resolve(ai_service.PlanOrganize(request));
}

export function apply_plan(plan_id: string, excluded_file_ids: number[]): Promise<ApplyResult> {
    return Promise.resolve(ai_service.ApplyPlan(plan_id, excluded_file_ids));
}

export function undo_apply(undo_token: string): Promise<UndoResult> {
    return Promise.resolve(ai_service.UndoApply(undo_token));
}

/**
 * Approves or refuses one pending tool call. The backend returns nothing on
 * success; a failure rejects.
 */
export function resolve_action(request_id: string, approved: boolean): Promise<ActionOutcome> {
    return Promise.resolve(ai_service.ResolveAction(request_id, approved)).then(() => ({ ok: true, message: '' }));
}

/** Moves the files one tool call changed back. The token works once. */
export function undo_action(undo_token: string): Promise<ActionOutcome> {
    return Promise.resolve(ai_service.UndoAction(undo_token));
}

/**
 * Subscribes to one AI event. The payload arrives as the first element of the
 * event data. A missing runtime returns a no-op teardown so the panel still
 * renders in plain browsers and tests.
 */
export function subscribe_ai_event<Payload>(name: AiEventName, handler: (payload: Payload) => void): () => void {
    try {
        const off = Events.On(name, (event) => {
            const raw: unknown = event?.data;
            const payload = Array.isArray(raw) ? raw[0] : raw;
            if (payload !== undefined && payload !== null) handler(payload as Payload);
        });
        return typeof off === 'function' ? off : () => {};
    } catch {
        return () => {};
    }
}

function error_texts(error: unknown): string {
    const parts: string[] = [];
    let current: unknown = error;
    for (let depth = 0; depth < 4 && current; depth += 1) {
        if (current instanceof Error) parts.push(current.message);
        else if (typeof current === 'string') parts.push(current);
        current = current instanceof Error && 'cause' in current ? (current as { cause?: unknown }).cause : null;
    }
    return parts.join(' ').toLowerCase();
}

/** Maps a backend failure message onto the kinds the panel explains inline. */
export function classify_ai_error(error: unknown): AiErrorKind {
    const text = error_texts(error);
    if (/no (default )?model|no model|model (is )?not (configured|set)|add a model/.test(text)) return 'no_model';
    if (/api key|apikey|invalid key|unauthori[sz]ed|forbidden|\b40[13]\b/.test(text)) return 'invalid_key';
    if (/rate limit|rate-limit|too many requests|quota|\b429\b/.test(text)) return 'rate_limit';
    return 'generic';
}
