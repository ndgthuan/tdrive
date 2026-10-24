<script lang="ts">
    import ChevronDownIcon from 'phosphor-svelte/lib/CaretDownIcon';
    import GearIcon from 'phosphor-svelte/lib/GearIcon';
    import LockKeyholeIcon from 'phosphor-svelte/lib/LockKeyIcon';
    import LogOutIcon from 'phosphor-svelte/lib/SignOutIcon';
    import HardDriveIcon from 'phosphor-svelte/lib/HardDriveIcon';
    import PhotoBackupPanel from '../gallery/photo_backup_panel.svelte';
    import { tick } from 'svelte';
    import { t_store } from '../../i18n/i18n';
    import { eventOccurredWithin } from '../event_path';
    import { recoverThemeTransitionClick } from '../theme/theme_interaction';
    import { open_settings } from '../settings/settings_host';
    import { updateBadge } from '../updates/update_store';
    import Avatar from './avatar.svelte';
    import { encryptionEntryVisible, profileLoaded, profileUser } from './profile_store';

    interface Props {
        // Called when the menu opens, so the module can lazily fetch Me().
        onOpen: () => void;
        onEncryptionSettings: () => void;
        onLogout: () => void;
        // Kept for the shell's call site. Updates now live in the Settings dialog,
        // which checks the platform itself.
        updaterAvailable?: boolean;
    }

    let { onOpen, onEncryptionSettings, onLogout }: Props = $props();

    type MenuView = 'account' | 'backup';

    let open = $state(false);
    let view = $state<MenuView>('account');
    let triggerEl = $state<HTMLButtonElement | null>(null);
    let menuEl = $state<HTMLElement | null>(null);

    const displayName = $derived(
        !$profileLoaded ? $t_store('leftovers.loading_account') : $profileUser?.displayName || $t_store('leftovers.telegram_account'),
    );
    const handle = $derived(($profileUser?.username || '').trim());

    function menuItems(): HTMLElement[] {
        if (!menuEl) return [];
        return Array.from(menuEl.querySelectorAll<HTMLElement>('[role="menuitem"]'))
            .filter((item) => !item.hidden && !item.hasAttribute('disabled'));
    }

    async function openMenu(): Promise<void> {
        view = 'account';
        open = true;
        onOpen();
        await tick();
        menuItems()[0]?.focus();
    }

    function closeMenu(returnFocus = false): void {
        open = false;
        view = 'account';
        if (returnFocus) triggerEl?.focus();
    }

    function toggleMenu(): void {
        if (open) closeMenu();
        else void openMenu();
    }

    function activate(action: () => void): void {
        closeMenu();
        action();
    }

    async function openBackup(): Promise<void> { view = 'backup'; await tick(); menuEl?.querySelector<HTMLElement>('#photo-backup-back')?.focus(); }
    async function closeBackup(): Promise<void> { view = 'account'; await tick(); menuEl?.querySelector<HTMLElement>('#profile-menu-photo-backup')?.focus(); }

    function onMenuKeydown(event: KeyboardEvent): void {
        if (!open) return;
        if (event.key === 'Escape') {
            event.preventDefault();
            // Keep the window-level Escape handler from also closing the
            // popover after this view has handled the first navigation step.
            event.stopPropagation();
            if (view === 'backup') { void closeBackup(); return; }
            closeMenu(true);
            return;
        }
        if (view !== 'account') return;
        if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;

        const items = menuItems();
        if (!items.length) return;

        event.preventDefault();
        const current = document.activeElement as HTMLElement | null;
        const index = current ? items.indexOf(current) : -1;

        if (event.key === 'Home') {
            items[0].focus();
            return;
        }
        if (event.key === 'End') {
            items[items.length - 1].focus();
            return;
        }

        const direction = event.key === 'ArrowDown' ? 1 : -1;
        const next = index >= 0
            ? (index + direction + items.length) % items.length
            : direction > 0 ? 0 : items.length - 1;
        items[next].focus();
    }

    function onWindowKeydown(event: KeyboardEvent): void {
        if (event.key === 'Escape' && open) {
            closeMenu(true);
        }
    }

    function onDocumentClick(event: MouseEvent): void {
        if (!open) return;
        if (eventOccurredWithin(event, menuEl) || eventOccurredWithin(event, triggerEl)) return;
        if (recoverThemeTransitionClick(event, menuEl)
            || recoverThemeTransitionClick(event, triggerEl)) return;
        closeMenu();
    }

    function labelledBy(currentView: MenuView): string {
        if (currentView === 'backup') return 'photo-backup-back';
        return 'profile-trigger';
    }
</script>

<svelte:window onkeydown={onWindowKeydown} />
<svelte:document onclickcapture={onDocumentClick} />

<button
    bind:this={triggerEl}
    id="profile-trigger"
    data-theme-hit-target
    class="profile-trigger"
    type="button"
    aria-haspopup="menu"
    aria-expanded={open ? 'true' : 'false'}
    aria-controls="profile-menu"
    title={$t_store('leftovers.account_title')}
    onclick={(event) => {
        event.stopPropagation();
        toggleMenu();
    }}
>
    <Avatar user={$profileUser} />
    {#if $updateBadge === 'ready'}
        <span class="profile-update-dot" aria-hidden="true"></span>
    {/if}
    <ChevronDownIcon class="profile-chevron" size={16} aria-hidden="true" />
</button>

<div
    bind:this={menuEl}
    id="profile-menu"
    class:backup-view={view === 'backup'}
    class="profile-menu"
    role={view === 'account' ? 'menu' : 'dialog'}
    aria-labelledby={labelledBy(view)}
    tabindex="-1"
    hidden={!open}
    onkeydown={onMenuKeydown}
>
    {#if view === 'backup'}
        <button id="photo-backup-back" class="profile-menu-item" type="button" onclick={() => void closeBackup()}>{$t_store('leftovers.back_to_account')}</button>
        <PhotoBackupPanel />
    {:else}
        <div class="profile-menu-header">
            <Avatar user={$profileUser} large />
            <div class="profile-menu-meta">
                <div id="profile-menu-name" class="profile-menu-name">{displayName}</div>
                {#if handle}
                    <div id="profile-menu-handle" class="profile-menu-handle">@{handle}</div>
                {/if}
            </div>
        </div>
        <div class="profile-menu-divider" role="separator"></div>
        <button
            id="profile-menu-settings"
            class="profile-menu-item"
            type="button"
            role="menuitem"
            onclick={() => activate(() => open_settings('general'))}
        >
            <GearIcon size={16} aria-hidden="true" />
            <span>{$t_store('settings.menu_settings')}</span>
        </button>
        <button id="profile-menu-photo-backup" class="profile-menu-item" type="button" role="menuitem" onclick={() => void openBackup()}>
            <HardDriveIcon size={16} aria-hidden="true" /><span>{$t_store('settings.menu_backup')}</span>
        </button>
        {#if $encryptionEntryVisible}
            <button
                id="profile-menu-encryption-settings"
                class="profile-menu-item"
                type="button"
                role="menuitem"
                onclick={() => activate(onEncryptionSettings)}
            >
                <LockKeyholeIcon size={16} aria-hidden="true" />
                <span>{$t_store('leftovers.encryption_settings')}</span>
            </button>
        {/if}
        <div class="profile-menu-divider" role="separator"></div>
        <button
            id="profile-menu-logout"
            class="profile-menu-item"
            type="button"
            role="menuitem"
            onclick={() => activate(onLogout)}
        >
            <LogOutIcon size={16} aria-hidden="true" />
            <span>{$t_store('settings.menu_logout')}</span>
        </button>
    {/if}
</div>
