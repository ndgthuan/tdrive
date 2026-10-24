<script lang="ts">
    import { tick } from 'svelte';
    import FileIcon from 'phosphor-svelte/lib/FileIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import EyeIcon from 'phosphor-svelte/lib/EyeIcon';
    import ExternalLinkIcon from 'phosphor-svelte/lib/ArrowSquareOutIcon';
    import PlayIcon from 'phosphor-svelte/lib/PlayIcon';
    import DownloadIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import PencilIcon from 'phosphor-svelte/lib/PencilSimpleIcon';
    import FolderInputIcon from 'phosphor-svelte/lib/FolderOpenIcon';
    import Trash2Icon from 'phosphor-svelte/lib/TrashIcon';
    import UploadIcon from 'phosphor-svelte/lib/UploadSimpleIcon';
    import FolderUpIcon from 'phosphor-svelte/lib/UploadSimpleIcon';
    import FolderPlusIcon from 'phosphor-svelte/lib/FolderPlusIcon';
    import RefreshCwIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import UsersIcon from 'phosphor-svelte/lib/UsersIcon';
    import {
        contextMenuState,
        hideContextMenu,
        type ContextMenuIcon,
        type ContextMenuItem,
    } from './context_menu_store';
    import { fileTypeFamily, fileTypeIcon } from '../file_list/file_type';
    import { createSheetDragController, FLICK_SPEED } from '../modals/sheet_gesture';
    import { isMobilePlatform } from '../../api';
    import { hapticPress } from '../mobile/haptics';
    import { installModalA11y } from '../modals/modal_a11y';
    import { t_store } from '../../i18n/i18n';

    // The store names an icon; this module owns what that name looks like, so
    // the action builders never import a component.
    const ICONS: Record<ContextMenuIcon, typeof FileIcon> = {
        open: EyeIcon,
        play: PlayIcon,
        download: DownloadIcon,
        rename: PencilIcon,
        move: FolderInputIcon,
        delete: Trash2Icon,
        upload: UploadIcon,
        'folder-up': FolderUpIcon,
        'folder-new': FolderPlusIcon,
        refresh: RefreshCwIcon,
        external: ExternalLinkIcon,
        clear: XIcon,
        users: UsersIcon,
    };

    /** The header glyph: the same type icon the row showed, not a generic page. */
    const headerIcon = $derived.by(() => {
        const header = $contextMenuState.header;
        if (!header) return FileIcon;
        if (header.kind === 'folder') return FolderIcon;
        return fileTypeIcon(fileTypeFamily(header.ext ?? ''));
    });

    // The phone sheet is a plain list under its header, as the board draws it:
    // Open, Download, Rename, Move and Move to Trash, then Cancel. The icon tiles
    // and the details table belong to the desktop popover's habits and are not
    // drawn here, so every action reads as one row with its own label.
    const listItems = $derived($contextMenuState.items);
    /** The phone calls the delete action what it does: it moves to the Trash. */
    function rowLabel(item: { label: string; icon?: string }): string {
        if (item.icon === 'delete') return $t_store('mobile.move_to_trash');
        return item.label.replace(/…$/, '');
    }

    const VIEWPORT_MARGIN = 8;

    // The store feeds both surfaces; the phone renders a bottom action sheet,
    // every other platform the anchored popover. Resolved once: platform is
    // fixed for the session.
    const asSheet = isMobilePlatform();

    let panel = $state<HTMLElement | null>(null);
    let sheet = $state<HTMLElement | null>(null);
    let sheetOverlay = $state<HTMLElement | null>(null);
    let left = $state(0);
    let top = $state(0);
    let lastFocusVersion = 0;
    let invoker: HTMLElement | null = null;

    function container(): HTMLElement | null {
        return asSheet ? sheet : panel;
    }

    function menuButtons(): HTMLButtonElement[] {
        const root = container();
        if (!root) return [];
        const selector = asSheet ? 'button:not(:disabled)' : 'button[role="menuitem"]:not(:disabled)';
        return Array.from(root.querySelectorAll<HTMLButtonElement>(selector));
    }

    function focusMenuItem(delta: number): void {
        const items = menuButtons();
        if (!items.length) return;
        const current = document.activeElement instanceof HTMLButtonElement
            ? items.indexOf(document.activeElement)
            : -1;
        const next = current < 0 ? 0 : (current + delta + items.length) % items.length;
        items[next]?.focus();
    }

    function focusMenuEdge(edge: 'first' | 'last'): void {
        const items = menuButtons();
        const target = edge === 'first' ? items[0] : items[items.length - 1];
        target?.focus();
    }

    function captureInvoker(): void {
        const active = document.activeElement;
        if (active instanceof HTMLElement && !container()?.contains(active)) invoker = active;
    }

    function focusInvoker(): void {
        const target = invoker;
        invoker = null;
        if (!target?.isConnected || target.hasAttribute('disabled')) return;
        target.focus({ preventScroll: true });
    }

    async function dismissAndRestoreFocus(): Promise<void> {
        hideContextMenu();
        await tick();
        focusInvoker();
    }

    // Light impact when the action sheet appears -- the same feedback the long
    // press that opened it gives, from the same shared helper.
    function openHaptic(): void {
        if (!asSheet) return;
        hapticPress();
    }

    async function positionHost(): Promise<void> {
        const state = $contextMenuState;
        if (!state.open) return;

        if (asSheet) {
            if (lastFocusVersion !== state.focusVersion) {
                captureInvoker();
                lastFocusVersion = state.focusVersion;
                openHaptic();
                await tick();
                // Focus the sheet, not the first row: a touch-open shows no focus
                // ring, while arrow keys still move into the rows for keyboard use.
                sheet?.focus({ preventScroll: true });
            }
            return;
        }

        left = state.x;
        top = state.y;
        await tick();

        if (!panel) return;
        const rect = panel.getBoundingClientRect();
        const maxX = Math.max(VIEWPORT_MARGIN, window.innerWidth - rect.width - VIEWPORT_MARGIN);
        const maxY = Math.max(VIEWPORT_MARGIN, window.innerHeight - rect.height - VIEWPORT_MARGIN);
        left = Math.max(VIEWPORT_MARGIN, Math.min(state.x, maxX));
        top = Math.max(VIEWPORT_MARGIN, Math.min(state.y, maxY));

        if (lastFocusVersion !== state.focusVersion) {
            captureInvoker();
            lastFocusVersion = state.focusVersion;
            await tick();
            focusMenuEdge('first');
        }
    }

    function invoke(item: ContextMenuItem): void {
        if (item.type === 'divider' || item.disabled) return;
        hideContextMenu();
        // Hand the invoker to actions synchronously. A dialog opened by the
        // action records it as its restore target; navigation remains free to
        // establish its own focus without a delayed menu restoration.
        focusInvoker();
        void item.action();
    }

    function onDocumentClick(event: MouseEvent): void {
        if (!$contextMenuState.open) return;
        const host = asSheet ? sheetOverlay : container();
        if (host?.contains(event.target as Node)) return;
        void dismissAndRestoreFocus();
    }

    function onDocumentKeydown(event: KeyboardEvent): void {
        if (!$contextMenuState.open) return;
        if (event.key === 'Tab') {
            hideContextMenu();
            focusInvoker();
            return;
        }
        if (event.key === 'Escape') {
            event.preventDefault();
            void dismissAndRestoreFocus();
            return;
        }
        if (event.key === 'ArrowDown') {
            event.preventDefault();
            focusMenuItem(1);
            return;
        }
        if (event.key === 'ArrowUp') {
            event.preventDefault();
            focusMenuItem(-1);
            return;
        }
        if (event.key === 'Home') {
            event.preventDefault();
            focusMenuEdge('first');
            return;
        }
        if (event.key === 'End') {
            event.preventDefault();
            focusMenuEdge('last');
        }
    }

    // --- action-sheet swipe to dismiss ---------------------------------------
    // The pointer wiring is the shared sheet controller; this host only says
    // how to settle (spring back) and dismiss. The grab area is the grip plus
    // the header, so the whole top of the sheet drags.
    const sheetDrag = createSheetDragController({
        sheet: () => sheet,
        threshold: (height) => Math.max(72, height * 0.3),
        settle: (velocity) => {
            if (!sheet) return;
            const settle = velocity > FLICK_SPEED ? 'var(--motion-fast)' : 'var(--motion-med)';
            sheet.style.transition = `transform ${settle} var(--ease-enter)`;
            sheet.style.transform = 'translateY(0)';
        },
        dismiss: () => { void dismissAndRestoreFocus(); },
    });

    $effect(() => {
        void positionHost();
    });

    // A phone action sheet is a modal dialog. The shared ownership primitive
    // keeps the app behind it inert, loops Tab, restores the source row, and
    // unifies Escape with Android BACK.
    $effect(() => {
        if (!asSheet || !$contextMenuState.open || !sheetOverlay) return;
        const a11y = installModalA11y(sheetOverlay, {
            requestClose: () => { void dismissAndRestoreFocus(); },
            initialFocus: () => sheet,
            restoreFocus: () => invoker,
        });
        a11y.activate();
        return () => a11y.deactivate();
    });

</script>

<svelte:document onclick={onDocumentClick} onkeydown={onDocumentKeydown} />

{#if $contextMenuState.open}
    {#if asSheet}
        <div class="action-sheet-overlay" bind:this={sheetOverlay}>
            <div class="action-sheet-scrim" aria-hidden="true" onclick={() => { void dismissAndRestoreFocus(); }}></div>
            <div
                class="action-sheet"
                role="dialog"
                aria-modal="true"
                aria-labelledby={$contextMenuState.header ? 'action-sheet-title' : undefined}
                aria-label={$contextMenuState.header ? undefined : $t_store('menus.actions')}
                tabindex="-1"
                bind:this={sheet}
            >
            <!-- A visible, labelled way out. The scrim and swipe dismiss too, but
                 both are hidden from assistive technology, so without this an
                 iOS VoiceOver user has no control to close the sheet with. -->
            <button
                type="button"
                class="action-sheet-close"
                aria-label={$t_store('common.close')}
                onclick={() => { void dismissAndRestoreFocus(); }}
            >
                <XIcon size={20} aria-hidden="true" />
            </button>

            <!-- Grip plus header are one grab area, so the whole top drags; the
                 Close button above sits apart and keeps its tap. -->
            <div
                class="sheet-grab"
                role="presentation"
                onpointerdown={sheetDrag.onPointerDown}
                onpointermove={sheetDrag.onPointerMove}
                onpointerup={sheetDrag.onPointerUp}
                onpointercancel={sheetDrag.onPointerUp}
            >
                <div class="sheet-handle" aria-hidden="true"><span></span></div>

                {#if $contextMenuState.header}
                    {@const HeaderIcon = headerIcon}
                    <div class="action-sheet-header">
                        <span
                            class="action-sheet-icon"
                            class:is-folder={$contextMenuState.header.kind === 'folder'}
                            aria-hidden="true"
                        >
                            <HeaderIcon size={40} />
                        </span>
                        <span class="action-sheet-heading">
                            <span id="action-sheet-title" class="action-sheet-title">{$contextMenuState.header.title}</span>
                            {#if $contextMenuState.header.meta}
                                <span class="action-sheet-meta">{$contextMenuState.header.meta}</span>
                            {/if}
                        </span>
                    </div>
                {/if}
            </div>


            <div class="action-sheet-items">
                {#each listItems as item, index (item.type === 'divider' ? `divider-${index}` : `${item.label}-${index}`)}
                    {#if item.type === 'divider'}
                        <div class="action-sheet-sep" role="separator"></div>
                    {:else}
                        <button
                            type="button"
                            class="action-sheet-row"
                            class:danger={item.danger}
                            disabled={item.disabled}
                            onclick={() => invoke(item)}
                        >
                            {#if item.icon}
                                {@const RowIcon = ICONS[item.icon]}
                                <RowIcon size={20} aria-hidden="true" />
                            {/if}
                            <span>{rowLabel(item)}</span>
                        </button>
                    {/if}
                {/each}
                <button type="button" class="action-sheet-row action-sheet-cancel" onclick={() => { void dismissAndRestoreFocus(); }}>
                    <span>{$t_store('common.cancel')}</span>
                </button>
            </div>

            </div>
        </div>
    {:else}
        <div
            bind:this={panel}
            class="context-menu-panel"
            role="menu"
            aria-label={$t_store('menus.actions')}
            style:left={`${left}px`}
            style:top={`${top}px`}
        >
            {#each $contextMenuState.items as item, index (item.type === 'divider' ? `divider-${index}` : `${item.label}-${index}`)}
                {#if item.type === 'divider'}
                    <div class="divider" role="separator"></div>
                {:else}
                    <button
                        type="button"
                        role="menuitem"
                        class:danger={item.danger}
                        disabled={item.disabled}
                        tabindex="-1"
                        onclick={() => invoke(item)}
                    >
                        {#if item.icon}
                            {@const RowIcon = ICONS[item.icon]}
                            <RowIcon size={16} aria-hidden="true" />
                        {/if}
                        <span class="context-menu-label">{item.label}</span>
                        {#if item.shortcut}
                            <span class="context-menu-shortcut">{item.shortcut}</span>
                        {/if}
                    </button>
                {/if}
            {/each}
        </div>
    {/if}
{/if}
