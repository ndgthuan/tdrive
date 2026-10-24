<script lang="ts">
    import DotsThreeIcon from 'phosphor-svelte/lib/DotsThreeIcon';
    import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlassIcon';
    import ModalShell from '../modals/modal_shell.svelte';
    import { notify } from '../../modules/notifications';
    import { t, t_store } from '../../i18n/i18n';
    import {
        create_drive_invite,
        get_drive_default_role,
        set_drive_default_role,
        type AssignableRole,
        type DriveMemberRow,
        type DriveRole,
    } from '../../api/drive_members';
    import { members_dialog } from './members_dialog_store';
    import {
        access_from_snapshot,
        change_member_role,
        drive_roles,
        load_members,
        load_more_members,
        members_view,
        remove_member,
    } from './members_store';
    import {
        assignable_roles,
        can_change_role,
        can_remove_member,
        error_message_key,
        role_is_elevated,
        role_label_key,
    } from './drive_roles';

    const view = members_dialog.state;
    const list = members_view;

    const driveId = $derived($view.payload?.driveId ?? 0);
    const access = $derived(access_from_snapshot(driveId, true, $drive_roles[driveId]));
    const subtitle = $derived($view.payload?.title ?? '');
    const actor = $derived({ role: access.role, canManageMembers: access.canManageMembers });

    let query = $state('');
    let searchTimer: ReturnType<typeof setTimeout> | null = null;
    let menuUserId = $state(0);
    let pending = $state<{ userId: number; name: string; ban: boolean } | null>(null);

    let requireApproval = $state(false);
    let inviteState = $state<'idle' | 'working' | 'copied' | 'failed'>('idle');
    let inviteLink = $state('');
    let inviteResetTimer: ReturnType<typeof setTimeout> | null = null;
    let defaultRole = $state<'editor' | 'viewer'>('editor');
    let defaultRoleFor = $state(0);
    let wasOpen = false;

    $effect(() => {
        if ($view.open && !wasOpen) {
            query = '';
            menuUserId = 0;
            pending = null;
            inviteLink = '';
            inviteState = 'idle';
            defaultRoleFor = 0;
        }
        wasOpen = $view.open;
    });

    // The default role is only readable and writable by someone who manages members.
    $effect(() => {
        const id = driveId;
        if (!$view.open || !id || !access.canManageMembers || defaultRoleFor === id) return;
        defaultRoleFor = id;
        void loadDefaultRole(id);
    });

    async function loadDefaultRole(id: number): Promise<void> {
        try {
            const answer = await get_drive_default_role(id);
            if (answer.ok && (answer.role === 'editor' || answer.role === 'viewer')) defaultRole = answer.role;
        } catch {
            // Keep the calm default; the backend still decides where newcomers land.
        }
    }

    function close(): void {
        if (searchTimer) clearTimeout(searchTimer);
        if (inviteResetTimer) clearTimeout(inviteResetTimer);
        members_dialog.close();
    }

    function reportError(code: string): void {
        notify({ level: 'error', title: t(error_message_key(code)) });
    }

    function onSearchInput(value: string): void {
        query = value;
        if (searchTimer) clearTimeout(searchTimer);
        searchTimer = setTimeout(() => {
            searchTimer = null;
            void load_members(driveId, query.trim());
        }, 250);
    }

    function nameOf(row: DriveMemberRow): string {
        if (row.displayName.trim()) return row.displayName.trim();
        if (row.username) return `@${row.username}`;
        return $t_store('members.unnamed');
    }

    function initialsOf(row: DriveMemberRow): string {
        const source = (row.displayName || row.username || '?').trim();
        const parts = source.split(/\s+/).filter(Boolean);
        const first = Array.from(parts[0] ?? '?')[0] ?? '?';
        const last = parts.length > 1 ? Array.from(parts[parts.length - 1])[0] ?? '' : '';
        return `${first}${last}`.toUpperCase();
    }

    // Every role that can appear in the row's dropdown. The current role is always
    // listed (disabled), so the select can show it; the others are enabled only
    // when the actor may make that change.
    function roleChoices(row: DriveMemberRow): { value: DriveRole; disabled: boolean }[] {
        const target = { userId: row.userId, role: row.role, isSelf: row.isSelf };
        const offered = assignable_roles(access.role);
        const values: DriveRole[] = ['manager', 'editor', 'viewer'];
        return values
            .filter((value) => value === row.role || offered.includes(value as AssignableRole))
            .map((value) => ({
                value,
                disabled: value === row.role || !can_change_role(actor, target, value as AssignableRole),
            }));
    }

    function hasRowControls(row: DriveMemberRow): boolean {
        return access.canManageMembers && !row.isSelf && row.role !== 'owner';
    }

    function rowCanBeRemoved(row: DriveMemberRow): boolean {
        return can_remove_member(actor, { userId: row.userId, role: row.role, isSelf: row.isSelf });
    }

    async function onRoleChange(row: DriveMemberRow, event: Event): Promise<void> {
        const next = (event.currentTarget as HTMLSelectElement).value as AssignableRole;
        if (next === row.role) return;
        const outcome = await change_member_role(row.userId, next);
        if (!outcome.ok) reportError(outcome.errorCode);
    }

    function askRemove(row: DriveMemberRow, ban: boolean): void {
        menuUserId = 0;
        pending = { userId: row.userId, name: nameOf(row), ban };
    }

    async function confirmRemove(): Promise<void> {
        const target = pending;
        if (!target) return;
        pending = null;
        const outcome = await remove_member(target.userId, target.ban);
        if (!outcome.ok) reportError(outcome.errorCode);
    }

    function onWindowClick(event: MouseEvent): void {
        if (!menuUserId) return;
        const target = event.target as Element | null;
        if (target?.closest?.('.member-more-wrap')) return;
        menuUserId = 0;
    }

    async function copyInvite(): Promise<void> {
        if (inviteState === 'working') return;
        inviteState = 'working';
        let link = '';
        try {
            const invite = await create_drive_invite(driveId, requireApproval);
            if (!invite.ok || !invite.link) {
                inviteState = 'failed';
                reportError(invite.errorCode);
                return;
            }
            link = invite.link;
        } catch {
            inviteState = 'failed';
            reportError('');
            return;
        }
        inviteLink = link;
        try {
            await navigator.clipboard.writeText(link);
            inviteState = 'copied';
        } catch {
            // The link stays on screen so it can be selected and copied by hand.
            inviteState = 'failed';
        }
        if (inviteResetTimer) clearTimeout(inviteResetTimer);
        inviteResetTimer = setTimeout(() => {
            inviteState = 'idle';
            inviteResetTimer = null;
        }, 1600);
    }

    async function onDefaultRoleChange(next: 'editor' | 'viewer'): Promise<void> {
        if (next === defaultRole) return;
        const previous = defaultRole;
        defaultRole = next;
        try {
            const outcome = await set_drive_default_role(driveId, next);
            if (!outcome.ok) {
                defaultRole = previous;
                reportError(outcome.errorCode);
                return;
            }
            notify({ level: 'success', title: t('members.default_role_saved', { role: t(role_label_key(next)) }) });
        } catch {
            defaultRole = previous;
            reportError('');
        }
    }
</script>

<svelte:window onclick={onWindowClick} />

<ModalShell
    hostId="members-dialog"
    open={$view.open}
    title={$t_store('members.title')}
    titleId="members-title"
    {subtitle}
    cardClass="members-card"
    initialFocus="#members-search"
    restoreFocus="#drives-nav"
    onClose={close}
>
    <div class="members-body">
        {#if access.viewOnly}
            <p class="members-notice" role="note">{$t_store('members.view_only_hint')}</p>
        {/if}

        <div class="members-toolbar">
            <label class="search-field members-search">
                <MagnifyingGlassIcon size={15} aria-hidden="true" />
                <input
                    id="members-search"
                    type="search"
                    autocomplete="off"
                    spellcheck="false"
                    placeholder={$t_store('members.search_placeholder')}
                    aria-label={$t_store('members.search_label')}
                    value={query}
                    oninput={(event) => onSearchInput((event.currentTarget as HTMLInputElement).value)}
                />
            </label>
            {#if $list.status === 'ready'}
                <span class="members-count">{$t_store('members.count', { count: $list.total })}</span>
            {/if}
        </div>

        {#if pending}
            <div class="members-confirm" role="alertdialog" aria-labelledby="members-confirm-title" aria-describedby="members-confirm-body">
                <div id="members-confirm-title" class="members-confirm-title">
                    {pending.ban
                        ? $t_store('members.confirm_ban_title', { name: pending.name })
                        : $t_store('members.confirm_remove_title', { name: pending.name })}
                </div>
                <p id="members-confirm-body" class="members-confirm-body">
                    {pending.ban ? $t_store('members.confirm_ban_body') : $t_store('members.confirm_remove_body')}
                </p>
                <div class="members-confirm-actions">
                    <button type="button" class="secondary-btn compact-btn" onclick={() => (pending = null)}>{$t_store('common.cancel')}</button>
                    <button type="button" class="danger-btn compact-btn" onclick={() => void confirmRemove()}>
                        {pending.ban ? $t_store('members.confirm_ban_action') : $t_store('members.confirm_remove_action')}
                    </button>
                </div>
            </div>
        {/if}

        <div class="members-list-wrap">
            {#if $list.status === 'loading' || $list.status === 'idle'}
                <div class="modal-empty">{$t_store('members.loading')}</div>
            {:else if $list.status === 'error'}
                <div class="modal-error members-error" role="alert">
                    <span>{$t_store('members.load_error')}</span>
                    <button type="button" class="link-button" onclick={() => void load_members(driveId, query.trim())}>{$t_store('members.retry')}</button>
                </div>
            {:else if $list.members.length === 0}
                <div class="modal-empty">{$t_store('members.empty')}</div>
            {:else}
                <ul class="members-list" aria-label={$t_store('members.list_label')}>
                    {#each $list.members as row (row.userId)}
                        <li class="member-row" class:is-busy={$list.busyUserId === row.userId} data-user-id={row.userId}>
                            <span class="member-avatar" aria-hidden="true">{initialsOf(row)}</span>
                            <div class="member-meta">
                                <div class="member-name-line">
                                    <span class="member-name">{nameOf(row)}</span>
                                    {#if row.isSelf}
                                        <span class="member-you">{$t_store('members.you')}</span>
                                    {/if}
                                </div>
                                {#if row.username}
                                    <div class="member-username">@{row.username}</div>
                                {/if}
                            </div>

                            <div class="member-controls">
                                {#if hasRowControls(row)}
                                    {@const choices = roleChoices(row)}
                                    <select
                                        class="select-field member-role-select"
                                        aria-label={$t_store('members.change_role_aria', { name: nameOf(row) })}
                                        value={row.role}
                                        disabled={$list.busyUserId !== 0 || choices.every((choice) => choice.disabled)}
                                        onchange={(event) => void onRoleChange(row, event)}
                                    >
                                        {#each choices as choice (choice.value)}
                                            <option value={choice.value} disabled={choice.disabled}>{$t_store(role_label_key(choice.value))}</option>
                                        {/each}
                                    </select>
                                {:else}
                                    <span class="role-pill" data-tone={role_is_elevated(row.role) ? 'elevated' : 'muted'}>
                                        {$t_store(role_label_key(row.role))}
                                    </span>
                                {/if}

                                {#if access.canManageMembers && rowCanBeRemoved(row)}
                                    <div class="member-more-wrap">
                                        <button
                                            type="button"
                                            class="icon-btn member-more"
                                            aria-label={$t_store('members.more_aria', { name: nameOf(row) })}
                                            aria-haspopup="menu"
                                            aria-expanded={menuUserId === row.userId ? 'true' : 'false'}
                                            disabled={$list.busyUserId !== 0}
                                            onclick={(event) => {
                                                event.stopPropagation();
                                                menuUserId = menuUserId === row.userId ? 0 : row.userId;
                                            }}
                                        >
                                            <DotsThreeIcon size={18} aria-hidden="true" />
                                        </button>
                                        {#if menuUserId === row.userId}
                                            <div class="member-more-menu" role="menu">
                                                <button type="button" role="menuitem" onclick={() => askRemove(row, false)}>
                                                    {$t_store('members.remove')}
                                                </button>
                                                <button type="button" role="menuitem" class="danger-text" onclick={() => askRemove(row, true)}>
                                                    {$t_store('members.remove_and_ban')}
                                                </button>
                                            </div>
                                        {/if}
                                    </div>
                                {/if}
                            </div>
                        </li>
                    {/each}
                </ul>

                {#if $list.hasMore}
                    <div class="members-more">
                        <button type="button" class="secondary-btn compact-btn" disabled={$list.loadingMore} onclick={() => void load_more_members()}>
                            {$list.loadingMore ? $t_store('members.loading_more') : $t_store('members.load_more')}
                        </button>
                    </div>
                {/if}
            {/if}
        </div>

        {#if access.canInvite}
            <section class="members-invite" aria-labelledby="members-invite-title">
                <h3 id="members-invite-title" class="members-section-title">{$t_store('members.invite_title')}</h3>

                <div class="members-invite-row">
                    <label class="members-toggle">
                        <input type="checkbox" bind:checked={requireApproval} onchange={() => (inviteLink = '')} />
                        <span>
                            <span class="members-toggle-label">{$t_store('members.invite_require_approval')}</span>
                            <span class="members-toggle-hint">{$t_store('members.invite_require_approval_hint')}</span>
                        </span>
                    </label>
                    <button
                        id="members-copy-invite"
                        type="button"
                        class="primary-btn compact-btn"
                        disabled={inviteState === 'working'}
                        onclick={() => void copyInvite()}
                    >
                        {inviteState === 'copied'
                            ? $t_store('members.invite_copied')
                            : inviteState === 'failed'
                                ? $t_store('members.invite_copy_failed')
                                : $t_store('members.invite_copy')}
                    </button>
                </div>

                {#if inviteLink}
                    <div class="members-invite-link" aria-label={$t_store('members.invite_link_label')}>{inviteLink}</div>
                {/if}

                {#if access.canManageMembers}
                    <div class="members-default-row" role="group" aria-labelledby="members-default-label">
                        <span id="members-default-label" class="members-default-label">{$t_store('members.default_role')}</span>
                        <div class="members-role-chips">
                            {#each ['editor', 'viewer'] as role (role)}
                                <button
                                    id="members-default-role-{role}"
                                    type="button"
                                    class="members-role-chip"
                                    class:is-selected={defaultRole === role}
                                    aria-pressed={defaultRole === role}
                                    onclick={() => void onDefaultRoleChange(role as 'editor' | 'viewer')}
                                >
                                    {role === 'editor' ? $t_store('members.role_editor') : $t_store('members.role_viewer')}
                                </button>
                            {/each}
                        </div>
                    </div>
                {/if}
            </section>
        {/if}
    </div>

    {#snippet actions()}
        <button id="members-close" class="secondary-btn" type="button" onclick={close}>{$t_store('common.close')}</button>
    {/snippet}
</ModalShell>

<style>
    .members-body {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        min-width: 0;
    }

    .members-notice {
        margin: 0;
        padding: var(--space-2) var(--space-3);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        color: var(--text-muted);
        font-size: var(--font-size-sm);
    }

    .members-toolbar {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }

    .members-search {
        flex: 1;
        min-width: 0;
    }

    /* The wrapper already draws the focus ring; the input inside must not add a second. */
    .members-search input:focus,
    .members-search input:focus-visible {
        outline: none;
        box-shadow: none;
    }

    .members-count {
        color: var(--text-muted);
        font-size: var(--font-size-sm);
        white-space: nowrap;
    }

    .members-confirm {
        padding: var(--space-3);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        background: var(--overlay-white-1);
    }

    .members-confirm-title {
        color: var(--text-main);
        font-weight: 600;
        font-size: var(--font-size-base);
    }

    .members-confirm-body {
        margin: var(--space-1) 0 var(--space-2);
        color: var(--text-muted);
        font-size: var(--font-size-sm);
    }

    .members-confirm-actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--space-2);
    }

    .members-list-wrap {
        min-width: 0;
    }

    .members-list {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        margin: 0;
        padding: 0;
        list-style: none;
        max-height: min(340px, 44vh);
        overflow-y: auto;
        overscroll-behavior: contain;
    }

    .member-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-2) var(--space-3);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        background: var(--overlay-white-1);
        transition: opacity 120ms ease;
    }

    .member-row.is-busy {
        opacity: 0.6;
    }

    .member-avatar {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        flex-shrink: 0;
        width: 32px;
        height: 32px;
        border-radius: var(--radius-pill);
        background: var(--surface-control);
        color: var(--text-main);
        font-size: 0.72rem;
        font-weight: 600;
        letter-spacing: 0.02em;
    }

    .member-meta {
        flex: 1;
        min-width: 0;
    }

    .member-name-line {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        min-width: 0;
    }

    .member-name {
        color: var(--text-main);
        font-weight: 600;
        font-size: 0.9rem;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .member-you {
        flex-shrink: 0;
        padding: 1px 6px;
        border-radius: var(--radius-pill);
        border: 1px solid var(--border);
        color: var(--text-muted);
        font-size: 0.68rem;
    }

    .member-username {
        color: var(--text-muted);
        font-size: 0.78rem;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .member-controls {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        flex-shrink: 0;
    }

    .member-role-select {
        width: auto;
        min-width: 112px;
        font-size: 0.8rem;
        padding: 6px 28px 6px 10px;
    }

    .role-pill {
        display: inline-block;
        padding: 3px 10px;
        border-radius: var(--radius-pill);
        border: 1px solid var(--border);
        color: var(--text-muted);
        font-size: 0.75rem;
        font-weight: 500;
        white-space: nowrap;
    }

    .role-pill[data-tone='elevated'] {
        color: var(--accent);
        border-color: color-mix(in srgb, var(--accent) 45%, transparent);
        background: color-mix(in srgb, var(--accent) 14%, transparent);
    }

    .member-more-wrap {
        position: relative;
    }

    .member-more-menu {
        position: absolute;
        right: 0;
        top: calc(100% + 4px);
        z-index: 2;
        display: flex;
        flex-direction: column;
        min-width: 180px;
        padding: var(--space-1);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        background: var(--surface-control);
    }

    .member-more-menu button {
        padding: 8px 10px;
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-main);
        font: inherit;
        font-size: 0.82rem;
        text-align: left;
        cursor: pointer;
    }

    .member-more-menu button:hover {
        background: var(--overlay-white-1);
    }

    .members-more {
        display: flex;
        justify-content: center;
        margin-top: var(--space-2);
    }

    .members-error {
        display: flex;
        flex-direction: column;
        align-items: flex-start;
        gap: var(--space-2);
    }

    .members-invite {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
        padding-top: var(--space-3);
        border-top: 1px solid var(--border);
    }

    .members-section-title {
        margin: 0;
        color: var(--text-main);
        font-size: 0.85rem;
        font-weight: 600;
    }

    .members-invite-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }

    .members-toggle {
        display: flex;
        align-items: flex-start;
        justify-content: flex-start;
        justify-items: start;
        gap: var(--space-2);
        flex: 1 1 auto;
        width: auto;
        min-width: 0;
        margin: 0;
        cursor: pointer;
    }

    .members-toggle input {
        flex: 0 0 16px;
        width: 16px;
        height: 16px;
        margin: 2px 0 0;
        accent-color: var(--accent);
    }

    .members-toggle-label {
        display: block;
        color: var(--text-main);
        font-size: 0.85rem;
    }

    .members-toggle-hint {
        display: block;
        color: var(--text-muted);
        font-size: 0.75rem;
    }

    .members-invite-link {
        padding: var(--space-2) var(--space-3);
        border: 1px dashed var(--border);
        border-radius: var(--radius-md);
        color: var(--text-muted);
        font-size: 0.78rem;
        word-break: break-all;
        user-select: all;
    }

    .members-default-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }

    .members-default-label {
        color: var(--text-main);
        font-size: 0.85rem;
    }

    .members-role-chips {
        display: flex;
        gap: 8px;
    }

    .members-role-chip {
        height: 34px;
        padding: 0 14px;
        border-radius: 17px;
        background: var(--color-surface-2);
        color: var(--color-text-muted);
        font: inherit;
        font-size: 14px;
    }

    .members-role-chip.is-selected {
        background: var(--color-accent-soft, var(--color-surface-3));
        color: var(--color-accent);
        font-weight: 600;
    }
</style>
