<script lang="ts">
    import { onMount } from 'svelte';
    import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeftIcon';
    import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlassIcon';
    import PlusIcon from 'phosphor-svelte/lib/PlusIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import AiChatView from './ai_chat_view.svelte';
    import AiComposer from './ai_composer.svelte';
    import AiOrganizeDialog from './ai_organize_dialog.svelte';
    import AiSessionList from './ai_session_list.svelte';
    import { panel_open, sessions, sessions_query, sessions_searching, view, active_session_id } from './ai_store';
    import { refresh_models, refresh_sessions, start_new_chat } from './ai_chat';
    import { t_store } from '../../i18n/i18n';

    // Ctrl+I (Cmd+I on macOS) toggles the panel from anywhere in the app shell.
    function on_key_down(event: KeyboardEvent): void {
        if (!(event.ctrlKey || event.metaKey) || event.shiftKey || event.altKey) return;
        if (event.key.toLowerCase() !== 'i') return;
        event.preventDefault();
        panel_open.update((open) => !open);
    }

    // Coming back to the app (a model added in another window, or Settings
    // closed) reloads the model list while the panel is open.
    function on_focus(): void {
        if ($panel_open) void refresh_models();
    }

    onMount(() => {
        window.addEventListener('keydown', on_key_down);
        window.addEventListener('focus', on_focus);
        return () => {
            window.removeEventListener('keydown', on_key_down);
            window.removeEventListener('focus', on_focus);
        };
    });

    // Models and sessions load each time the panel opens, so a model added in
    // Settings or a chat from another window shows up without a reload.
    $effect(() => {
        if ($panel_open) {
            void refresh_models();
            void refresh_sessions();
        }
    });

    const active_title = $derived(
        $active_session_id ? ($sessions.find((session) => session.id === $active_session_id)?.title ?? '') : '',
    );

    function back_to_list(): void {
        view.set('list');
        sessions_searching.set(false);
        sessions_query.set('');
    }

    function new_chat(): void {
        start_new_chat();
    }

    function toggle_search(): void {
        sessions_searching.update((on) => !on);
        if (!$sessions_searching) sessions_query.set('');
    }

    function close_panel(): void {
        panel_open.set(false);
    }
</script>

<!-- The organize preview is a modal with its own host. It sits outside the panel
     markup so it stays mounted (hidden) while the panel is closed. -->
<div id="ai-organize-modal" class="modal-overlay" style="display: none;" aria-hidden="true">
    <AiOrganizeDialog />
</div>

{#if $panel_open}
    <aside id="ai-panel" class="ai-panel" aria-label={$t_store('ai.panel_label')}>
        <header class="ai-panel-header">
            {#if $view === 'chat'}
                <button
                    type="button"
                    class="ai-icon-btn"
                    aria-label={$t_store('ai.back_to_chats')}
                    title={$t_store('ai.back_to_chats')}
                    onclick={back_to_list}
                >
                    <ArrowLeftIcon size={16} aria-hidden="true" />
                </button>
                <div class="ai-panel-title" title={active_title}>
                    {active_title || $t_store('ai.panel_title')}
                </div>
            {:else}
                <div class="ai-panel-title">{$t_store('ai.panel_title')}</div>
                <button
                    type="button"
                    class="ai-icon-btn"
                    aria-label={$t_store('ai.search_chats')}
                    title={$t_store('ai.search_chats')}
                    aria-pressed={$sessions_searching ? 'true' : 'false'}
                    onclick={toggle_search}
                >
                    <MagnifyingGlassIcon size={16} aria-hidden="true" />
                </button>
            {/if}
            <button
                type="button"
                class="ai-icon-btn"
                aria-label={$t_store('ai.new_chat')}
                title={$t_store('ai.new_chat')}
                onclick={new_chat}
            >
                <PlusIcon size={16} aria-hidden="true" />
            </button>
            <button
                type="button"
                class="ai-icon-btn"
                aria-label={$t_store('ai.close_panel')}
                title={$t_store('ai.close_panel')}
                onclick={close_panel}
            >
                <XIcon size={16} aria-hidden="true" />
            </button>
        </header>

        {#if $view === 'list'}
            <AiSessionList />
        {:else}
            <AiChatView />
            <AiComposer />
        {/if}
    </aside>
{/if}

<style>
    .ai-panel {
        width: 380px;
        flex: 0 0 380px;
        min-width: 0;
        height: calc(100vh - 64px);
        margin-top: 64px;
        display: flex;
        flex-direction: column;
        background: var(--color-surface-1);
        border-left: 1px solid var(--color-border-soft);
        color: var(--text-main);
        font-size: var(--type-base);
    }

    .ai-panel-header {
        height: 52px;
        flex: none;
        display: flex;
        align-items: center;
        gap: var(--space-1);
        padding: 0 var(--space-2) 0 var(--space-4);
        border-bottom: 1px solid var(--color-border-soft);
    }

    .ai-panel-title {
        flex: 1;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-size: var(--type-base);
        font-weight: var(--weight-semibold);
        color: var(--text-main);
    }

    .ai-icon-btn {
        width: 28px;
        height: 28px;
        flex: none;
        display: inline-grid;
        place-items: center;
        padding: 0;
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
    }

    .ai-icon-btn:hover {
        background: var(--color-surface-2);
        color: var(--text-main);
    }

    .ai-icon-btn[aria-pressed='true'] {
        background: color-mix(in srgb, var(--color-accent) 18%, transparent);
        color: var(--color-accent);
    }

    .ai-icon-btn:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }
</style>
