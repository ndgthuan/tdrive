<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import PlusIcon from 'phosphor-svelte/lib/PlusIcon';
    import { format_relative_time } from './ai_format';
    import { open_session, start_new_chat } from './ai_chat';
    import { sessions, sessions_loading, sessions_query, sessions_searching } from './ai_store';
    import { t_store } from '../../i18n/i18n';

    const visible = $derived.by(() => {
        const needle = $sessions_query.trim().toLocaleLowerCase();
        if (!needle) return $sessions;
        return $sessions.filter((session) => session.title.toLocaleLowerCase().includes(needle));
    });

    let search_input = $state<HTMLInputElement | null>(null);

    $effect(() => {
        if ($sessions_searching) search_input?.focus();
    });
</script>

<div class="ai-sessions">
    {#if $sessions_searching}
        <div class="ai-search">
            <input
                bind:this={search_input}
                bind:value={$sessions_query}
                type="search"
                class="ai-search-input"
                placeholder={$t_store('ai.search_placeholder')}
                aria-label={$t_store('ai.search_chats')}
                autocomplete="off"
                spellcheck="false"
            />
        </div>
    {/if}

    {#if $sessions_loading && $sessions.length === 0}
        <p class="ai-muted-line">{$t_store('ai.sessions_loading')}</p>
    {:else if $sessions.length === 0}
        <div class="ai-empty">
            <p class="ai-empty-title">{$t_store('ai.no_sessions')}</p>
            <button type="button" class="ai-button is-accent" onclick={() => start_new_chat()}>
                <PlusIcon size={14} aria-hidden="true" />
                {$t_store('ai.new_chat')}
            </button>
        </div>
    {:else if visible.length === 0}
        <p class="ai-muted-line">{$t_store('ai.no_matches')}</p>
    {:else}
        <ul class="ai-session-list" aria-label={$t_store('ai.panel_title')}>
            {#each visible as session (session.id)}
                <li>
                    <button type="button" class="ai-session" onclick={() => open_session(session.id)}>
                        <span class="ai-session-dot" aria-hidden="true"></span>
                        <span class="ai-session-text">
                            <span class="ai-session-title">{session.title || $t_store('ai.untitled_chat')}</span>
                            <span class="ai-session-time">{format_relative_time(session.updated_at)}</span>
                        </span>
                    </button>
                </li>
            {/each}
        </ul>
    {/if}
</div>

<style>
    .ai-sessions {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        padding: var(--space-2);
    }

    .ai-search {
        padding: 0 var(--space-1) var(--space-2);
    }

    .ai-search-input {
        width: 100%;
        height: 32px;
        padding: 0 var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        background: var(--color-canvas);
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-base);
    }

    .ai-search-input,
    .ai-search-input:focus,
    .ai-search-input:focus-visible {
        outline: none;
    }

    .ai-search-input:focus,
    .ai-search-input:focus-visible {
        border-color: rgb(127 156 196 / 0.5);
        box-shadow: 0 0 0 3px rgb(127 156 196 / 0.12);
    }

    .ai-session-list {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }

    .ai-session {
        width: 100%;
        display: flex;
        align-items: flex-start;
        gap: var(--space-3);
        padding: var(--space-2-5) var(--space-3);
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
    }

    .ai-session:hover {
        background: var(--color-surface-2);
    }

    .ai-session:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    .ai-session-dot {
        width: 6px;
        height: 6px;
        margin-top: 7px;
        flex: none;
        border-radius: 50%;
        background: var(--text-muted);
    }

    .ai-session-text {
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }

    .ai-session-title {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        font-weight: var(--weight-medium);
        line-height: var(--leading-snug);
    }

    .ai-session-time {
        font-size: var(--type-xs);
        color: var(--text-muted);
    }

    .ai-empty {
        padding: var(--space-8) var(--space-4);
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-3);
        text-align: center;
    }

    .ai-empty-title,
    .ai-muted-line {
        margin: 0;
        color: var(--text-muted);
    }

    .ai-muted-line {
        padding: var(--space-4);
    }

    .ai-button {
        height: 32px;
        padding: 0 var(--space-3);
        display: inline-flex;
        align-items: center;
        gap: var(--space-2);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        background: var(--color-surface-2);
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-base);
        font-weight: var(--weight-medium);
        cursor: pointer;
        white-space: nowrap;
    }

    .ai-button.is-accent {
        background: var(--color-accent);
        border-color: var(--color-accent);
        color: var(--color-on-accent);
        font-weight: var(--weight-semibold);
    }

    .ai-button:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }
</style>
