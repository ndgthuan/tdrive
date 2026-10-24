<script lang="ts">
    import { untrack } from 'svelte';
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import PlusIcon from 'phosphor-svelte/lib/PlusIcon';
    import SearchIcon from 'phosphor-svelte/lib/MagnifyingGlassIcon';
    import LinkIcon from 'phosphor-svelte/lib/LinkIcon';
    import ModalShell from '../modals/modal_shell.svelte';
    import { toAppError } from '../../modules/errors';
    import ChannelAvatar from './channel_avatar.svelte';
    import { sourceHandle, sourceKey, sourcePeerLabel, type ChannelSource } from './channel_model';
    import { t, t_store } from '../../i18n/i18n';

    interface Props {
        open: boolean;
        /** Joined direct messages, bots, groups and channels, including archived peers. */
        loadCandidates: () => Promise<ChannelSource[]>;
        /** Checks a public @username or t.me link without joining Telegram. */
        resolvePublic: (input: string) => Promise<ChannelSource>;
        /**
         * The channels in TDrive now, as the sidebar lists them. They decide
         * what reads as added, since a list kept from an earlier open may not.
         */
        added: readonly ChannelSource[];
        loadPhoto: (source: ChannelSource) => Promise<string>;
        onAdd: (source: ChannelSource) => Promise<void>;
        onOpen: (source: ChannelSource) => void;
        onClose: () => void;
    }

    let { open, loadCandidates, resolvePublic, added, loadPhoto, onAdd, onOpen, onClose }: Props = $props();

    type Candidates =
        | { status: 'loading' }
        | { status: 'ready'; sources: readonly ChannelSource[] }
        | { status: 'error'; error: unknown };

    let candidates = $state<Candidates>({ status: 'loading' });
    let query = $state('');
    let adding = $state<string | null>(null);
    let addError = $state('');
    let publicInput = $state('');
    let publicSource = $state<ChannelSource | null>(null);
    let publicError = $state('');
    let resolvingPublic = $state(false);
    let loadVersion = 0;
    let publicVersion = 0;

    const addedIds = $derived(new Set(added.map(sourceKey)));
    const isAdded = (source: ChannelSource) => addedIds.has(sourceKey(source));
    const candidatesTruncated = $derived(candidates.status === 'ready' && candidates.sources.some((source) => source.candidatesTruncated));
    const canResolvePublic = $derived(publicInput.trim() !== '' && !resolvingPublic && adding === null);

    // Channels still to add lead; the ones already in TDrive sit at the end.
    const visible = $derived.by(() => {
        if (candidates.status !== 'ready') return [];
        const needle = query.trim().toLocaleLowerCase();
        const matches = needle
            ? candidates.sources.filter((source) => source.title.toLocaleLowerCase().includes(needle)
                || source.username.toLocaleLowerCase().includes(needle)
                || sourcePeerLabel(source).toLocaleLowerCase().includes(needle))
            : candidates.sources;
        return [...matches.filter((source) => !isAdded(source)), ...matches.filter(isAdded)];
    });

    // Listing joined channels walks every Telegram dialog, so it happens only
    // while the picker is open, never on the way into a channel. Opened again,
    // the picker shows the last list until the fresh one lands.
    async function load(): Promise<void> {
        const version = ++loadVersion;
        if (candidates.status !== 'ready') candidates = { status: 'loading' };
        try {
            const sources = await loadCandidates();
            if (version === loadVersion) candidates = { status: 'ready', sources };
        } catch (error) {
            if (version === loadVersion && candidates.status !== 'ready') candidates = { status: 'error', error };
        }
    }

    $effect(() => {
        if (!open) return;
        query = '';
        adding = null;
        addError = '';
        publicInput = '';
        publicSource = null;
        publicError = '';
        publicVersion += 1;
        // Only opening reloads. load() reads the list it replaces, which
        // would otherwise make every answer reload again.
        untrack(() => void load());
        return () => {
            loadVersion += 1;
        };
    });

    function close(): void {
        if (adding === null) onClose();
    }

    async function choose(source: ChannelSource): Promise<void> {
        if (adding !== null) return;
        if (isAdded(source)) {
            onOpen(source);
            onClose();
            return;
        }
        if (!source.available) {
            addError = t('sources.unavailable_account', { title: source.title });
            return;
        }
        adding = sourceKey(source);
        addError = '';
        try {
            await onAdd(source);
            adding = null;
            onClose();
        } catch (error) {
            adding = null;
            addError = t('sources.add_failed', { title: source.title, message: toAppError(error, { source: 'backend' }).message });
        }
    }

    function onSearchKeydown(event: KeyboardEvent): void {
        if (event.key !== 'Enter' || visible.length !== 1) return;
        event.preventDefault();
        void choose(visible[0]);
    }

    function handle(source: ChannelSource): string {
        const details = [sourceHandle(source), sourcePeerLabel(source)];
        if (source.protected) details.push($t_store('sources.restriction_protected'));
        if (!source.available) details.push($t_store('sources.unavailable'));
        return details.filter((value, index, values) => values.indexOf(value) === index).join(' · ');
    }

    async function resolveLink(): Promise<void> {
        if (!canResolvePublic) return;
        const input = publicInput.trim();
        const version = ++publicVersion;
        resolvingPublic = true;
        publicError = '';
        publicSource = null;
        try {
            const resolved = await resolvePublic(input);
            if (resolved.peerKind !== 'channel') throw new Error(t('sources.not_public_channel'));
            if (version === publicVersion) publicSource = { ...resolved, publicUsername: resolved.username };
        } catch (error) {
            if (version === publicVersion) publicError = toAppError(error, { source: 'backend' }).message;
        } finally {
            if (version === publicVersion) resolvingPublic = false;
        }
    }

    function onPublicInput(): void {
        publicVersion += 1;
        publicSource = null;
        publicError = '';
        resolvingPublic = false;
    }

    function onPublicKeydown(event: KeyboardEvent): void {
        if (event.key !== 'Enter') return;
        event.preventDefault();
        void resolveLink();
    }
</script>

<ModalShell
    hostId="channel-picker-modal"
    {open}
    title={$t_store('sources.add_source')}
    titleId="channel-picker-title"
    subtitle={$t_store('sources.picker_subtitle')}
    cardClass="channel-picker-card"
    initialFocus="#channel-picker-search"
    onClose={close}
>
    <label class="search-field channel-picker-search">
        <SearchIcon size={16} aria-hidden="true" />
        <input
            id="channel-picker-search"
            bind:value={query}
            onkeydown={onSearchKeydown}
            type="text"
            placeholder={$t_store('sources.picker_search_placeholder')}
            autocomplete="off"
            spellcheck="false"
            aria-label={$t_store('sources.search_sources')}
            aria-controls="channel-picker-list"
        />
    </label>

    <div class="channel-picker-public">
        <label for="channel-public-link">{$t_store('sources.public_link_label')}</label>
        <div class="channel-picker-public-row">
            <div class="channel-picker-public-field" class:has-error={publicError !== ''}>
                <LinkIcon size={16} aria-hidden="true" />
                <input
                    id="channel-public-link"
                    bind:value={publicInput}
                    oninput={onPublicInput}
                    onkeydown={onPublicKeydown}
                    type="text"
                    inputmode="url"
                    autocomplete="off"
                    spellcheck="false"
                    placeholder={$t_store('sources.public_link_placeholder')}
                    aria-invalid={publicError ? 'true' : undefined}
                    aria-describedby={publicError ? 'channel-public-link-help channel-public-link-error' : 'channel-public-link-help'}
                />
            </div>
            <button class="secondary-btn channel-picker-check" type="button" disabled={!canResolvePublic} aria-busy={resolvingPublic} onclick={() => void resolveLink()}>
                {resolvingPublic ? $t_store('sources.checking') : $t_store('sources.check_link')}
            </button>
        </div>
        <p id="channel-public-link-help" class="channel-picker-public-help">{$t_store('sources.public_help')}</p>
        {#if publicError}<p id="channel-public-link-error" class="channel-picker-error" role="alert">{publicError}</p>{/if}
        {#if publicSource}
            {@const publicAdded = isAdded(publicSource)}
            {@const publicBusy = adding === sourceKey(publicSource)}
            <button
                class="channel-picker-public-result"
                type="button"
                disabled={!publicSource.available || (adding !== null && !publicBusy)}
                aria-busy={publicBusy}
                aria-label={publicBusy ? $t_store('sources.connecting', { title: publicSource.title }) : publicAdded ? $t_store('sources.open_connected', { title: publicSource.title }) : publicSource.available ? $t_store('sources.connect', { title: publicSource.title }) : $t_store('sources.title_unavailable', { title: publicSource.title })}
                onclick={() => { if (publicSource) void choose(publicSource); }}
            >
                <ChannelAvatar source={publicSource} {loadPhoto} />
                <span class="channel-picker-text">
                    <span class="channel-picker-name">{publicSource.title}</span>
                    <span class="channel-picker-handle">{handle(publicSource)}</span>
                </span>
                <span class="channel-picker-trail" aria-hidden="true">
                    {#if publicBusy}
                        <span class="channel-picker-spinner"></span>
                    {:else}
                        {publicAdded ? $t_store('sources.open') : publicSource.available ? $t_store('sources.connect_short') : $t_store('sources.unavailable')}
                    {/if}
                </span>
            </button>
        {/if}
    </div>

    {#if addError}
        <p class="channel-picker-error" role="alert">{addError}</p>
    {/if}

    <div id="channel-picker-list" class="channel-picker-list" aria-busy={candidates.status === 'loading'}>
        {#if candidates.status === 'loading'}
            {#each [58, 42, 66, 36, 50] as width, index (index)}
                <div class="channel-picker-row is-placeholder" aria-hidden="true">
                    <span class="channel-picker-avatar file-state-skeleton-icon"></span>
                    <span class="channel-picker-text">
                        <span class="file-state-skeleton-bar" style:--skeleton-width={`${width}%`}></span>
                        <span class="file-state-skeleton-bar" style:--skeleton-width="28%"></span>
                    </span>
                </div>
            {/each}
        {:else if candidates.status === 'error'}
            <div class="channel-picker-empty" role="alert">
                {toAppError(candidates.error, { source: 'backend' }).message}
                <button class="link-button" type="button" onclick={() => void load()}>{$t_store('sources.try_again')}</button>
            </div>
        {:else if candidates.sources.length === 0}
            <div class="channel-picker-empty">{$t_store('sources.picker_empty')}</div>
        {:else if visible.length === 0}
            <div class="channel-picker-empty">{$t_store('sources.picker_no_match', { query: query.trim() })}</div>
        {:else}
            {#each visible as source (sourceKey(source))}
                {@const busy = adding === sourceKey(source)}
                {@const inTDrive = isAdded(source)}
                <button
                    class="channel-picker-row"
                    class:is-added={inTDrive}
                    type="button"
                    disabled={!source.available || (adding !== null && !busy)}
                    aria-busy={busy}
                    aria-label={inTDrive ? $t_store('sources.open_connected', { title: source.title }) : source.available ? $t_store('sources.connect', { title: source.title }) : $t_store('sources.title_unavailable', { title: source.title })}
                    onclick={() => void choose(source)}
                >
                    <ChannelAvatar {source} {loadPhoto} />
                    <span class="channel-picker-text">
                        <span class="channel-picker-name">{source.title}</span>
                        <span class="channel-picker-handle">{handle(source)}</span>
                    </span>
                    <span class="channel-picker-trail" class:is-added={inTDrive} aria-hidden="true">
                        {#if busy}
                            <span class="channel-picker-spinner"></span>
                        {:else if inTDrive}
                            <CheckIcon size={14} weight="bold" />{$t_store('sources.connected')}
                        {:else if !source.available}
                            {$t_store('sources.unavailable')}
                        {:else}
                            <span class="channel-picker-plus"><PlusIcon size={16} /></span>
                        {/if}
                    </span>
                </button>
            {/each}
        {/if}
    </div>

    {#if candidatesTruncated}
        <p class="channel-picker-list-note">{$t_store('sources.truncated_note')}</p>
    {/if}

    {#snippet actions()}
        <button class="secondary-btn" type="button" disabled={adding !== null} onclick={close}>{$t_store('sources.cancel')}</button>
    {/snippet}
</ModalShell>

<style>
    /* The add-source sheet as the design draws it: a surface-2 card with 24px
       padding, 32px fields and buttons, and 36px rows with 32px avatars. The
       phone sheet keeps its own layout (see the html.mobile rules below). */
    :global(.channel-picker-card:not(.modal-sheet)) {
        max-width: 470px;
        padding: var(--space-6);
        border-color: var(--color-border-soft);
        border-radius: var(--radius-lg);
        background: var(--color-surface-2);
    }

    :global(.channel-picker-card:not(.modal-sheet) > .modal-title) {
        margin-bottom: 0;
        font-size: var(--type-xl);
        letter-spacing: -0.01em;
    }

    :global(.channel-picker-card:not(.modal-sheet) > .modal-subtitle) {
        margin: var(--space-1) 0 0;
        font-size: var(--type-sm);
        line-height: 1.5;
    }

    .channel-picker-search {
        height: var(--control-md);
        margin-top: var(--space-4);
        padding: 0 var(--space-3);
        border: 1px solid var(--color-border);
        border-radius: var(--radius-sm);
        background: var(--color-surface-1);
    }

    .channel-picker-search input,
    .channel-picker-search input:focus { font-size: var(--type-base); }

    .channel-picker-error {
        margin-top: var(--space-3);
        color: var(--danger);
        font-size: var(--type-sm);
        line-height: 1.4;
    }

    .channel-picker-public {
        margin-top: var(--space-4);
    }

    .channel-picker-public > label {
        display: block;
        margin-bottom: var(--space-1);
        color: var(--text-muted);
        font-size: var(--type-xs);
        font-weight: var(--weight-medium);
    }

    .channel-picker-public-row {
        display: grid;
        grid-template-columns: minmax(0, 1fr) auto;
        gap: var(--space-2);
    }

    .channel-picker-public-field {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-width: 0;
        height: var(--control-md);
        padding: 0 var(--space-3);
        border: 1px solid var(--color-border);
        border-radius: var(--radius-sm);
        background: var(--color-surface-1);
        color: var(--text-muted);
    }

    .channel-picker-public-field:focus-within { border-color: var(--accent); box-shadow: var(--focus-ring); }
    .channel-picker-public-field.has-error { border-color: var(--danger); }
    .channel-picker-public-field input { min-width: 0; width: 100%; height: 100%; margin: 0; padding: 0; border: 0; border-radius: 0; outline: 0; box-shadow: none; background: transparent; color: var(--text-main); font: inherit; font-size: var(--type-base); }
    .channel-picker-public-field input::placeholder { color: var(--text-muted); }
    .channel-picker-check { min-height: var(--control-md); height: var(--control-md); padding: 0 var(--space-3); white-space: nowrap; font-size: var(--type-sm); }
    .channel-picker-check:disabled { color: var(--text-muted); cursor: default; opacity: 0.8; }
    .channel-picker-check:hover:disabled { background: transparent; }
    .channel-picker-check:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); }
    .channel-picker-check:focus-visible { outline: none; box-shadow: var(--focus-ring); }
    .channel-picker-public-help { margin: var(--space-2) 0 0; color: var(--text-muted); font-size: var(--type-xs); line-height: 1.4; }

    .channel-picker-public-result {
        --avatar-size: 32px;
        display: flex;
        width: 100%;
        min-height: 36px;
        align-items: center;
        gap: var(--space-3);
        margin-top: var(--space-2);
        padding: 2px var(--space-2);
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-main);
        font: inherit;
        text-align: left;
        cursor: pointer;
    }
    .channel-picker-public-result:hover:not(:disabled) { background: var(--color-surface-3); }
    .channel-picker-public-result:focus-visible { outline: none; box-shadow: var(--focus-ring); }
    .channel-picker-public-result:disabled { cursor: default; opacity: 0.5; }
    .channel-picker-public-result[aria-busy='true'] { opacity: 1; }

    .channel-picker-list {
        display: flex;
        flex-direction: column;
        gap: 0;
        /* Sized to its rows, with a floor so the sheet does not jump while the list loads. */
        min-height: 144px;
        max-height: min(288px, 42vh);
        margin: var(--space-3) calc(-1 * var(--space-2)) var(--space-4);
        padding: 0 var(--space-2);
        overflow-y: auto;
    }

    .channel-picker-row {
        --avatar-size: 32px;
        flex: 0 0 auto;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 36px;
        padding: 2px var(--space-2);
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-main);
        font: inherit;
        text-align: left;
        cursor: pointer;
        transition: background-color var(--motion-fast) var(--ease-standard);
    }

    .channel-picker-row:hover:not(:disabled) { background: var(--color-surface-3); }
    .channel-picker-row.is-added,
    .channel-picker-row.is-added:hover:not(:disabled) { background: var(--overlay-accent-1); }
    .channel-picker-row:focus-visible { outline: none; box-shadow: var(--focus-ring); }
    .channel-picker-row:disabled { cursor: default; opacity: 0.5; }
    .channel-picker-row[aria-busy='true'] { opacity: 1; }
    .channel-picker-row.is-placeholder { cursor: default; }

    /* The loading rows hold the avatar's place. */
    .channel-picker-avatar {
        flex: 0 0 auto;
        width: 32px;
        height: 32px;
        border-radius: var(--radius-pill);
    }

    .channel-picker-text {
        flex: 1 1 auto;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 1px;
        line-height: 1.2;
    }

    .is-placeholder .channel-picker-text { gap: 6px; }

    .channel-picker-name,
    .channel-picker-handle {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .channel-picker-name {
        color: var(--color-text);
        font-size: var(--type-base);
        font-weight: var(--weight-semibold);
    }

    .channel-picker-handle {
        color: var(--text-muted);
        font-size: var(--type-xs);
    }

    .channel-picker-trail {
        flex: 0 0 auto;
        display: inline-flex;
        align-items: center;
        gap: 4px;
        color: var(--text-muted);
        font-size: var(--type-xs);
        font-weight: var(--weight-semibold);
    }

    .channel-picker-trail.is-added { color: var(--color-accent-text); }

    /* The add control is a 32px square, as the design draws it. */
    .channel-picker-plus {
        display: grid;
        width: var(--control-md);
        height: var(--control-md);
        place-items: center;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        color: var(--text-muted);
        transition:
            background-color var(--motion-fast) var(--ease-standard),
            border-color var(--motion-fast) var(--ease-standard),
            color var(--motion-fast) var(--ease-standard);
    }

    .channel-picker-row:hover .channel-picker-plus,
    .channel-picker-row:focus-visible .channel-picker-plus {
        border-color: var(--color-border);
        background: var(--color-surface-3);
        color: var(--color-text);
    }

    .channel-picker-spinner {
        width: 16px;
        height: 16px;
        border: 2px solid var(--border);
        border-top-color: var(--accent);
        border-radius: var(--radius-pill);
        animation: channel-picker-spin 720ms linear infinite;
    }

    .channel-picker-empty {
        padding: var(--space-6) var(--space-4);
        color: var(--text-muted);
        font-size: var(--type-sm);
        line-height: 1.5;
        text-align: center;
        text-wrap: pretty;
    }

    .channel-picker-list-note {
        margin: calc(-1 * var(--space-2)) var(--space-2) var(--space-3);
        color: var(--text-muted);
        font-size: var(--type-xs);
        line-height: 1.4;
    }

    @keyframes channel-picker-spin {
        to { transform: rotate(360deg); }
    }

    /* The sheet scrolls its body itself and sizes it to the screen. */
    :global(html.mobile) .channel-picker-list {
        height: auto;
        min-height: 280px;
        margin-bottom: 0;
        overflow: visible;
    }

    :global(html.mobile) .channel-picker-search input,
    :global(html.mobile) .channel-picker-public-field input { font-size: max(16px, var(--type-base)); }
    :global(html.mobile) .channel-picker-public-row { grid-template-columns: minmax(0, 1fr); }
    :global(html.mobile) .channel-picker-check { width: 100%; }
    :global(html.mobile) .channel-picker-row,
    :global(html.mobile) .channel-picker-public-result { min-height: 60px; }

    @media (prefers-reduced-motion: reduce) {
        .channel-picker-row,
        .channel-picker-plus { transition: none; }

        .channel-picker-spinner { animation: none; }
    }
</style>
