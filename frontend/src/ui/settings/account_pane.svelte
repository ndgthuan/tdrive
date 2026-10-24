<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import { t_store } from '../../i18n/i18n';
    import Avatar from '../chrome/avatar.svelte';
    import { profileLoaded, profileUser } from '../chrome/profile_store';

    const displayName = $derived(
        !$profileLoaded ? $t_store('settings.account_loading') : $profileUser?.displayName || $t_store('settings.account_default_name'),
    );
    const handle = $derived(($profileUser?.username || '').trim());
</script>

<section class="settings-pane" aria-labelledby="settings-account-title">
    <header class="settings-pane-header">
        <h3 id="settings-account-title" class="settings-pane-title">{$t_store('settings.account_title')}</h3>
        <p class="settings-pane-sub">{$t_store('settings.account_subtitle')}</p>
    </header>

    <div class="account-card">
        <Avatar user={$profileUser} large />
        <div class="account-meta">
            <div class="account-name">{displayName}</div>
            {#if handle}<div class="account-handle">@{handle}</div>{/if}
        </div>
    </div>
</section>

<style>
    .account-card {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-width: 0;
        padding: var(--space-3) var(--space-4);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-1);
    }

    .account-meta { min-width: 0; flex: 1; }

    .account-name {
        color: var(--color-text);
        font-size: var(--type-md);
        font-weight: var(--weight-semibold);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .account-handle {
        margin-top: 2px;
        color: var(--color-text-muted);
        font-size: var(--type-xs);
        white-space: nowrap;
    }
</style>
