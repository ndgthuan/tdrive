<script lang="ts">
    import ClockIcon from 'phosphor-svelte/lib/ClockIcon';
    import EllipsisIcon from 'phosphor-svelte/lib/DotsThreeIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import type { DriveChannel, PendingJoin } from '../../types';
    import { contextMenuState } from '../menus/context_menu_store';
    import { sidebarState, type SidebarActionMenuRequest } from './sidebar_store';
    import { openChannelKey } from '../channels/channel_store';
    import { drive_roles, drive_member_counts } from '../members/members_store';
    import { driveListFailed } from './sidebar_store';
    import { retryLoadChannels } from '../../modules/channels';
    import { t_store } from '../../i18n/i18n';

    type DriveKind = 'personal' | 'shared';

    // The personal drive's Telegram channel is titled "TDrive" in the data; the
    // app calls the drive My Drive everywhere a person reads it, so the row does too.
    type ActionMenuHandler<T> = (request: SidebarActionMenuRequest, value: T) => void;

    interface Props {
        kind: DriveKind;
        onDriveClick: (channelId: number) => void;
        onDriveActions?: ActionMenuHandler<DriveChannel>;
        onPendingClick?: (inviteHash: string) => void;
        onPendingActions?: ActionMenuHandler<PendingJoin>;
        /** Shows the caller's role beside each shared drive once Telegram has answered for it. */
        showRoles?: boolean;
    }

    let {
        kind,
        onDriveClick,
        onDriveActions,
        onPendingClick,
        onPendingActions,
        showRoles = false,
    }: Props = $props();

    function roleLabelKey(role: string): string {
        return `members.role_${role}`;
    }

    let activeActionKey = $state<string | null>(null);
    let returnFocusTo = $state<HTMLElement | null>(null);
    let menuOpenedByThisList = $state(false);

    $effect(() => {
        const menuOpen = $contextMenuState.open;
        if (!activeActionKey) return;
        if (menuOpen) {
            menuOpenedByThisList = true;
            return;
        }
        if (!menuOpenedByThisList) return;

        const target = returnFocusTo;
        activeActionKey = null;
        returnFocusTo = null;
        menuOpenedByThisList = false;
        queueMicrotask(() => {
            if (target?.isConnected) target.focus();
        });
    });

    function titleFor(channel: DriveChannel): string {
        return channel.title || $t_store('sidebar.untitled');
    }

    function pendingTitleFor(pending: PendingJoin): string {
        return pending.title || $t_store('sidebar.pending_request');
    }

    function pendingTooltipFor(pending: PendingJoin): string {
        const error = pending.lastError.trim();
        return error ? $t_store('sidebar.waiting_error', { error }) : $t_store('sidebar.waiting_admin');
    }

    function driveActionKey(channel: DriveChannel): string {
        return `drive:${channel.id}`;
    }

    function pendingActionKey(pending: PendingJoin): string {
        return `pending:${pending.inviteHash}`;
    }

    function isActiveDrive(channel: DriveChannel): boolean {
        return $openChannelKey === null && $sidebarState.virtualView === null && $sidebarState.activeChannelId === channel.id;
    }

    function requestBelow(trigger: HTMLElement): SidebarActionMenuRequest {
        const rect = trigger.getBoundingClientRect();
        return { x: rect.left, y: rect.bottom + 4 };
    }

    function requestFromContextMenu(
        event: MouseEvent,
        returnTarget: HTMLElement,
    ): SidebarActionMenuRequest {
        if (event.clientX !== 0 || event.clientY !== 0) {
            return { x: event.clientX, y: event.clientY };
        }
        return requestBelow(returnTarget);
    }

    function openActions<T>(
        key: string,
        returnTarget: HTMLElement,
        request: SidebarActionMenuRequest,
        value: T,
        handler: ActionMenuHandler<T>,
    ): void {
        activeActionKey = key;
        returnFocusTo = returnTarget;
        menuOpenedByThisList = false;
        try {
            handler(request, value);
        } catch (error) {
            activeActionKey = null;
            returnFocusTo = null;
            throw error;
        }
    }

    function actionsFromButton<T>(
        event: MouseEvent,
        key: string,
        value: T,
        handler: ActionMenuHandler<T> | undefined,
    ): void {
        if (!handler) return;
        event.stopPropagation();
        const trigger = event.currentTarget as HTMLButtonElement;
        openActions(key, trigger, requestBelow(trigger), value, handler);
    }

    function returnTargetForContextMenu(row: HTMLElement, eventTarget: EventTarget | null): HTMLElement {
        const targetButton = eventTarget instanceof Element
            ? eventTarget.closest<HTMLButtonElement>('button')
            : null;
        if (targetButton && row.contains(targetButton)) return targetButton;
        return row.querySelector<HTMLButtonElement>('.drive-actions-trigger') ?? row;
    }

    function actionsFromContextMenu<T>(
        event: MouseEvent,
        key: string,
        value: T,
        handler: ActionMenuHandler<T> | undefined,
    ): void {
        if (!handler) return;
        event.preventDefault();
        const row = event.currentTarget as HTMLElement;
        const returnTarget = returnTargetForContextMenu(row, event.target);
        openActions(key, returnTarget, requestFromContextMenu(event, returnTarget), value, handler);
    }
</script>

{#if kind === 'shared' && $driveListFailed}
    <!-- The drives already listed stay put; this row says the refresh failed and
         offers the one action that matters, asking again. -->
    <button type="button" class="drive-retry" onclick={retryLoadChannels}>
        <span>{$t_store('mobile.shared_load_failed')}</span>
        <span class="drive-retry-action">{$t_store('mobile.retry')}</span>
    </button>
{/if}
{#if kind === 'personal'}
    {#if $sidebarState.personal.length === 0}
        <div class="drive-empty">{$t_store('sidebar.loading')}</div>
    {:else}
        {#each $sidebarState.personal as channel (channel.id)}
            <button
                type="button"
                class="drive-item"
                class:active={isActiveDrive(channel)}
                data-channel-id={channel.id}
                title={$t_store('sidebar.my_drive')}
                aria-current={isActiveDrive(channel) ? 'page' : undefined}
                onclick={() => onDriveClick(channel.id)}
            >
                <FolderIcon class="icon" size={16} aria-hidden="true" />
                <span class="drive-item-title">{$t_store('sidebar.my_drive')}</span>
            </button>
        {/each}
    {/if}
{:else if $sidebarState.shared.length === 0 && $sidebarState.pending.length === 0}
    <div class="drive-empty">{$t_store('sidebar.no_shared')}</div>
{:else}
    {#each $sidebarState.shared as channel (channel.id)}
        <div
            class="sidebar-drive-row"
            role="group"
            aria-label={titleFor(channel)}
            oncontextmenu={(event) => actionsFromContextMenu(event, driveActionKey(channel), channel, onDriveActions)}
        >
            <button
                type="button"
                class="drive-item"
                class:active={isActiveDrive(channel)}
                data-channel-id={channel.id}
                title={titleFor(channel)}
                aria-current={isActiveDrive(channel) ? 'page' : undefined}
                aria-describedby={showRoles && ($drive_roles[channel.id]?.role || $drive_member_counts[channel.id] !== undefined) ? `drive-role-${channel.id}` : undefined}
                onclick={() => onDriveClick(channel.id)}
            >
                <FolderIcon class="icon" size={18} aria-hidden="true" />
                <span class="drive-item-text">
                    <span class="drive-item-title">{titleFor(channel)}</span>
                    {#if showRoles && $drive_member_counts[channel.id] !== undefined}
                        <!-- Read out through the row's description with the role, not in its name. -->
                        <span class="drive-item-sub" aria-hidden="true">{$t_store('members.count', { count: $drive_member_counts[channel.id] })}</span>
                    {/if}
                </span>
                {#if showRoles && $drive_roles[channel.id]?.role}
                    <!-- The role is read out as the row's description, not as part of
                         its name, so the drive keeps the name people know it by. -->
                    <span class="drive-role-tag" aria-hidden="true">{$t_store(roleLabelKey($drive_roles[channel.id].role))}</span>
                {/if}
            </button>
            {#if showRoles && ($drive_roles[channel.id]?.role || $drive_member_counts[channel.id] !== undefined)}
                <span id="drive-role-{channel.id}" class="drive-role-sr">{[$t_store(roleLabelKey($drive_roles[channel.id].role)), $drive_member_counts[channel.id] !== undefined ? $t_store('members.count', { count: $drive_member_counts[channel.id] }) : ''].filter(Boolean).join(', ')}</span>
            {/if}
            {#if onDriveActions}
                <button
                    type="button"
                    class="drive-actions-trigger"
                    aria-label={$t_store('sidebar.actions_for', { title: titleFor(channel) })}
                    title={$t_store('sidebar.actions_for', { title: titleFor(channel) })}
                    aria-haspopup="menu"
                    aria-expanded={activeActionKey === driveActionKey(channel) && $contextMenuState.open ? 'true' : 'false'}
                    onclick={(event) => actionsFromButton(event, driveActionKey(channel), channel, onDriveActions)}
                >
                    <EllipsisIcon size={18} aria-hidden="true" />
                </button>
            {/if}
        </div>
    {/each}

    {#each $sidebarState.pending as pending (pending.inviteHash)}
        <div
            class="sidebar-drive-row"
            role="group"
            aria-label={pendingTitleFor(pending)}
            oncontextmenu={(event) => actionsFromContextMenu(event, pendingActionKey(pending), pending, onPendingActions)}
        >
            <button
                type="button"
                class="drive-item pending-drive-item"
                data-invite-hash={pending.inviteHash}
                title={pendingTooltipFor(pending)}
                onclick={() => onPendingClick?.(pending.inviteHash)}
            >
                <ClockIcon class="icon" size={18} aria-hidden="true" />
                <span class="drive-item-title">{pendingTitleFor(pending)}</span>
                <span class="pending-drive-tag">{$t_store('sidebar.pending_tag')}</span>
            </button>
            {#if onPendingActions}
                <button
                    type="button"
                    class="drive-actions-trigger"
                    aria-label={$t_store('sidebar.actions_for', { title: pendingTitleFor(pending) })}
                    title={$t_store('sidebar.actions_for', { title: pendingTitleFor(pending) })}
                    aria-haspopup="menu"
                    aria-expanded={activeActionKey === pendingActionKey(pending) && $contextMenuState.open ? 'true' : 'false'}
                    onclick={(event) => actionsFromButton(event, pendingActionKey(pending), pending, onPendingActions)}
                >
                    <EllipsisIcon size={18} aria-hidden="true" />
                </button>
            {/if}
        </div>
    {/each}
{/if}

<style>
    .sidebar-drive-row {
        display: grid;
        grid-template-columns: minmax(0, 1fr) 32px;
        align-items: center;
        min-width: 0;
    }

    .sidebar-drive-row > .drive-item {
        min-width: 0;
    }

    .drive-item-text {
        display: flex;
        flex: 1 1 auto;
        flex-direction: column;
        min-width: 0;
    }

    .drive-item-sub {
        overflow: hidden;
        color: var(--text-muted);
        font-size: 0.8rem;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .drive-role-sr {
        position: absolute;
        width: 1px;
        height: 1px;
        overflow: hidden;
        clip: rect(0 0 0 0);
        white-space: nowrap;
    }

    .drive-role-tag {
        flex: 0 0 auto;
        margin-left: auto;
        padding: 2px 8px;
        border: 1px solid var(--border);
        border-radius: var(--radius-pill);
        color: var(--text-muted);
        font-size: 0.72rem;
        white-space: nowrap;
    }

    .drive-actions-trigger {
        display: inline-grid;
        width: 32px;
        height: 32px;
        margin-right: 4px;
        padding: 0;
        place-items: center;
        border: 0;
        border-radius: var(--radius-sm);
        color: var(--text-muted);
        background: transparent;
        cursor: pointer;
    }

    /* The row's "..." is there for a pointer already on the row, or a keyboard
       already in it, so the sidebar stays a clean list until then. */
    .drive-actions-trigger {
        opacity: 0;
    }

    .sidebar-drive-row:hover .drive-actions-trigger,
    .sidebar-drive-row:focus-within .drive-actions-trigger,
    .drive-actions-trigger[aria-expanded='true'] {
        opacity: 1;
    }

    .drive-actions-trigger:hover {
        color: var(--text-main);
        background: var(--surface-control);
    }

    .drive-actions-trigger:focus-visible {
        outline: 2px solid var(--accent);
        outline-offset: -2px;
    }

    .drive-actions-trigger[aria-expanded='true'] {
        color: var(--accent);
        background: var(--overlay-accent-1);
    }
</style>
