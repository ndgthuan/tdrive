<script lang="ts">
    import { tick, untrack } from 'svelte';
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import ChevronDownIcon from 'phosphor-svelte/lib/CaretDownIcon';
    import HashIcon from 'phosphor-svelte/lib/HashIcon';
    import LayersIcon from 'phosphor-svelte/lib/StackIcon';
    import LockIcon from 'phosphor-svelte/lib/LockIcon';
    import SearchIcon from 'phosphor-svelte/lib/MagnifyingGlassIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import ModalShell from '../modals/modal_shell.svelte';
    import { installModalA11y } from '../modals/modal_a11y';
    import { toAppError } from '../../modules/errors';
    import type { ChannelSource } from './channel_model';
    import { createChannelTopicPager, type ChannelTopic, type ChannelTopicFetcher, type ChannelTopicView } from './channel_topics';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        source: ChannelSource;
        fetchTopics: ChannelTopicFetcher;
        selected?: ChannelTopic;
        mobile?: boolean;
        onChoose: (topic: ChannelTopic | undefined) => void;
        onTopics: (topics: readonly ChannelTopic[]) => void;
    }
    let { source, fetchTopics, selected, mobile = false, onChoose, onTopics }: Props = $props();
    const pager = createChannelTopicPager((request) => fetchTopics(request));
    let open = $state(false);
    let query = $state('');
    let view = $state<ChannelTopicView>(pager.snapshot());
    let trigger = $state<HTMLButtonElement | null>(null);
    let panel = $state<HTMLDivElement | null>(null);
    let timer: ReturnType<typeof setTimeout> | undefined;
    const label = $derived(selected?.title ?? $t_store('sources.all_topics'));
    const error = $derived(view.status === 'error' ? toAppError(view.error, { source: 'backend' }).message : '');

    function close(): void {
        open = false;
    }

    function choose(topic?: ChannelTopic): void {
        onChoose(topic);
        close();
    }

    async function load(append = false): Promise<void> {
        const pending = pager.load(append);
        view = pager.snapshot();
        view = await pending;
        if (view.status === 'ready') onTopics(view.topics);
    }

    function search(): void {
        clearTimeout(timer);
        // Invalidate immediately, before debounce, so an old query cannot appear
        // beneath the text the user has already changed.
        pager.search(query);
        view = pager.snapshot();
        timer = setTimeout(() => void load(), 300);
    }

    $effect(() => {
        // Read the source outside untrack so account/connection replacement
        // invalidates the outstanding topic request even while the picker is open.
        const target = { ...source };
        if (!open) return;
        untrack(() => {
            query = '';
            pager.select(target);
            void load();
        });
        return () => {
            clearTimeout(timer);
            pager.invalidate();
        };
    });

    $effect(() => {
        if (!open || mobile || !panel) return;
        const a11y = installModalA11y(panel, { requestClose: close, initialFocus: () => panel?.querySelector('input') ?? null, restoreFocus: () => trigger });
        void tick().then(() => {
            if (open) a11y.activate();
        });
        return () => a11y.deactivate();
    });

    function outside(event: MouseEvent): void {
        // A loading action can remove its button before this document listener
        // runs. The original event path still identifies the owning popover.
        const path = event.composedPath();
        if (open && !mobile && !path.includes(trigger as EventTarget) && !path.includes(panel as EventTarget)) {
            close();
        }
    }
</script>

<svelte:document onclick={outside} />

{#snippet contents()}
    <label class="search-field topic-search">
        <SearchIcon size={16} aria-hidden="true" />
        <input type="search" aria-label={$t_store('sources.search_topics')} placeholder={$t_store('sources.search_topics_placeholder')} maxlength="120" autocomplete="off" spellcheck="false" bind:value={query} oninput={search} />
    </label>
    <div class="topic-options" role="group" aria-label={$t_store('sources.topics')}>
        <button type="button" class="topic-option" aria-pressed={!selected} onclick={() => choose()}>
            <LayersIcon size={18} aria-hidden="true" /><span>{$t_store('sources.all_topics')}</span>
            {#if !selected}<CheckIcon class="topic-check" size={16} aria-hidden="true" />{/if}
        </button>
        {#each view.topics as topic (topic.id)}
            <button type="button" class="topic-option" aria-pressed={selected?.id === topic.id} onclick={() => choose(topic)}>
                <span class="topic-symbol" style:color={topic.iconColor > 0 && topic.iconColor <= 0xffffff ? `#${topic.iconColor.toString(16).padStart(6, '0')}` : undefined}><HashIcon size={18} aria-hidden="true" /></span>
                <span class="topic-name">{topic.title}</span>
                {#if topic.closed}<LockIcon size={13} aria-hidden="true" weight="bold" />{/if}
                {#if selected?.id === topic.id}<CheckIcon class="topic-check" size={16} aria-hidden="true" />{/if}
            </button>
        {/each}
        {#if view.status === 'loading' || view.status === 'idle'}
            <p class="topic-note" role="status">{$t_store('sources.loading_topics')}</p>
        {:else if error}
            <p class="topic-note" role="alert">{error}</p>
            <button class="topic-retry link-button" type="button" onclick={() => void load(view.topics.length > 0)}>{$t_store('sources.try_again')}</button>
        {:else if view.topics.length === 0}
            <p class="topic-note" role="status">{query.trim() ? $t_store('sources.no_topics_match') : $t_store('sources.no_topics')}</p>
        {/if}
        {#if view.status === 'ready' && view.hasMore}
            <button class="topic-retry link-button" type="button" onclick={() => void load(true)}>{$t_store('sources.load_more_topics')}</button>
        {:else if view.capped}
            <p class="topic-note">{$t_store('sources.search_for_more_topics')}</p>
        {/if}
    </div>
{/snippet}

<div class="topic-picker" class:is-mobile={mobile}>
    <button bind:this={trigger} class="topic-trigger" type="button" aria-haspopup="dialog" aria-expanded={open} aria-label={$t_store('sources.choose_topic_aria', { label })} onclick={(event) => { event.stopPropagation(); open = !open; }}>
        {#if selected}<HashIcon size={15} aria-hidden="true" />{:else}<LayersIcon size={15} aria-hidden="true" />{/if}
        <span>{label}</span><ChevronDownIcon size={14} aria-hidden="true" weight="bold" />
    </button>
    {#if !mobile && open}
        <div bind:this={panel} class="topic-popover" role="dialog" aria-label={$t_store('sources.choose_topic')} tabindex="-1">
            {@render contents()}
        </div>
    {/if}
</div>
{#if mobile}
    <div id="channel-topic-sheet" class="modal-overlay" aria-hidden="true">
        <ModalShell hostId="channel-topic-sheet" {open} title={$t_store('sources.choose_topic')} titleId="channel-topic-title" presentation="sheet" cardClass="topic-sheet" initialFocus=".topic-option" onClose={close}>
            {#snippet header()}
                <div class="topic-sheet-heading"><h2 id="channel-topic-title">{$t_store('sources.choose_topic')}</h2><button class="topic-close" type="button" aria-label={$t_store('sources.close_topics')} onclick={close}><XIcon size={22} aria-hidden="true" /></button></div>
            {/snippet}
            {@render contents()}
        </ModalShell>
    </div>
{/if}

<style>
    .topic-picker {
        position: relative;
        flex: 0 1 auto;
        min-width: 0;
    }

    .topic-trigger {
        display: flex;
        align-items: center;
        gap: 6px;
        max-width: 200px;
        height: 32px;
        padding: 0 12px;
        border: 0;
        border-radius: var(--radius-pill);
        background: var(--bg-panel);
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-xs);
        font-weight: var(--weight-semibold);
        cursor: pointer;
    }

    .topic-trigger span {
        min-width: 0;
        overflow: hidden;
        white-space: nowrap;
        text-overflow: ellipsis;
    }

    .topic-trigger :global(svg) {
        flex: 0 0 auto;
    }

    .topic-trigger:hover, .topic-trigger[aria-expanded='true'] {
        background: var(--color-surface-3);
    }

    .topic-trigger:focus-visible, .topic-close:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    .topic-popover {
        position: absolute;
        top: calc(100% + 6px);
        left: 0;
        z-index: var(--z-popover);
        width: 296px;
        padding: var(--space-2);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        background: var(--color-surface-1);
        box-shadow: var(--shadow-md);
        outline: none;
    }

    .topic-search {
        width: 100%;
        margin-bottom: var(--space-2);
    }

    .topic-search input {
        min-width: 0;
        width: 100%;
    }

    .topic-options {
        max-height: 320px;
        overflow-y: auto;
        overscroll-behavior: contain;
    }

    .topic-option {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        width: 100%;
        min-height: 38px;
        padding: 6px 10px;
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-sm);
        text-align: left;
        cursor: pointer;
        content-visibility: auto;
        contain-intrinsic-size: auto 38px;
    }

    .topic-option:hover, .topic-option:focus-visible {
        outline: none;
        background: var(--overlay-neutral-2);
    }

    .topic-option[aria-pressed='true'] {
        color: var(--accent);
    }

    .topic-option :global(svg), .topic-symbol {
        flex: 0 0 auto;
    }

    .topic-name {
        flex: 1 1 auto;
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .topic-option :global(.topic-check) {
        margin-left: auto;
    }

    .topic-note {
        padding: var(--space-3) var(--space-2);
        color: var(--text-muted);
        font-size: var(--type-sm);
    }

    .topic-retry {
        min-height: 38px;
        margin: 0 var(--space-2);
    }

    .is-mobile {
        padding: 0 var(--space-4) var(--space-2);
    }

    .is-mobile .topic-trigger {
        max-width: 100%;
        min-height: var(--touch-target);
        font-size: var(--mobile-type-meta);
    }

    :global(.topic-sheet) {
        max-height: min(75dvh, 600px);
    }

    :global(.topic-sheet) .topic-options {
        max-height: min(45dvh, 400px);
    }

    :global(.topic-sheet) .topic-option {
        min-height: var(--touch-target);
        font-size: var(--mobile-type-body);
    }

    :global(.topic-sheet) .topic-retry {
        min-height: var(--touch-target);
    }

    .topic-sheet-heading {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }

    .topic-sheet-heading h2 {
        color: var(--text-main);
        font-size: var(--type-lg);
    }

    .topic-close {
        display: grid;
        place-items: center;
        width: 44px;
        height: 44px;
        border: 0;
        border-radius: var(--radius-md);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
        touch-action: manipulation;
    }

</style>
