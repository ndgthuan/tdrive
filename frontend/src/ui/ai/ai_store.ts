// Shared state for the AI panel. Plain writable stores: the panel, the toggle
// in the top bar and the controllers all read the same values.
import { writable } from 'svelte/store';
import type { ActionItem, AiErrorKind, Plan, Scope, SessionSummary } from './ai_api';

export type AiView = 'list' | 'chat';
export type AiMode = 'agent' | 'ask';
export type PlanStatus = 'planning' | 'ready' | 'applying' | 'applied' | 'undone' | 'failed';

export interface PlanCard {
    status: PlanStatus;
    instruction: string;
    scope: Scope;
    analyzed: number;
    total: number;
    plan: Plan | null;
    moved: number;
    undo_token: string;
    error: AiErrorKind | null;
}

/** One tool call in the timeline. Rows are shown in order, first call on top. */
export interface ActionRow {
    id: string;
    tool: string;
    summary: string;
    ok: boolean;
    error: string;
    undo_token: string;
    undone: boolean;
    undoing: boolean;
}

/** Approval states: pending and resolving keep the buttons; the rest collapse to an outcome row. */
export type ApprovalState = 'pending' | 'resolving' | 'approved' | 'denied' | 'expired';

export interface ApprovalRequest {
    request_id: string;
    tool: string;
    summary: string;
    items: ActionItem[];
    state: ApprovalState;
}

export type TranscriptItem =
    | { kind: 'text'; id: string; role: 'user' | 'assistant'; text: string; created_at: number }
    | { kind: 'error'; id: string; error: AiErrorKind; retry_text: string }
    | { kind: 'plan'; id: string; card: PlanCard }
    | { kind: 'actions'; id: string; rows: ActionRow[] }
    | { kind: 'approval'; id: string; request: ApprovalRequest };

export interface OrganizePreview {
    card_id: string;
    instruction: string;
    plan: Plan;
    excluded: number[];
}

export const panel_open = writable(false);
export const view = writable<AiView>('list');
export const sessions = writable<SessionSummary[]>([]);
export const sessions_loading = writable(false);
export const sessions_query = writable('');
export const sessions_searching = writable(false);
export const active_session_id = writable<string | null>(null);
export const transcript = writable<TranscriptItem[]>([]);
export const busy = writable(false);
/** True from the first tool call of a turn until its reply lands; drives the 'Working' label. */
export const tools_running = writable(false);
export const streaming_text = writable('');
export const mode = writable<AiMode>('agent');
export const organize_preview = writable<OrganizePreview | null>(null);

let item_sequence = 0;

/** Unique id for a transcript item; ids only need to be unique within this session. */
export function next_item_id(prefix: string): string {
    item_sequence += 1;
    return `${prefix}-${item_sequence}`;
}
