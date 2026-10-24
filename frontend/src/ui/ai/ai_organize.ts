// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Photo organize flow: plan (with progress), preview, apply and undo. Nothing
// moves until apply, and apply only moves files, so undo always has a target.
import { get } from 'svelte/store';
import * as api from './ai_api';
import type { Plan, ProgressPayload } from './ai_api';
import { classify_ai_error, subscribe_ai_event } from './ai_api';
import { build_scope } from './ai_scope';
import { selected_model_id } from './ai_models_store';
import {
    busy,
    next_item_id,
    organize_preview,
    transcript,
    type PlanCard,
    type TranscriptItem,
} from './ai_store';
import { notify } from '../../modules/notifications';
import { refreshFiles } from '../../modules/file_list';
import { t } from '../../i18n/i18n';

/** One toast is replaced in place while files move, then turns into the result. */
const APPLY_TOAST_ID = 'ai-organize-apply';
const LONG_TOAST_MS = 10 * 60 * 1000;

export function plan_file_count(plan: Plan): number {
    return (plan.folders ?? []).reduce((sum, folder) => sum + (folder.files?.length ?? 0), 0);
}

function push(item: TranscriptItem): void {
    transcript.update((list) => [...list, item]);
}

function find_card(card_id: string): PlanCard | null {
    const item = get(transcript).find((entry) => entry.kind === 'plan' && entry.id === card_id);
    return item && item.kind === 'plan' ? item.card : null;
}

export function patch_card(card_id: string, patch: Partial<PlanCard>): void {
    transcript.update((list) => list.map((item) => (
        item.kind === 'plan' && item.id === card_id ? { ...item, card: { ...item.card, ...patch } } : item
    )));
}

/** Starts a plan for the instruction in the current scope and adds its card to the chat. */
export async function start_organize(instruction: string): Promise<void> {
    if (get(busy)) return;
    const text = instruction.trim();
    if (!text) return;
    const card_id = next_item_id('plan');
    const scope = build_scope();
    push({ kind: 'text', id: next_item_id('user'), role: 'user', text, created_at: Date.now() });
    push({
        kind: 'plan',
        id: card_id,
        card: {
            status: 'planning',
            instruction: text,
            scope,
            analyzed: 0,
            total: 0,
            plan: null,
            moved: 0,
            undo_token: '',
            error: null,
        },
    });
    await run_plan(card_id);
}

async function run_plan(card_id: string): Promise<void> {
    const card = find_card(card_id);
    if (!card) return;
    busy.set(true);
    patch_card(card_id, { status: 'planning', analyzed: 0, total: 0, error: null, plan: null });
    const unsubscribe = subscribe_ai_event<ProgressPayload>('ai:organize_progress', (progress) => {
        patch_card(card_id, { analyzed: progress.analyzed ?? 0, total: progress.total });
    });
    try {
        const plan = await api.plan_organize({
            scope: card.scope,
            instruction: card.instruction,
            model_id: get(selected_model_id),
        });
        patch_card(card_id, { status: 'ready', plan, analyzed: plan.total, total: plan.total });
    } catch (error) {
        patch_card(card_id, { status: 'failed', error: classify_ai_error(error) });
    } finally {
        unsubscribe();
        busy.set(false);
    }
}

export function retry_plan(card_id: string): void {
    void run_plan(card_id);
}

export function open_preview(card_id: string): void {
    const card = find_card(card_id);
    if (!card?.plan) return;
    organize_preview.set({ card_id, instruction: card.instruction, plan: card.plan, excluded: [] });
}

export function close_preview(): void {
    organize_preview.set(null);
}

/** Adds or removes one file from the set the apply step will skip. */
export function set_file_excluded(file_id: number, excluded: boolean): void {
    organize_preview.update((preview) => {
        if (!preview) return preview;
        const without = preview.excluded.filter((id) => id !== file_id);
        return { ...preview, excluded: excluded ? [...without, file_id] : without };
    });
}

function show_progress(done: number, total: number): void {
    notify({
        id: APPLY_TOAST_ID,
        level: 'info',
        title: t('ai_organize.moving', { done, total }),
        spinner: true,
        durationMs: LONG_TOAST_MS,
        history: false,
    });
}

export async function apply_card(card_id: string, excluded: number[]): Promise<void> {
    const card = find_card(card_id);
    if (!card?.plan || card.status === 'applying') return;
    const plan = card.plan;
    const total = plan_file_count(plan) - excluded.length;
    patch_card(card_id, { status: 'applying' });
    show_progress(0, total);
    const unsubscribe = subscribe_ai_event<ProgressPayload>('ai:apply_progress', (progress) => {
        show_progress(progress.done ?? 0, progress.total);
    });
    try {
        const result = await api.apply_plan(plan.plan_id, excluded);
        patch_card(card_id, { status: 'applied', moved: result.moved, undo_token: result.undo_token });
        refreshFiles({ background: true });
        const failed = result.failed?.length ?? 0;
        notify({
            id: APPLY_TOAST_ID,
            level: failed > 0 ? 'warning' : 'success',
            title: t('ai_organize.moved_done', { count: result.moved }),
            body: failed > 0 ? t('ai_organize.some_failed', { count: failed }) : '',
            durationMs: LONG_TOAST_MS,
            action: result.undo_token
                ? { label: t('ai_organize.undo'), run: () => void undo_card(card_id) }
                : undefined,
        });
    } catch {
        patch_card(card_id, { status: 'ready' });
        notify({
            id: APPLY_TOAST_ID,
            level: 'error',
            title: t('ai_organize.apply_failed'),
        });
    } finally {
        unsubscribe();
    }
}

export async function undo_card(card_id: string): Promise<void> {
    const card = find_card(card_id);
    if (!card?.undo_token) return;
    try {
        const result = await api.undo_apply(card.undo_token);
        patch_card(card_id, { status: 'undone', undo_token: '' });
        refreshFiles({ background: true });
        notify({
            level: 'success',
            title: t('ai_organize.undo_done', { count: result.restored }),
        });
    } catch {
        notify({ level: 'error', title: t('ai_organize.undo_failed') });
    }
}
