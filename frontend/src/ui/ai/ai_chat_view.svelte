<script lang="ts">
    import ArrowClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import WarningCircleIcon from 'phosphor-svelte/lib/WarningCircleIcon';
    import AiPlanCard from './ai_plan_card.svelte';
    import { render_ai_markdown } from './ai_markdown';
    import { retry_error, run_suggestion } from './ai_chat';
    import { busy, streaming_text, transcript } from './ai_store';
    import { t_store } from '../../i18n/i18n';

    let scroller = $state<HTMLDivElement | null>(null);

    // Keep the newest line in view as replies arrive.
    $effect(() => {
        void $transcript;
        void $streaming_text;
        if (scroller) scroller.scrollTop = scroller.scrollHeight;
    });

    // A running plan has its own progress card, so the generic dots would repeat it.
    const planning = $derived($transcript.some((item) => item.kind === 'plan' && item.card.status === 'planning'));

    const suggestions = [
        { key: 'ai.suggest_organize_photos', organize: true },
        { key: 'ai.suggest_gather_screenshots', organize: true },
        { key: 'ai.suggest_find_duplicates', organize: false },
    ] as const;
</script>

<div class="ai-chat" bind:this={scroller} role="log" aria-live="polite" aria-relevant="additions">
    {#if $transcript.length === 0 && !$busy}
        <div class="ai-welcome">
            <p class="ai-welcome-title">{$t_store('ai.empty_title')}</p>
            <p class="ai-welcome-body">{$t_store('ai.empty_body')}</p>
            <div class="ai-suggestions">
                {#each suggestions as suggestion (suggestion.key)}
                    {@const text = $t_store(suggestion.key)}
                    <button type="button" class="ai-chip" onclick={() => run_suggestion(text, suggestion.organize)}>
                        {text}
                    </button>
                {/each}
            </div>
        </div>
    {/if}

    {#each $transcript as item (item.id)}
        {#if item.kind === 'text' && item.role === 'user'}
            <div class="ai-bubble-row is-user">
                <div class="ai-bubble">{item.text}</div>
            </div>
        {:else if item.kind === 'text'}
            <!-- eslint-disable-next-line svelte/no-at-html-tags -- render_ai_markdown drops raw HTML and sanitizes the output -->
            <div class="ai-reply">{@html render_ai_markdown(item.text)}</div>
        {:else if item.kind === 'plan'}
            <AiPlanCard id={item.id} card={item.card} />
        {:else}
            <div class="ai-error" role="alert">
                <WarningCircleIcon size={16} aria-hidden="true" />
                <span class="ai-error-text">
                    {#if item.error === 'no_model'}
                        {$t_store('ai.error_no_model')}
                    {:else if item.error === 'invalid_key'}
                        {$t_store('ai.error_invalid_key')}
                    {:else if item.error === 'rate_limit'}
                        {$t_store('ai.error_rate_limit')}
                    {:else}
                        {$t_store('ai.error_generic')}
                    {/if}
                </span>
                {#if item.retry_text}
                    <button type="button" class="ai-retry" onclick={() => retry_error(item.id)}>
                        <ArrowClockwiseIcon size={14} aria-hidden="true" />
                        {$t_store('ai.retry')}
                    </button>
                {/if}
            </div>
        {/if}
    {/each}

    {#if $busy && !planning}
        {#if $streaming_text}
            <div class="ai-reply is-streaming">{$streaming_text}</div>
        {:else}
            <div class="ai-typing" role="status" aria-label={$t_store('ai.thinking')}>
                <span></span><span></span><span></span>
            </div>
        {/if}
    {/if}
</div>

<style>
    .ai-chat {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        padding: var(--space-4);
        display: flex;
        flex-direction: column;
        gap: var(--space-3-5);
    }

    .ai-welcome {
        margin-top: var(--space-6);
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }

    .ai-welcome-title {
        margin: 0;
        font-size: var(--type-lg);
        font-weight: var(--weight-semibold);
    }

    .ai-welcome-body {
        margin: 0 0 var(--space-3);
        color: var(--text-muted);
        line-height: var(--leading-normal);
    }

    .ai-suggestions {
        display: flex;
        flex-direction: column;
        align-items: flex-start;
        gap: var(--space-2);
    }

    .ai-chip {
        height: 28px;
        padding: 0 var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        background: var(--color-surface-2);
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-sm);
        cursor: pointer;
        white-space: nowrap;
    }

    .ai-chip:hover {
        border-color: var(--color-accent);
    }

    .ai-chip:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    .ai-bubble-row {
        display: flex;
    }

    .ai-bubble-row.is-user {
        justify-content: flex-end;
    }

    .ai-bubble {
        max-width: 85%;
        padding: var(--space-2) var(--space-3);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-2);
        white-space: pre-wrap;
        word-break: break-word;
        line-height: var(--leading-normal);
    }

    .ai-reply {
        line-height: var(--leading-normal);
        word-break: break-word;
    }

    .ai-reply.is-streaming {
        white-space: pre-wrap;
    }

    .ai-reply :global(p) {
        margin: 0 0 var(--space-2);
    }

    .ai-reply :global(ul),
    .ai-reply :global(ol) {
        margin: 0 0 var(--space-2);
        padding-left: var(--space-5);
    }

    .ai-reply :global(pre) {
        margin: 0 0 var(--space-2);
        padding: var(--space-2) var(--space-3);
        border-radius: var(--radius-sm);
        background: var(--color-canvas);
        overflow-x: auto;
        font-size: var(--type-sm);
    }

    .ai-reply :global(code) {
        font-family: ui-monospace, Consolas, monospace;
        font-size: var(--type-sm);
    }

    .ai-typing {
        display: inline-flex;
        gap: 4px;
        padding: var(--space-2) 0;
    }

    .ai-typing span {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        background: var(--text-muted);
        animation: ai-blink 1.2s infinite ease-in-out;
    }

    .ai-typing span:nth-child(2) {
        animation-delay: 0.15s;
    }

    .ai-typing span:nth-child(3) {
        animation-delay: 0.3s;
    }

    @keyframes ai-blink {
        0%, 80%, 100% { opacity: 0.25; }
        40% { opacity: 1; }
    }

    @media (prefers-reduced-motion: reduce) {
        .ai-typing span {
            animation: none;
        }
    }

    .ai-error {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        padding: var(--space-2) var(--space-3);
        border-radius: var(--radius-sm);
        background: color-mix(in srgb, var(--color-danger) 10%, transparent);
        color: var(--text-muted);
        font-size: var(--type-sm);
        line-height: var(--leading-snug);
    }

    .ai-error :global(svg) {
        color: var(--color-danger);
        flex: none;
    }

    .ai-error-text {
        flex: 1;
        min-width: 0;
    }

    .ai-retry {
        height: 24px;
        padding: 0 var(--space-2);
        display: inline-flex;
        align-items: center;
        gap: var(--space-1);
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--color-accent);
        font: inherit;
        font-weight: var(--weight-medium);
        cursor: pointer;
        white-space: nowrap;
    }

    .ai-retry:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }
</style>
