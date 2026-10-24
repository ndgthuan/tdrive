// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// The configured model list, shared by Settings > AI Models and the AI panel.
// Every add, edit, delete and default change in Settings reloads it through
// refresh_configured_models(), so the composer never shows a stale list. The
// model each chat uses is kept per chat session (a new chat has its own key).
import { derived, get, writable } from 'svelte/store';
import { list_models, type ModelView } from '../../api/ai';
import { active_session_id } from './ai_store';

export const configured_models = writable<ModelView[]>([]);

/** Model id per chat session. '' is the choice made before the chat has a session. */
export const chat_model_choices = writable<Record<string, string>>({});

export const NEW_CHAT_KEY = '';

/** Reloads the shared list and drops choices for models that no longer exist. Throws on failure. */
export async function refresh_configured_models(): Promise<ModelView[]> {
    const list = await list_models();
    configured_models.set(list);
    const known = new Set(list.map((model) => model.id));
    chat_model_choices.update((choices) => {
        const next: Record<string, string> = {};
        for (const [key, id] of Object.entries(choices)) {
            if (!id || known.has(id)) next[key] = id;
        }
        return next;
    });
    return list;
}

/** The default model, or the first configured one when none is marked default. */
export function default_model(models: readonly ModelView[]): ModelView | null {
    return models.find((model) => model.is_default) ?? models[0] ?? null;
}

/** The model a choice names; '' ("Auto") names the default. Null when no model is configured. */
export function resolve_model(models: readonly ModelView[], id: string): ModelView | null {
    if (id) {
        const chosen = models.find((model) => model.id === id);
        if (chosen) return chosen;
    }
    return default_model(models);
}

/** The model id chosen for the open chat, '' for Auto. */
export const selected_model_id = derived(
    [chat_model_choices, active_session_id],
    ([choices, session]) => choices[session ?? NEW_CHAT_KEY] ?? '',
);

/** Chooses the model for the open chat only. */
export function choose_model(id: string): void {
    const key = get(active_session_id) ?? NEW_CHAT_KEY;
    chat_model_choices.update((choices) => ({ ...choices, [key]: id }));
}

/** A chat that was started without a session takes its choice once the session exists. */
export function adopt_new_chat_choice(session_id: string): void {
    chat_model_choices.update((choices) => {
        const next = { ...choices, [session_id]: choices[NEW_CHAT_KEY] ?? '' };
        delete next[NEW_CHAT_KEY];
        return next;
    });
}

/** A new chat starts on Auto. */
export function reset_new_chat_choice(): void {
    chat_model_choices.update((choices) => {
        const next = { ...choices };
        delete next[NEW_CHAT_KEY];
        return next;
    });
}
