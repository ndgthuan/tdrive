<script lang="ts">
    import { onMount, tick, untrack } from 'svelte';
    import ChevronLeftIcon from 'phosphor-svelte/lib/CaretLeftIcon';
    import EllipsisIcon from 'phosphor-svelte/lib/DotsThreeIcon';
    import ExternalLinkIcon from 'phosphor-svelte/lib/ArrowSquareOutIcon';
    import LockIcon from 'phosphor-svelte/lib/LockIcon';
    import EyeIcon from 'phosphor-svelte/lib/EyeIcon';
    import PlayIcon from 'phosphor-svelte/lib/PlayIcon';
    import RadioTowerIcon from 'phosphor-svelte/lib/BroadcastIcon';
    import SearchIcon from 'phosphor-svelte/lib/MagnifyingGlassIcon';
    import TriangleAlertIcon from 'phosphor-svelte/lib/WarningIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import DownloadIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import { setScreenProtect } from '../../api';
    import { acquireScreenProtection, bindViewOnlyGuards } from '../viewers/view_only_guards';
    import { bindLongPress } from '../file_list/touch';
    import ChannelTopicPicker from './channel_topic_picker.svelte';
    import { mergeTopicMetadata, type ChannelTopic, type ChannelTopicFetcher } from './channel_topics';
    import ChannelSortMenu from './channel_sort_menu.svelte';
    import ChannelViewToggle from './channel_view_toggle.svelte';
    import { load_source_view_mode, save_source_view_mode, type SourceViewMode } from './channel_view_mode';
    import { thumbnail_key, wants_thumbnail, watch_thumbnail } from './channel_thumbnails';
    import { SvelteMap } from 'svelte/reactivity';
    import { fileTypeFamily, fileTypeIcon, type FileTypeFamily } from '../file_list/file_type';
    import { pushSheet } from '../modals/sheet_stack';
    import { toAppError } from '../../modules/errors';
    import { splitNameAndExt } from '../../utils';
    import { current_locale, t, t_store } from '../../i18n/i18n';
    import {
        canDownloadChannelPost,
        createChannelMediaPager,
        isOpenable,
        MAX_CHANNEL_ITEMS,
        media_columns,
        mediaDetails,
        mediaMeta,
        mediaActionKind,
        mediaActionLabel,
        mediaTitle,
        restrictionLabel,
        sortPosts,
        channelInitial,
        channelTelegramUrl,
        sourceHandle,
        type ChannelMediaFetcher,
        type ChannelMediaItem,
        type ChannelMediaKind,
        type ChannelMediaView,
        type ChannelPageMemory,
        type ChannelSort,
        type ChannelSource,
    } from './channel_model';

    interface Props {
        source: ChannelSource;
        fetchMedia: ChannelMediaFetcher;
        fetchTopics?: ChannelTopicFetcher;
        /** `posts` is the list as shown, which a video queues as its playlist. */
        onOpenPost: (item: ChannelMediaItem, source: ChannelSource, posts: readonly ChannelMediaItem[]) => void | Promise<void>;
        onOpenTelegram: (url: string) => void;
        /** The phone top bar's ⋯. Desktop offers the same actions on the sidebar row. */
        onActions?: (x: number, y: number, source: ChannelSource) => void;
        onPostActions: (x: number, y: number, item: ChannelMediaItem, source: ChannelSource, posts: readonly ChannelMediaItem[]) => void;
        /** Saves a post to a path the user picks. Desktop rows and tiles offer it on hover. */
        onDownloadPost?: (item: ChannelMediaItem, source: ChannelSource) => void;
        /** Phone only: its top bar needs a way back, desktop leaves by the sidebar. */
        onBack?: () => void;
        /** The first page from an earlier visit, shown while the fresh one loads. */
        recentPages?: ChannelPageMemory;
        mobile?: boolean;
    }

    let { source, fetchMedia, fetchTopics, onOpenPost, onOpenTelegram, onActions, onPostActions, onDownloadPost, onBack, recentPages, mobile = false }: Props = $props();

    const KINDS: ReadonlyArray<{ value: ChannelMediaKind; label_key: string }> = [
        { value: 'all', label_key: 'sources.filter_all' },
        { value: 'video', label_key: 'sources.filter_videos' },
        { value: 'audio', label_key: 'sources.filter_audio' },
        { value: 'image', label_key: 'sources.filter_images' },
        { value: 'document', label_key: 'sources.filter_documents' },
    ];
    const SEARCH_DELAY_MS = 300;
    const SKELETON_WIDTHS = [62, 44, 71, 38, 56, 49];

    const pager = createChannelMediaPager((request) => fetchMedia(request));

    let selectedTopic = $state<ChannelTopic | undefined>(untrack(() => recentPages?.selectedTopic?.(source)));
    let forum = $state(untrack(() => Boolean(source.forum || selectedTopic)));
    let topicNames = $state<ReadonlyMap<number, ChannelTopic>>(new Map());
    let view = $state<ChannelMediaView>({ status: 'idle' });
    let kind = $state<ChannelMediaKind>('all');
    let sort = $state<ChannelSort>('newest');
    // Desktop only: tiles or a table. A phone keeps its own rows either way.
    let view_mode = $state<SourceViewMode>(load_source_view_mode());
    // Preview data URLs for photo and video posts, by thumbnail_key.
    const previews = new SvelteMap<string, string>();
    let query = $state('');
    let appliedQuery = $state('');
    let scroller = $state<HTMLElement | null>(null);
    let list = $state<HTMLElement | null>(null);
    let sentinel = $state<HTMLElement | null>(null);
    let searchInput = $state<HTMLInputElement | null>(null);
    let searchOpen = $state(false);
    let scrolled = $state(false);
    // Reaching the end loads older posts only while they keep adding rows. In
    // a channel with few videos the end stays in view, so without this it
    // would page through the whole history, request after request.
    let olderAddedRows = $state(true);
    let viewVersion = 0;
    let searchTimer: ReturnType<typeof setTimeout> | undefined;

    const items = $derived('items' in view ? view.items : []);
    // What the list shows, and so what a video queues, in the chosen order.
    const shown = $derived(sortPosts(items, sort));
    const browsingSource = $derived(selectedTopic ? { ...source, title: `${source.title} · ${selectedTopic.title}` } : source);
    const searchLabel = $derived(selectedTopic ? $t_store('sources.search_files_in', { title: selectedTopic.title }) : $t_store('sources.search_files'));
    const topicUnavailable = $derived(view.status === 'error' && typeof view.error === 'object' && view.error !== null && 'code' in view.error && ['topic_unavailable', 'forum_unavailable'].includes(String(view.error.code)));
    const handle = $derived(sourceHandle(source));
    const gridding = $derived(!mobile && view_mode === 'grid');
    const listing = $derived(!mobile && view_mode === 'list');
    const telegramUrl = $derived(channelTelegramUrl(source));
    const failure = $derived(view.status === 'error' ? toAppError(view.error, { source: 'backend' }) : null);
    const empty = $derived.by(() => {
        if (appliedQuery) {
            return {
                title: $t_store('sources.no_results_title', { query: appliedQuery }),
                body: kind === 'all' ? $t_store('sources.no_results_hint') : $t_store('sources.no_results_filtered_hint'),
            };
        }
        const noun_key = { all: 'sources.noun_files', video: 'sources.noun_videos', audio: 'sources.noun_audio', image: 'sources.noun_images', document: 'sources.noun_documents' }[kind];
        const noun = $t_store(noun_key);
        const older = view.status === 'empty' && view.hasMore;
        return {
            title: selectedTopic ? $t_store('sources.empty_in_topic', { noun, title: selectedTopic.title }) : $t_store('sources.empty_title', { noun }),
            body: older ? $t_store('sources.empty_older') : $t_store('sources.empty_hint'),
        };
    });

    async function load(append = false): Promise<void> {
        if (pager.snapshot().status === 'loading') return;
        const current = ++viewVersion;
        const shown = items.length;
        const pending = pager.load({ append });
        view = pager.snapshot();
        const fresh = await pending;
        if (current !== viewVersion) return;
        view = fresh;
        syncTopicMetadata();
        olderAddedRows = !append || items.length > shown;
        if (!append) remember();
    }

    // The unfiltered first page is what a return visit shows at once.
    function remember(): void {
        if (view.status === 'ready' && kind === 'all' && !appliedQuery) recentPages?.set(source, view.items, selectedTopic?.id ?? 0);
    }

    function syncTopicMetadata(): void {
        const scope = pager.scope();
        if (view.status === 'ready' || view.status === 'empty') forum = scope.forum;
        rememberTopics(scope.topics);
    }

    function rememberTopics(topics: readonly ChannelTopic[]): void {
        const next = mergeTopicMetadata(topicNames, topics);
        topicNames = next;
        if (selectedTopic && next.has(selectedTopic.id)) {
            selectedTopic = next.get(selectedTopic.id);
            recentPages?.selectTopic?.(source, selectedTopic);
        }
    }

    function chooseTopic(next: ChannelTopic | undefined): void {
        if ((next?.id ?? 0) === (selectedTopic?.id ?? 0)) return;
        selectedTopic = next;
        recentPages?.selectTopic?.(source, next);
        pager.setTopic(next?.id ?? 0);
        reload();
    }

    function topicLabel(item: ChannelMediaItem): string {
        if (!forum || selectedTopic || !item.topicId) return '';
        return topicNames.get(item.topicId)?.title ?? t('sources.topic_fallback', { id: item.topicId });
    }

    function reload(): void {
        pager.setFilter(appliedQuery, kind);
        scroller?.scrollTo({ top: 0 });
        void load();
    }

    function chooseKind(next: ChannelMediaKind): void {
        if (next === kind) return;
        kind = next;
        reload();
    }

    function choose_view_mode(next: SourceViewMode): void {
        if (next === view_mode) return;
        view_mode = next;
        save_source_view_mode(next);
    }

    function chooseSort(next: ChannelSort): void {
        sort = next;
        scroller?.scrollTo({ top: 0 });
    }

    function onQueryInput(): void {
        clearTimeout(searchTimer);
        searchTimer = setTimeout(() => {
            if (query.trim() === appliedQuery) return;
            appliedQuery = query.trim();
            reload();
        }, SEARCH_DELAY_MS);
    }

    function clearQuery(): void {
        clearTimeout(searchTimer);
        query = '';
        if (!appliedQuery) return;
        appliedQuery = '';
        reload();
    }

    function onSearchKeydown(event: KeyboardEvent): void {
        if (event.key !== 'Escape' || !query) return;
        event.stopPropagation();
        clearQuery();
    }

    async function toggleSearch(): Promise<void> {
        if (searchOpen) {
            closeSearch();
            return;
        }
        searchOpen = true;
        await tick();
        searchInput?.focus();
    }

    function closeSearch(): void {
        searchOpen = false;
        clearQuery();
    }

    // The menu closes on any document click, so this one must not reach it.
    function openActions(event: MouseEvent): void {
        event.stopPropagation();
        const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
        onActions?.(rect.left, rect.bottom + 4, source);
    }

    function bindProtectedPost(row: HTMLElement, protectedContent: boolean) {
        let currentProtection = protectedContent;
        const cleanup = bindViewOnlyGuards(row, () => currentProtection, true);
        return { update: (next: boolean) => { currentProtection = next; }, destroy: cleanup };
    }

    function family(item: ChannelMediaItem): FileTypeFamily {
        if (item.kind === 'video' || item.kind === 'audio' || item.kind === 'image') return item.kind;
        if (item.kind === 'pdf' || item.kind === 'text') return 'document';
        return fileTypeFamily(splitNameAndExt(item.name).ext);
    }

    // A whole channel being protected is said once, in the bar; repeating it
    // on every row would only add noise.
    function badge(item: ChannelMediaItem): string {
        return source.protected && (item.protected || item.blockReason === 'protected') ? '' : restrictionLabel(item);
    }

    function rowLabel(item: ChannelMediaItem): string {
        const parts = [mediaTitle(item), mediaMeta(item).replace(/ · /g, ', '), topicLabel(item), badge(item)].filter(Boolean).join(', ');
        if (isOpenable(item)) return t('sources.row_openable', { action: mediaActionLabel(item), parts });
        return item.telegramUrl ? t('sources.row_telegram', { parts }) : t('sources.row_unavailable', { parts });
    }

    function openPostActions(event: MouseEvent, item: ChannelMediaItem): void {
        event.preventDefault();
        onPostActions(event.clientX, event.clientY, item, browsingSource, shown);
    }

    // The phone row's trailing "…": a visible way into the same actions the long
    // press opens, positioned under the button rather than at a pointer it has.
    function openRowActions(event: MouseEvent, item: ChannelMediaItem): void {
        event.stopPropagation();
        const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
        onPostActions(rect.left, rect.bottom + 4, item, browsingSource, shown);
    }

    // Arrow keys walk the rows the way they walk the file list.
    function onListKeydown(event: KeyboardEvent): void {
        const rows = Array.from((event.currentTarget as HTMLElement).querySelectorAll<HTMLButtonElement>('.channel-row-main'));
        const index = rows.indexOf(document.activeElement as HTMLButtonElement);
        if (index < 0) return;
        const next = { ArrowDown: index + 1, ArrowUp: index - 1, Home: 0, End: rows.length - 1 }[event.key];
        if (next === undefined) return;
        event.preventDefault();
        rows[Math.max(0, Math.min(rows.length - 1, next))]?.focus();
    }

    $effect(() => {
        if (!mobile || !(source.protected || shown.some((item) => item.protected))) return;
        return acquireScreenProtection(setScreenProtect);
    });

    $effect(() => {
        if (!sentinel || !scroller || typeof IntersectionObserver === 'undefined') return;
        const observer = new IntersectionObserver((entries) => {
            if (entries.some((entry) => entry.isIntersecting)) void load(true);
        }, { root: scroller, rootMargin: '0px 0px 480px 0px' });
        observer.observe(sentinel);
        return () => observer.disconnect();
    });

    // A phone has no hover or right-click: a long press opens the post's menu,
    // the way it does on a file row.
    $effect(() => {
        if (!mobile || !list) return;
        return bindLongPress(list, '.channel-row', (row, x, y) => {
            const item = items.find((entry) => String(entry.msgId) === row.dataset.msgId);
            if (item) onPostActions(x, y, item, browsingSource, shown);
        });
    });

    // Android BACK closes the phone's search field before it leaves the channel.
    $effect(() => {
        if (!searchOpen) return;
        const back = pushSheet(closeSearch);
        return () => back.release();
    });

    onMount(() => {
        pager.select(source);
        pager.setTopic(selectedTopic?.id ?? 0);
        const recent = recentPages?.get(source, selectedTopic?.id ?? 0);
        if (recent?.length) {
            // Shown until the fresh page replaces it. No more pages are asked
            // for in the meantime, so nothing appends to rows about to go.
            view = { status: 'ready', items: recent, hasMore: false, capped: false };
            const current = ++viewVersion;
            void pager.load().then((fresh) => {
                if (current !== viewVersion) return;
                view = fresh;
                syncTopicMetadata();
                remember();
            });
        } else {
            void load();
        }
        return () => {
            viewVersion += 1;
            clearTimeout(searchTimer);
            pager.invalidate();
        };
    });
</script>

{#snippet kindPicker()}
    <div class="segmented channel-kinds" role="group" aria-label={$t_store('sources.show')}>
        {#each KINDS as option (option.value)}
            <button type="button" aria-pressed={kind === option.value} onclick={() => chooseKind(option.value)}>{$t_store(option.label_key)}</button>
        {/each}
    </div>
{/snippet}

{#snippet row(item: ChannelMediaItem)}
    {@const openable = isOpenable(item)}
    {@const TypeIcon = fileTypeIcon(family(item))}
    {@const label = badge(item)}
    {@const preview_key = thumbnail_key(browsingSource, item.msgId)}
    {@const preview = wants_thumbnail(item) ? previews.get(preview_key) : undefined}
    {@const columns = media_columns(item)}
    <li class="channel-row" class:is-locked={!openable} class:is-protected={source.protected || item.protected} use:bindProtectedPost={Boolean(source.protected || item.protected)} data-msg-id={item.msgId}>
        <button
            class="channel-row-main"
            type="button"
            title={mediaDetails(item) || undefined}
            aria-label={rowLabel(item)}
            onclick={() => void onOpenPost(item, browsingSource, shown)}
            oncontextmenu={mobile ? undefined : (event) => openPostActions(event, item)}
        >
            <span
                class="file-type-icon channel-chip"
                class:has-preview={Boolean(preview)}
                aria-hidden="true"
                use:watch_thumbnail={wants_thumbnail(item) ? { source: browsingSource, msgId: item.msgId, onLoad: (url) => previews.set(preview_key, url) } : null}
            >
                {#if preview}
                    <img class="channel-chip-preview" src={preview} alt="" decoding="async" draggable="false" />
                {:else}
                    <span class="chip-layer chip-type"><TypeIcon size={listing ? 16 : 20} /></span>
                {/if}
                <span class="chip-layer chip-action">
                    {#if openable && mediaActionKind(item) === 'play'}<PlayIcon size={16} weight="fill" />{:else if openable}<EyeIcon size={16} />{:else}<ExternalLinkIcon size={16} />{/if}
                </span>
            </span>
            <span class="channel-row-text">
                <span class="channel-row-title">{mediaTitle(item)}{#if listing && topicLabel(item)}<span class="channel-row-topic"> · {topicLabel(item)}</span>{/if}</span>
                {#if !listing}<span class="channel-row-meta">{mediaMeta(item)}{#if topicLabel(item)}<span class="channel-row-topic"> · {topicLabel(item)}</span>{/if}</span>{/if}
                {#if label && !mobile}<span class="channel-row-badge" title={label === t('sources.restriction_protected') ? t('sources.view_only_hint') : undefined}>{label}</span>{/if}
            </span>
            {#if listing}
                <span class="channel-row-kind">{columns.kind}</span>
                <span class="channel-row-size">{columns.size}</span>
                <span class="channel-row-date">{columns.date}</span>
            {/if}
            {#if label && mobile}<span class="channel-row-badge" title={label === t('sources.restriction_protected') ? t('sources.view_only_hint') : undefined}>{label}</span>{/if}
        </button>
        {#if onDownloadPost && !mobile && canDownloadChannelPost(item, source)}
            <button
                class="channel-row-download"
                type="button"
                title={$t_store('sources.download')}
                aria-label={$t_store('sources.download_post', { title: mediaTitle(item) })}
                onclick={() => onDownloadPost(item, browsingSource)}
            >
                <DownloadIcon size={16} aria-hidden="true" />
            </button>
        {/if}
        {#if openable && item.telegramUrl && !mobile}
            <button
                class="channel-row-telegram"
                type="button"
                title={$t_store('sources.open_in_telegram')}
                aria-label={$t_store('sources.open_post_in_telegram', { title: mediaTitle(item) })}
                onclick={() => onOpenTelegram(item.telegramUrl)}
            >
                <ExternalLinkIcon size={16} aria-hidden="true" />
            </button>
        {/if}
        {#if mobile && (openable || item.telegramUrl)}
            <button
                class="channel-row-actions"
                type="button"
                aria-haspopup="menu"
                aria-label={$t_store('sources.actions_for', { title: mediaTitle(item) })}
                onclick={(event) => openRowActions(event, item)}
            >
                <EllipsisIcon size={20} aria-hidden="true" />
            </button>
        {/if}
    </li>
{/snippet}

{#key $current_locale}
<section class="channel-view" class:is-mobile={mobile} class:is-grid={gridding} class:is-list={listing} aria-labelledby="channel-view-title">
    {#if mobile}
        <header class="channel-topbar" class:is-scrolled={scrolled}>
            <div class="topbar-row">
                <button type="button" class="topbar-back" aria-label={$t_store('sources.back')} onclick={onBack}>
                    <ChevronLeftIcon size={24} aria-hidden="true" />
                </button>
                <div class="topbar-folder-titles">
                    <h1 id="channel-view-title" class="topbar-folder-title">{source.title}</h1>
                    <span class="topbar-folder-meta">{handle}{source.protected ? ` · ${$t_store('sources.restriction_protected')}` : ''}</span>
                </div>
                <div class="topbar-actions">
                    <button
                        type="button"
                        class="topbar-icon-btn"
                        aria-expanded={searchOpen}
                        aria-label={$t_store(searchOpen ? 'sources.hide_search' : 'sources.search_posts')}
                        onclick={toggleSearch}
                    >
                        <SearchIcon size={22} aria-hidden="true" />
                    </button>
                    <ChannelSortMenu iconOnly value={sort} onChange={chooseSort} />
                    <button type="button" class="topbar-icon-btn" aria-haspopup="menu" aria-label={$t_store('sources.source_actions')} onclick={openActions}>
                        <EllipsisIcon size={22} aria-hidden="true" />
                    </button>
                </div>
            </div>
            <div class="topbar-search-shell" data-open={searchOpen} inert={!searchOpen}>
                <div class="topbar-search">
                    <SearchIcon class="topbar-search-icon" size={18} aria-hidden="true" />
                    <input
                        bind:this={searchInput}
                        bind:value={query}
                        oninput={onQueryInput}
                        type="text"
                        enterkeyhint="search"
                        maxlength="120"
                        placeholder={searchLabel}
                        autocomplete="off"
                        spellcheck="false"
                        aria-label={selectedTopic ? searchLabel : $t_store('sources.search_files_in', { title: source.title })}
                    />
                </div>
            </div>
            {#if forum && fetchTopics}<ChannelTopicPicker {source} {mobile} fetchTopics={fetchTopics} selected={selectedTopic} onChoose={chooseTopic} onTopics={rememberTopics} />{/if}
            <div class="channel-topbar-kinds">{@render kindPicker()}</div>
        </header>
    {:else}
        <header class="channel-bar">
            <div class="channel-bar-top">
                <span class="channel-bar-avatar" aria-hidden="true">{channelInitial(source.title)}</span>
                <div class="channel-bar-heading">
                    <h1 id="channel-view-title" class="channel-bar-title" title={source.title}>{source.title}</h1>
                    <span class="channel-bar-handle">{handle}</span>
                </div>
                <div class="channel-bar-actions">
                    {#if source.protected}
                        <span class="channel-bar-protected" title={$t_store('sources.view_only_hint')}>
                            <LockIcon size={12} aria-hidden="true" weight="bold" />
                            {$t_store('sources.restriction_protected')}
                        </span>
                    {/if}
                    {#if telegramUrl}
                        <button class="channel-bar-ghost" type="button" onclick={() => onOpenTelegram(telegramUrl)}>{$t_store('sources.open_in_telegram')}</button>
                    {/if}
                    {#if onActions}
                        <button class="channel-bar-ghost channel-bar-icon" type="button" aria-haspopup="menu" aria-label={$t_store('sources.source_actions')} title={$t_store('sources.source_actions')} onclick={openActions}>
                            <EllipsisIcon size={18} aria-hidden="true" />
                        </button>
                    {/if}
                </div>
            </div>
            <div class="channel-bar-tools">
                {#if forum && fetchTopics}<ChannelTopicPicker {source} fetchTopics={fetchTopics} selected={selectedTopic} onChoose={chooseTopic} onTopics={rememberTopics} />{/if}
                {@render kindPicker()}
                <ChannelSortMenu value={sort} onChange={chooseSort} />
                {#if !mobile}<ChannelViewToggle value={view_mode} onChange={choose_view_mode} />{/if}
                <label class="search-field channel-search">
                    <SearchIcon size={15} aria-hidden="true" />
                    <input
                        bind:value={query}
                        oninput={onQueryInput}
                        onkeydown={onSearchKeydown}
                        type="text"
                        maxlength="120"
                        placeholder={searchLabel}
                        autocomplete="off"
                        spellcheck="false"
                        aria-label={selectedTopic ? searchLabel : $t_store('sources.search_files_in', { title: source.title })}
                    />
                    {#if query}
                        <button class="channel-search-clear" type="button" aria-label={$t_store('sources.clear_search')} onclick={clearQuery}>
                            <XIcon size={13} aria-hidden="true" weight="bold" />
                        </button>
                    {/if}
                </label>
            </div>
        </header>
    {/if}

    <div bind:this={scroller} class="channel-scroll" onscroll={() => { scrolled = (scroller?.scrollTop ?? 0) > 2; }}>
        {#if (view.status === 'idle' || view.status === 'loading') && items.length === 0}
            <div class="channel-skeleton" role="status" aria-busy="true" aria-label={$t_store('sources.loading_posts')}>
                {#each SKELETON_WIDTHS as width, index (index)}
                    <div class="channel-skeleton-row" style:--skeleton-delay={`${index * 65}ms`}>
                        <span class="file-state-skeleton-icon"></span>
                        <span class="channel-skeleton-text">
                            <span class="file-state-skeleton-bar" style:--skeleton-width={`${width}%`}></span>
                            <span class="file-state-skeleton-bar" style:--skeleton-width="22%"></span>
                        </span>
                    </div>
                {/each}
            </div>
        {:else if failure && items.length === 0}
            <div class="file-state is-error" role="alert">
                <div class="file-state-icon" aria-hidden="true"><TriangleAlertIcon size={24} /></div>
                <div class="file-state-title">{topicUnavailable ? $t_store('sources.topic_unavailable') : $t_store('sources.posts_failed')}</div>
                <div class="file-state-body">{failure.message}</div>
                <div class="file-state-actions">
                    <button class="secondary-btn" type="button" onclick={() => void load()}>{$t_store('sources.try_again')}</button>
                    {#if selectedTopic}<button class="secondary-btn" type="button" onclick={() => chooseTopic(undefined)}>{$t_store('sources.view_all_topics')}</button>{/if}
                </div>
            </div>
        {:else if view.status === 'empty'}
            <div class="file-state is-empty" role="status">
                <div class="file-state-icon" aria-hidden="true"><RadioTowerIcon size={24} /></div>
                <div class="file-state-title">{empty.title}</div>
                <div class="file-state-body">{empty.body}</div>
                {#if view.hasMore}
                    <div class="file-state-actions">
                        <button class="secondary-btn" type="button" onclick={() => void load(true)}>{$t_store('sources.look_further_back')}</button>
                    </div>
                {/if}
            </div>
        {:else}
            <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
            <ul bind:this={list} class="channel-rows" aria-label={$t_store('sources.posts_in', { title: source.title })} onkeydown={onListKeydown}>
                {#each shown as item (item.msgId)}
                    {@render row(item)}
                {/each}
            </ul>
            {#if view.status === 'loading'}
                <div class="channel-list-note" role="status"><span class="channel-spinner" aria-hidden="true"></span>{$t_store('sources.loading_older')}</div>
            {:else if failure}
                <div class="channel-list-note" role="alert">
                    {topicUnavailable ? failure.message : $t_store('sources.older_failed')}
                    <button class="link-button" type="button" onclick={() => void load(true)}>{$t_store('sources.try_again')}</button>
                    {#if topicUnavailable && selectedTopic}<button class="link-button" type="button" onclick={() => chooseTopic(undefined)}>{$t_store('sources.view_all_topics')}</button>{/if}
                </div>
            {:else if view.status === 'ready' && view.capped}
                <div class="channel-list-note">{$t_store('sources.capped', { max: MAX_CHANNEL_ITEMS })}</div>
            {:else if view.status === 'ready' && view.hasMore && sort !== 'newest'}
                <!-- Older posts would land anywhere in a sorted list, so they
                     arrive when asked for rather than under the reader. -->
                <div class="channel-list-note">
                    {$t_store('sources.sorted_partial')}
                    <button class="link-button" type="button" onclick={() => void load(true)}>{$t_store('sources.load_older')}</button>
                </div>
            {:else if view.status === 'ready' && view.hasMore && !olderAddedRows}
                <div class="channel-list-note">
                    {$t_store('sources.nothing_more')}
                    <button class="link-button" type="button" onclick={() => void load(true)}>{$t_store('sources.look_further_back')}</button>
                </div>
            {:else if view.status === 'ready' && view.hasMore}
                <div bind:this={sentinel} class="channel-sentinel" aria-hidden="true"></div>
            {/if}
        {/if}
    </div>
</section>
{/key}

<style>
    .channel-view {
        flex: 1 1 auto;
        min-width: 0;
        min-height: 0;
        display: flex;
        flex-direction: column;
        background: var(--bg-dark);
    }

    /* The bar is the source's header, as the design draws it: the avatar and
       name, then how the source may be used, with the posts' filters on a row
       of their own underneath. */
    .channel-bar {
        flex-shrink: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        padding: var(--space-4) var(--space-5) var(--space-3);
        border-bottom: 1px solid var(--border);
    }

    .channel-bar-top {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-width: 0;
    }

    .channel-bar-avatar {
        flex: 0 0 auto;
        display: grid;
        width: var(--control-md);
        height: var(--control-md);
        place-items: center;
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-pill);
        background: var(--color-surface-3);
        color: var(--color-text);
        font-size: var(--type-sm);
        font-weight: var(--weight-semibold);
    }

    .channel-bar-heading {
        flex: 1 1 auto;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }

    .channel-bar-title {
        min-width: 0;
        overflow: hidden;
        color: var(--color-text);
        font-size: var(--type-xl);
        font-weight: var(--weight-semibold);
        letter-spacing: -0.01em;
        line-height: var(--leading-tight);
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    /* Goes entirely on a narrow window rather than dwindling to a stray letter. */
    .channel-bar-handle {
        min-width: 0;
        overflow: hidden;
        color: var(--color-text-subtle);
        font-size: var(--type-xs);
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    @media (max-width: 1100px) {
        .channel-bar-handle { display: none; }
    }

    .channel-bar-actions {
        flex: 0 0 auto;
        display: flex;
        align-items: center;
        gap: var(--space-2);
    }

    .channel-bar-ghost {
        flex: 0 0 auto;
        height: var(--control-md);
        padding: 0 var(--space-3);
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--color-text);
        font-size: var(--type-sm);
        font-weight: var(--weight-semibold);
        white-space: nowrap;
        cursor: pointer;
        transition: background-color var(--motion-fast) var(--ease-standard);
    }

    .channel-bar-ghost:hover { background: var(--color-surface-3); }
    .channel-bar-ghost:focus-visible { outline: none; box-shadow: var(--focus-ring); }

    .channel-bar-icon {
        display: grid;
        width: var(--control-md);
        padding: 0;
        place-items: center;
        color: var(--text-muted);
    }

    .channel-bar-protected {
        flex: 0 0 auto;
        display: inline-flex;
        align-self: center;
        align-items: center;
        gap: 4px;
        height: 20px;
        padding: 0 8px;
        border-radius: var(--radius-pill);
        background: var(--overlay-neutral-2);
        color: var(--text-muted);
        font-size: var(--type-xs);
        font-weight: var(--weight-semibold);
        cursor: default;
    }

    .channel-bar-tools {
        flex: 0 1 auto;
        min-width: 0;
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: var(--space-2);
    }

    .channel-bar-tools .channel-search { margin-left: auto; }

    .channel-kinds { flex: 0 0 auto; }

    /* More file types fit without squeezing labels or shrinking touch targets. */
    .channel-topbar-kinds { overflow-x: auto; scrollbar-width: none; }

    .channel-search { width: clamp(150px, 18vw, 220px); }

    .channel-search-clear {
        flex: 0 0 auto;
        display: grid;
        width: 18px;
        height: 18px;
        place-items: center;
        border: 0;
        border-radius: var(--radius-pill);
        background: var(--overlay-neutral-3);
        color: var(--text-main);
        cursor: pointer;
    }

    .channel-search-clear:hover { background: var(--color-surface-3); }
    .channel-search-clear:focus-visible { outline: none; box-shadow: var(--focus-ring); }

    .channel-scroll {
        flex: 1 1 auto;
        min-height: 0;
        overflow-y: auto;
        padding: var(--space-5) var(--space-5) var(--space-8);
    }

    .channel-rows {
        display: flex;
        flex-direction: column;
        gap: 2px;
        list-style: none;
    }

    .channel-row {
        position: relative;
        display: flex;
        align-items: center;
        border-radius: var(--radius-md);
        transition: background-color var(--motion-fast) var(--ease-standard);
    }

    .channel-row.is-protected {
        user-select: none;
        -webkit-user-select: none;
        -webkit-touch-callout: none;
    }

    .channel-row:hover,
    .channel-row:focus-within { background: var(--bg-panel); }

    .channel-row-main {
        flex: 1 1 auto;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: 12px;
        padding: 10px 14px;
        border: 0;
        border-radius: inherit;
        background: transparent;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
    }

    .channel-row-main:focus-visible {
        outline: none;
        box-shadow: inset 0 0 0 1px var(--color-accent), var(--focus-ring);
    }

    /* The type chip doubles as the row's action: hovering turns the glyph into
       the thing a click will do, so no row needs a button of its own. */
    .channel-chip {
        position: relative;
        margin-right: 0; /* the row's gap spaces it */
        transition:
            background-color var(--motion-fast) var(--ease-standard),
            border-color var(--motion-fast) var(--ease-standard),
            color var(--motion-fast) var(--ease-standard);
    }

    .chip-layer {
        position: absolute;
        inset: 0;
        display: grid;
        place-items: center;
        transition:
            opacity var(--motion-fast) var(--ease-standard),
            transform var(--motion-fast) var(--ease-standard);
    }

    .chip-action {
        opacity: 0;
        transform: scale(0.8);
    }

    .chip-action :global(svg) {
        width: 16px;
        height: 16px;
    }

    @media (hover: hover) {
        .channel-row:hover .channel-chip,
        .channel-row-main:focus-visible .channel-chip {
            border-color: var(--accent);
            background: var(--accent);
            color: var(--color-on-accent);
        }

        .channel-row.is-locked:hover .channel-chip,
        .channel-row.is-locked .channel-row-main:focus-visible .channel-chip {
            border-color: var(--border);
            background: var(--color-surface-3);
            color: var(--color-text);
        }

        .channel-row:hover .chip-type,
        .channel-row-main:focus-visible .chip-type {
            opacity: 0;
            transform: scale(0.8);
        }

        .channel-row:hover .chip-action,
        .channel-row-main:focus-visible .chip-action {
            opacity: 1;
            transform: none;
        }
    }

    .channel-row-text {
        flex: 1 1 auto;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }

    .channel-row-title,
    .channel-row-meta {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .channel-row-title {
        color: var(--color-text);
        font-size: 0.9rem;
        font-weight: var(--weight-semibold);
        line-height: 1.35;
    }

    .channel-row-meta {
        color: var(--text-muted);
        font-size: 0.8rem;
        font-weight: var(--weight-medium);
        font-variant-numeric: tabular-nums;
        line-height: 1.35;
    }

    .channel-row-badge {
        flex: 0 0 auto;
        height: 20px;
        display: inline-grid;
        place-items: center;
        padding: 0 8px;
        border-radius: var(--radius-pill);
        background: var(--overlay-neutral-2);
        color: var(--text-muted);
        font-size: var(--type-xs);
        font-weight: var(--weight-semibold);
    }

    .channel-row-telegram {
        flex: 0 0 auto;
        display: grid;
        width: 28px;
        height: 28px;
        margin-right: 14px;
        place-items: center;
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
        transition:
            opacity var(--motion-fast) var(--ease-standard),
            background-color var(--motion-fast) var(--ease-standard),
            color var(--motion-fast) var(--ease-standard);
    }

    .channel-row-telegram:hover {
        background: var(--color-surface-3);
        color: var(--color-text);
    }

    .channel-row-telegram:focus-visible { outline: none; box-shadow: var(--focus-ring); }

    /* Save sits beside the Telegram button and shares its hover reveal. */
    .channel-row-download {
        flex: 0 0 auto;
        display: grid;
        width: 32px;
        height: 32px;
        margin: 0;
        place-items: center;
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
        transition:
            opacity var(--motion-fast) var(--ease-standard),
            background-color var(--motion-fast) var(--ease-standard),
            color var(--motion-fast) var(--ease-standard);
    }

    .channel-row-download:hover {
        background: var(--color-surface-3);
        color: var(--color-text);
    }

    .channel-row-download:focus-visible { outline: none; box-shadow: var(--focus-ring); }

    @media (hover: hover) {
        .channel-row-telegram,
        .channel-row-download { opacity: 0; }

        .channel-row:hover .channel-row-telegram,
        .channel-row:focus-within .channel-row-telegram,
        .channel-row:hover .channel-row-download,
        .channel-row:focus-within .channel-row-download { opacity: 1; }
    }

    .channel-skeleton {
        display: flex;
        flex-direction: column;
        gap: 2px;
    }

    .channel-skeleton-row {
        display: flex;
        align-items: center;
        gap: 12px;
        min-height: 60px;
        padding: 10px 14px;
    }

    .channel-skeleton-row .file-state-skeleton-icon {
        flex: 0 0 auto;
        width: 40px;
        height: 40px;
        border-radius: var(--radius-sm);
    }

    .channel-skeleton-text {
        flex: 1 1 auto;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 9px;
    }

    .channel-list-note {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: var(--space-2);
        padding: var(--space-5) var(--space-4);
        color: var(--text-muted);
        font-size: var(--type-sm);
        text-align: center;
    }

    .channel-spinner {
        width: 14px;
        height: 14px;
        border: 2px solid var(--border);
        border-top-color: var(--accent);
        border-radius: var(--radius-pill);
        animation: channel-spin 720ms linear infinite;
    }

    .channel-sentinel { height: 1px; }

    @keyframes channel-spin {
        to { transform: rotate(360deg); }
    }

    /* ---- Desktop grid. Posts sit four to a row as square tiles, with the
       name and meta under each, as the design draws the source. Scoped to the
       desktop view so the phone rows above keep their list. The tile defaults
       use :where so the hover that turns a tile's chip into its action still
       wins. ---- */
    .channel-view.is-grid .channel-rows {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
        justify-content: start;
        gap: var(--space-3);
    }

    /* Wide windows gain columns instead of bigger tiles. */
    .channel-view.is-grid .channel-row {
        flex-direction: column;
        align-items: stretch;
        min-width: 0;
        max-width: 220px;
    }

    .channel-view.is-grid .channel-row:hover,
    .channel-view.is-grid .channel-row:focus-within { background: transparent; }

    .channel-view.is-grid .channel-row-main {
        position: relative;
        flex-direction: column;
        align-items: stretch;
        gap: var(--space-2);
        width: 100%;
        padding: 0;
        border-radius: var(--radius-sm);
    }

    .channel-view:where(.is-grid) .channel-chip {
        width: 100%;
        height: auto;
        min-width: 0;
        min-height: 0;
        max-width: none;
        max-height: none;
        margin: 0;
        aspect-ratio: 1;
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        background: var(--color-surface-2);
        color: var(--text-muted);
    }

    .channel-view.is-grid .channel-row-text {
        padding: 0 2px;
        gap: 1px;
    }

    /* A caption keeps to two lines; anything longer ends in an ellipsis. */
    .channel-view.is-grid .channel-row-title {
        overflow: hidden;
        font-size: var(--type-sm);
        line-height: 1.3;
        white-space: normal;
        display: -webkit-box;
        -webkit-line-clamp: 2;
        line-clamp: 2;
        -webkit-box-orient: vertical;
    }

    .channel-view.is-grid .channel-row-meta { font-size: var(--type-xs); }

    .channel-view.is-grid .channel-row-badge {
        position: absolute;
        top: 6px;
        left: 6px;
        background: var(--color-surface-0);
    }

    .channel-view.is-grid .channel-row-telegram {
        position: absolute;
        top: 6px;
        right: 44px;
        width: var(--control-md);
        height: var(--control-md);
        margin: 0;
        background: var(--color-surface-0);
    }

    .channel-view.is-grid .channel-row-download {
        position: absolute;
        top: 6px;
        right: 6px;
        width: var(--control-md);
        height: var(--control-md);
        margin: 0;
        background: var(--color-surface-0);
    }

    .channel-view.is-grid .channel-skeleton {
        display: grid;
        grid-template-columns: repeat(4, minmax(0, 1fr));
        gap: var(--space-3);
    }

    .channel-view.is-grid .channel-skeleton-row {
        flex-direction: column;
        align-items: stretch;
        min-height: 0;
        padding: 0;
        gap: var(--space-2);
    }

    .channel-view.is-grid .channel-skeleton-row .file-state-skeleton-icon {
        width: 100%;
        height: auto;
        aspect-ratio: 1;
        border-radius: var(--radius-sm);
    }

    /* A photo or video post shows its preview in the type slot. The type icon
       stays for every other post, and the hover action sits over either. */
    .channel-chip.has-preview {
        overflow: hidden;
        background: var(--color-surface-2);
    }

    .channel-chip-preview {
        position: absolute;
        inset: 0;
        width: 100%;
        height: 100%;
        max-width: none;
        max-height: none;
        object-fit: cover;
        border-radius: inherit;
        pointer-events: none;
    }

    .channel-chip.has-preview .chip-action {
        background: color-mix(in srgb, #000 42%, transparent);
        color: #fff;
    }

    /* ---- Desktop list. A table of rows 36px tall: the type or preview slot,
       the name, then kind, size and date. Clicking a row opens it as a tile
       does. ---- */
    .channel-view.is-list .channel-row { min-height: 36px; border-radius: var(--radius-sm); }

    .channel-view.is-list .channel-row-main {
        min-height: 36px;
        height: 36px;
        gap: var(--space-3);
        padding: 0 var(--space-3);
    }

    .channel-view.is-list .channel-chip {
        width: var(--row-visual-size);
        height: var(--row-visual-size);
        min-width: var(--row-visual-size);
        min-height: var(--row-visual-size);
        max-width: var(--row-visual-size);
        max-height: var(--row-visual-size);
        margin: 0;
        border-radius: var(--radius-sm);
    }

    /* A bare glyph has no tile: it keeps no fill or border, and on hover the
       list shows its action in the accent colour. A preview keeps its own box. */
    .channel-view.is-list .channel-chip:not(.has-preview) {
        border-color: transparent;
        background: none;
    }

    .channel-view.is-list .channel-row:hover .channel-chip:not(.has-preview),
    .channel-view.is-list .channel-row.is-locked:hover .channel-chip:not(.has-preview) {
        color: var(--accent);
    }

    .channel-view.is-list .channel-row-text {
        flex: 1 1 auto;
        min-width: 0;
        flex-direction: row;
        align-items: center;
    }

    .channel-view.is-list .channel-row-title {
        font-size: var(--type-sm);
        line-height: 1.3;
    }

    .channel-view.is-list .channel-row-meta { display: none; }

    .channel-view.is-list .channel-row-kind,
    .channel-view.is-list .channel-row-size,
    .channel-view.is-list .channel-row-date {
        flex: 0 0 auto;
        overflow: hidden;
        color: var(--text-muted);
        font-size: var(--type-xs);
        font-variant-numeric: tabular-nums;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .channel-view.is-list .channel-row-kind { width: 88px; }
    .channel-view.is-list .channel-row-size { width: 84px; text-align: right; }
    .channel-view.is-list .channel-row-date { width: 120px; text-align: right; }

    .channel-view.is-list .channel-row-badge { position: static; flex: 0 0 auto; margin-left: var(--space-2); }

    /* The Save and Telegram buttons float over a reserved strip, so every
       row's columns line up whether or not it has either button. */
    .channel-view.is-list .channel-row-main { padding-right: 84px; }

    .channel-view.is-list .channel-row-download {
        position: absolute;
        top: 50%;
        right: 10px;
        width: 32px;
        height: 32px;
        margin: 0;
        transform: translateY(-50%);
    }

    .channel-view.is-list .channel-row-telegram {
        position: absolute;
        top: 50%;
        right: 46px;
        width: 28px;
        height: 28px;
        margin: 0;
        transform: translateY(-50%);
    }

    .channel-view.is-list .channel-skeleton-row {
        min-height: 36px;
        padding: 0 var(--space-3);
    }

    /* ---- Phone. The shell's top bar steps aside for this one, which restates
       it: same fill to the glass, same insets, a hairline only once rows pass
       under it. The rows become the inset card the file list draws. ---- */
    .channel-topbar {
        flex: 0 0 auto;
        padding: var(--inset-top) var(--inset-right) 0 var(--inset-left);
        border-bottom: 1px solid transparent;
        background: var(--color-surface-0);
        transition: border-color var(--motion-med) var(--ease-standard);
    }

    .channel-topbar.is-scrolled { border-bottom-color: var(--color-border-soft); }
    .channel-topbar .topbar-folder-meta { display: block; }

    .channel-topbar-kinds { padding: 0 var(--space-4) var(--space-3); }
    .channel-topbar-kinds .channel-kinds { display: flex; }
    .channel-topbar-kinds .channel-kinds button {
        flex: 1 0 auto;
        min-height: var(--touch-target);
        font-size: var(--mobile-type-meta);
    }

    .is-mobile .channel-scroll {
        /* The search field opens into this list, so its bottom clears the
           keyboard as well as the tab bar; otherwise the last posts sit under
           the keys while the user is typing to filter them. */
        padding: var(--space-2) 0 calc(var(--space-6) + var(--inset-keyboard));
        scrollbar-width: none;
        overscroll-behavior-y: contain;
        -webkit-overflow-scrolling: touch;
    }

    /* Landscape puts the notch or the navigation bar on a side, so the rows
       stand off it like the rest of the shell rather than running under it. */
    .is-mobile .channel-rows {
        gap: 0;
        margin: 0 calc(var(--space-3) + var(--inset-right)) 0 calc(var(--space-3) + var(--inset-left));
    }

    .is-mobile .channel-row {
        border-radius: 0;
        background: var(--color-surface-0);
        transition: background-color 80ms var(--ease-standard);
    }

    .is-mobile .channel-row:first-child { border-radius: var(--radius-xl) var(--radius-xl) 0 0; }
    .is-mobile .channel-row:last-child { border-bottom-left-radius: var(--radius-xl); border-bottom-right-radius: var(--radius-xl); }
    .is-mobile .channel-row:only-child { border-radius: var(--radius-xl); }
    .is-mobile .channel-row:active { background: var(--color-surface-2); }

    .is-mobile .channel-row + .channel-row::before {
        position: absolute;
        top: 0;
        left: 68px;
        right: 0;
        height: 1px;
        background: var(--color-border-soft);
        content: '';
    }

    .is-mobile .channel-row-main {
        min-height: 68px;
        padding: 10px 14px 10px 12px;
        -webkit-tap-highlight-color: transparent;
        -webkit-touch-callout: none;
        touch-action: manipulation;
        user-select: none;
    }

    .is-mobile .channel-chip {
        width: 44px;
        height: 44px;
        border-color: transparent;
        border-radius: var(--radius-lg);
        background: none;
        color: var(--color-text-muted);
    }

    .is-mobile .channel-chip.has-preview { background: var(--color-surface-2); }

    .is-mobile .channel-row-title { font-size: var(--mobile-type-body); }
    .is-mobile .channel-row-meta { font-size: var(--mobile-type-meta); }

    /* The row drops its own right padding so the "…" lands where the padding
       was, keeping a 44px target without widening the row. */
    .is-mobile .channel-row-main { padding-right: 2px; }
    .is-mobile .channel-row-actions {
        flex: 0 0 auto;
        display: grid;
        place-items: center;
        width: 44px;
        height: 44px;
        margin-right: var(--space-1);
        border: 0;
        border-radius: var(--radius-md);
        background: transparent;
        color: var(--color-text-muted);
        cursor: pointer;
        -webkit-tap-highlight-color: transparent;
        touch-action: manipulation;
    }
    .is-mobile .channel-row-actions:active { background: var(--color-surface-2); color: var(--color-text); }
    .is-mobile .channel-row-actions:focus-visible { outline: none; box-shadow: var(--focus-ring); }

    .is-mobile .channel-skeleton {
        margin: 0 calc(var(--space-3) + var(--inset-right)) 0 calc(var(--space-3) + var(--inset-left));
    }
    .is-mobile .channel-list-note { font-size: var(--mobile-type-meta); flex-wrap: wrap; }
    /* Retry / Try again / Load older posts are the one way on from a stuck list,
       so on a phone each is a 44px target instead of a zero-padding text link. */
    .is-mobile .channel-list-note .link-button {
        display: inline-flex;
        align-items: center;
        min-height: var(--touch-target);
        padding: 0 var(--space-2);
    }

    @media (prefers-reduced-motion: reduce) {
        .channel-topbar,
        .channel-row,
        .channel-chip,
        .chip-layer,
        .channel-row-telegram { transition: none; }

        .channel-spinner { animation: none; }
    }
</style>
