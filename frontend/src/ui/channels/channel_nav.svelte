<script lang="ts">
    import EllipsisIcon from 'phosphor-svelte/lib/DotsThreeIcon';
    import PlusIcon from 'phosphor-svelte/lib/PlusIcon';
    import ChannelAvatar from './channel_avatar.svelte';
    import { sourceKey, sourcePeerLabel, type ChannelSource } from './channel_model';
    import { current_locale, t_store } from '../../i18n/i18n';
    import type { ChannelSourcesState } from './channel_store';

    interface Props {
        sources: ChannelSourcesState;
        activeKey: string | null;
        loadPhoto: (source: ChannelSource) => Promise<string>;
        onSelect: (source: ChannelSource) => void;
        onActions: (x: number, y: number, source: ChannelSource) => void;
        onAdd: () => void;
        onRetry: () => void;
    }

    let { sources, activeKey, loadPhoto, onSelect, onActions, onAdd, onRetry }: Props = $props();

    function pointBelow(element: HTMLElement): { x: number; y: number } {
        const rect = element.getBoundingClientRect();
        return { x: rect.left, y: rect.bottom + 4 };
    }

    function openActions(event: MouseEvent, source: ChannelSource): void {
        event.preventDefault();
        // A row click would both open a channel and its menu; the "…" is a
        // sibling of the row button, so stop it reaching the row.
        event.stopPropagation();
        const point = event.clientX || event.clientY
            ? { x: event.clientX, y: event.clientY }
            : pointBelow(event.currentTarget as HTMLElement);
        onActions(point.x, point.y, source);
    }

    // Desktop keeps no visible "…" on channel rows (owner's decision), so the
    // keyboard reaches the same actions the way it reaches any context menu:
    // the Menu key, or Shift+F10, on the focused row.
    function onRowKeydown(event: KeyboardEvent, source: ChannelSource): void {
        if (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10')) return;
        event.preventDefault();
        const point = pointBelow(event.currentTarget as HTMLElement);
        onActions(point.x, point.y, source);
    }
</script>

<div class="channel-nav-title">
    <span>{$t_store('sources.nav_title')}</span>
    <button class="channel-nav-add" type="button" title={$t_store('sources.add_source')} aria-label={$t_store('sources.add_source')} onclick={onAdd}>
        <PlusIcon size={14} aria-hidden="true" weight="bold" />
    </button>
</div>

{#key $current_locale}
<div class="drives-list">
    {#each sources.sources as source (sourceKey(source))}
        {@const active = sourceKey(source) === activeKey}
        <div class="channel-nav-row" role="group" aria-label={source.title}>
            <button
                class="drive-item channel-nav-item"
                class:active
                type="button"
                title={`${source.title} (${sourcePeerLabel(source)})`}
                aria-current={active ? 'page' : undefined}
                onclick={() => onSelect(source)}
                oncontextmenu={(event) => openActions(event, source)}
                onkeydown={(event) => onRowKeydown(event, source)}
            >
                <ChannelAvatar {source} {loadPhoto} />
                <span class="drive-item-title">{source.title}</span>
            </button>
            <!-- Visible only in the phone drive sheet, where a long press has no
                 counterpart and iOS fires no contextmenu; the desktop sidebar
                 hides it and uses right-click or the keyboard instead. -->
            <button
                class="channel-nav-actions"
                type="button"
                aria-haspopup="menu"
                aria-label={$t_store('sources.actions_for', { title: source.title })}
                onclick={(event) => openActions(event, source)}
            >
                <EllipsisIcon size={18} aria-hidden="true" />
            </button>
        </div>
    {:else}
        {#if sources.status === 'error'}
            <div class="drive-empty channel-nav-error">
                {$t_store('sources.nav_error')}
                <button class="link-button" type="button" onclick={onRetry}>{$t_store('sources.retry')}</button>
            </div>
        {/if}
    {/each}
</div>
{/key}

<style>
    /* A caption like the Share one above it: small, uppercase, in the subtle
       text step. The add button sits at its right edge. */
    .channel-nav-title {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
        height: 24px;
        padding: 0 4px 0 8px;
        margin-bottom: 4px;
        color: var(--color-text-subtle);
        font-size: var(--type-xs);
        font-weight: var(--weight-semibold);
        letter-spacing: 0.08em;
        text-transform: uppercase;
    }

    .channel-nav-add {
        display: inline-grid;
        width: 22px;
        height: 22px;
        place-items: center;
        border: 0;
        border-radius: var(--radius-sm);
        color: var(--text-muted);
        background: transparent;
        cursor: pointer;
        transition:
            background-color var(--motion-fast) var(--ease-standard),
            color var(--motion-fast) var(--ease-standard);
    }

    .channel-nav-add:hover {
        color: var(--text-main);
        background: var(--bg-panel);
    }

    .channel-nav-add:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    /* Row and its trailing "…". On desktop the row is the only column and the
       action button is hidden; the phone sheet adds the button's column. */
    .channel-nav-row {
        display: grid;
        grid-template-columns: minmax(0, 1fr);
        align-items: center;
        min-width: 0;
    }
    .channel-nav-row > .drive-item { min-width: 0; }
    .channel-nav-actions { display: none; }

    /* The channel's own picture takes the drive row's icon column. The gap
       gives back the 2px the avatar is wider than an icon, so channel names
       start where drive names do. */
    .channel-nav-item {
        --avatar-size: 20px;
        gap: 10px;
    }

    .channel-nav-item:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    .channel-nav-error {
        display: flex;
        align-items: baseline;
        gap: var(--space-2);
        opacity: 1;
    }

    /* In the drive switcher the rows are touch-sized by the sheet, and the
       title row reads as the group's label with the add action beside it. */
    /* The 6px on the right is the add button's enlarged hit area: inside the
       row it lines the glyph up with the drive rows' action buttons, and it
       keeps the switcher from scrolling sideways. */
    :global(html.mobile) .channel-nav-title {
        padding: var(--space-4) 6px var(--space-1) 12px;
        font-size: var(--mobile-type-meta);
        height: auto;
        margin-bottom: 0;
        letter-spacing: 0;
        text-transform: none;
        color: var(--text-muted);
    }

    :global(html.mobile) .channel-nav-add {
        position: relative;
        width: 32px;
        height: 32px;
    }

    :global(html.mobile) .channel-nav-add::after {
        position: absolute;
        inset: -6px;
        content: '';
    }

    :global(html.mobile) .channel-nav-item {
        --avatar-size: 28px;
        gap: 12px;
    }

    /* The sheet gives each channel row the visible "…" the shared drives have,
       on a 44px target. */
    :global(html.mobile) .channel-nav-row {
        grid-template-columns: minmax(0, 1fr) var(--touch-target);
        width: 100%;
    }
    :global(html.mobile) .channel-nav-actions {
        display: inline-grid;
        place-items: center;
        width: var(--touch-target);
        height: var(--touch-target);
        border: 0;
        border-radius: var(--radius-sm);
        color: var(--text-muted);
        background: transparent;
        cursor: pointer;
    }
    :global(html.mobile) .channel-nav-actions:active {
        background: var(--color-surface-2);
        color: var(--text-main);
    }
    :global(html.mobile) .channel-nav-actions:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    @media (prefers-reduced-motion: reduce) {
        .channel-nav-add {
            transition: none;
        }
    }
</style>
