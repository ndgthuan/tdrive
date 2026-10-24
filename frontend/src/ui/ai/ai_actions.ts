// Tool-call activity inside a chat turn: the timeline rows, the undo button,
// the approval cards and the debounced file refresh that follows a change.
// The model decides which tools run; this file only shows what happened.
import { get } from 'svelte/store';
import * as api from './ai_api';
import type { ActionDonePayload, ActionRecord, ActionRequestPayload, ChatMessage } from './ai_api';
import { notify } from '../../modules/notifications';
import { refreshFiles } from '../../modules/file_list';
import { t } from '../../i18n/i18n';
import {
    next_item_id,
    tools_running,
    transcript,
    type ActionRow,
    type ApprovalRequest,
    type ApprovalState,
    type TranscriptItem,
} from './ai_store';

/** Refresh delay after a change, so a burst of tool calls refreshes the list once. */
export const REFRESH_DELAY_MS = 300;

/** Per-turn state: the timeline block that the turn's rows are appended to. */
export interface TurnActions {
    timeline_id: string | null;
}

/** Approvals still waiting on the user, keyed by request id, with their session. */
const open_approvals = new Map<string, { session_id: string; request: ApprovalRequest }>();

let refresh_timer: ReturnType<typeof setTimeout> | null = null;

/** Tool names that change files. Search and list calls leave the file list alone. */
export function is_mutating_tool(tool: string): boolean {
    if (/^list_/i.test(tool)) return false;
    return /rename|move|create|delete|trash|remove|restore/i.test(tool);
}

export type ToolIcon = 'rename' | 'move' | 'create' | 'delete' | 'restore' | 'search';

/** Which icon a tool's timeline row shows. */
export function icon_for_tool(tool: string): ToolIcon {
    if (/^list_/i.test(tool)) return 'search';
    if (/rename/i.test(tool)) return 'rename';
    if (/move/i.test(tool)) return 'move';
    if (/create|mkdir|new_folder/i.test(tool)) return 'create';
    if (/restore/i.test(tool)) return 'restore';
    if (/delete|trash|remove/i.test(tool)) return 'delete';
    return 'search';
}

/** Translation key for the approval button label; delete is styled as danger in the card. */
export function confirm_label_key(tool: string): string {
    if (/empty_trash/i.test(tool)) return 'ai.approval_confirm_empty_trash';
    switch (icon_for_tool(tool)) {
        case 'delete': return 'ai.approval_confirm_delete';
        case 'move': return 'ai.approval_confirm_move';
        case 'rename': return 'ai.approval_confirm_rename';
        case 'create': return 'ai.approval_confirm_create';
        default: return 'ai.approval_confirm_default';
    }
}

/** Debounced refresh of the file list, the same call organize and apply use. */
export function schedule_file_refresh(): void {
    if (refresh_timer !== null) clearTimeout(refresh_timer);
    refresh_timer = setTimeout(() => {
        refresh_timer = null;
        refreshFiles({ background: true });
    }, REFRESH_DELAY_MS);
}

function row_from_fields(
    tool: string,
    summary: string,
    ok: boolean,
    error: string | undefined,
    undo_token: string | undefined,
): ActionRow {
    return {
        id: next_item_id('row'),
        tool,
        summary,
        ok,
        error: error ?? '',
        undo_token: undo_token ?? '',
        undone: false,
        undoing: false,
    };
}

/** Rows from a stored reply, so the timeline survives a reload. */
export function rows_from_records(records: ActionRecord[] | null | undefined): ActionRow[] {
    return (records ?? []).map((record) => (
        row_from_fields(record.tool, record.summary, record.ok, record.error, record.undo_token)
    ));
}

function push(item: TranscriptItem): void {
    transcript.update((list) => [...list, item]);
}

function map_rows(update: (row: ActionRow) => ActionRow): void {
    transcript.update((list) => list.map((item) => (
        item.kind === 'actions' ? { ...item, rows: item.rows.map(update) } : item
    )));
}

function find_row(row_id: string): ActionRow | null {
    for (const item of get(transcript)) {
        if (item.kind !== 'actions') continue;
        const row = item.rows.find((entry) => entry.id === row_id);
        if (row) return row;
    }
    return null;
}

function patch_row(row_id: string, patch: Partial<ActionRow>): void {
    map_rows((row) => (row.id === row_id ? { ...row, ...patch } : row));
}

/** Adds one finished tool call to this turn's timeline, starting the timeline on the first call. */
export function record_action_done(turn: TurnActions, payload: ActionDonePayload): void {
    tools_running.set(true);
    const row = row_from_fields(payload.tool, payload.summary, payload.ok, payload.error, payload.undo_token);
    if (turn.timeline_id) {
        const timeline_id = turn.timeline_id;
        transcript.update((list) => list.map((item) => (
            item.kind === 'actions' && item.id === timeline_id ? { ...item, rows: [...item.rows, row] } : item
        )));
    } else {
        turn.timeline_id = next_item_id('actions');
        push({ kind: 'actions', id: turn.timeline_id, rows: [row] });
    }
    if (payload.ok && is_mutating_tool(payload.tool)) schedule_file_refresh();
}

/**
 * Shows the rows of a stored reply when the turn saw no live events for them,
 * so a reply never loses its timeline.
 */
export function timeline_for_reply(turn: TurnActions, reply: ChatMessage): TranscriptItem[] {
    if (turn.timeline_id) return [];
    const rows = rows_from_records(reply.actions);
    if (rows.length === 0) return [];
    turn.timeline_id = next_item_id('actions');
    return [{ kind: 'actions', id: turn.timeline_id, rows }];
}

/** Undo for one row. The token works once; the row then reads Undone. */
export async function undo_row(row_id: string): Promise<void> {
    const row = find_row(row_id);
    if (!row || !row.undo_token || row.undone || row.undoing) return;
    patch_row(row_id, { undoing: true });
    try {
        const outcome = await api.undo_action(row.undo_token);
        if (!outcome.ok) throw new Error(outcome.message);
        patch_row(row_id, { undone: true, undoing: false });
        schedule_file_refresh();
    } catch {
        patch_row(row_id, { undoing: false });
        notify({ level: 'error', title: t('ai.action_undo_failed') });
    }
}

function set_approval_state(request_id: string, state: ApprovalState): void {
    const entry = open_approvals.get(request_id);
    if (entry) entry.request = { ...entry.request, state };
    transcript.update((list) => list.map((item) => (
        item.kind === 'approval' && item.request.request_id === request_id
            ? { ...item, request: { ...item.request, state } }
            : item
    )));
}

/** Adds an approval card for a tool call that waits on the user. */
export function add_approval(payload: ActionRequestPayload): void {
    tools_running.set(true);
    const request: ApprovalRequest = {
        request_id: payload.request_id,
        tool: payload.tool,
        summary: payload.summary,
        items: payload.items ?? [],
        state: 'pending',
    };
    open_approvals.set(payload.request_id, { session_id: payload.session_id, request });
    push({ kind: 'approval', id: `approval-${payload.request_id}`, request });
}

/** Sends the user's choice. The card collapses once the backend accepts it. */
export async function resolve_approval(request_id: string, approved: boolean): Promise<void> {
    const entry = open_approvals.get(request_id);
    if (!entry || entry.request.state !== 'pending') return;
    set_approval_state(request_id, 'resolving');
    try {
        const outcome = await api.resolve_action(request_id, approved);
        set_approval_state(request_id, outcome.ok ? (approved ? 'approved' : 'denied') : 'expired');
        open_approvals.delete(request_id);
    } catch {
        set_approval_state(request_id, 'pending');
        notify({ level: 'error', title: t('ai.approval_failed') });
    }
}

/**
 * Called when a session is reopened: approvals that were never answered can no
 * longer be answered, so they come back as expired cards.
 */
export function expired_approval_items(session_id: string): TranscriptItem[] {
    const items: TranscriptItem[] = [];
    for (const [request_id, entry] of open_approvals) {
        if (entry.session_id !== session_id) continue;
        open_approvals.delete(request_id);
        items.push({
            kind: 'approval',
            id: `approval-${request_id}`,
            request: { ...entry.request, state: 'expired' },
        });
    }
    return items;
}

/** Clears pending approvals and the refresh timer, e.g. when the panel resets. */
export function reset_action_state(): void {
    open_approvals.clear();
    tools_running.set(false);
    if (refresh_timer !== null) {
        clearTimeout(refresh_timer);
        refresh_timer = null;
    }
}
