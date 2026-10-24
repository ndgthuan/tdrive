<script lang="ts">
    import EjectIcon from 'phosphor-svelte/lib/EjectIcon';
    import HardDriveIcon from 'phosphor-svelte/lib/HardDriveIcon';
    import { onMount } from 'svelte';
    import { notify } from '../../modules/notifications';
    import { t_store } from '../../i18n/i18n';
    import type { MountableDrive } from '../../types';
    import {
        createMountController,
        defaultMountApi,
        type MountController,
    } from './mount_controller';

    interface Props {
        controller?: MountController;
        onMenuAction?: () => void;
        loadDrives?: () => Promise<readonly MountableDrive[]>;
        variant?: 'menu' | 'sidebar';
    }

    let {
        controller = createMountController(defaultMountApi, notify),
        onMenuAction = () => undefined,
        loadDrives,
        variant = 'menu',
    }: Props = $props();

    let loadingDrives = $derived(controller.loadingDrives);

    onMount(() => {
        void controller.refresh();
    });

    const ejectLabel = $derived.by(() => {
        if ($controller.phase !== 'disconnecting') return $t_store('mount.eject_drive');
        if ($controller.mode === 'read-write' && $controller.writeState === 'draining') {
            if ($controller.activeWrites >= 1) return $t_store('mount.finishing_changes', { count: $controller.activeWrites });
            return $t_store('mount.finishing');
        }
        return $t_store('mount.ejecting');
    });
</script>

<div
    class="mount-control"
    role="group"
    aria-label={$t_store('mount.controls_label', { label: $controller.label })}
    aria-live="polite"
>
    {#if $controller.mounted}
        {#if variant === 'sidebar'}
            <div class="mount-status-row">
                <span class="mount-status-dot" aria-hidden="true"></span>
                <span class="mount-status-label">{$controller.label}</span>
                {#if $controller.location}<span class="mount-status-location" title={$controller.location}>{$controller.location}</span>{/if}
            </div>
        {/if}
        <button
            id="disconnect-mounted-drive-button"
            class={variant === 'menu' ? 'profile-menu-item mount-menu-item' : 'drive-action-btn mount-sidebar-action'}
            type="button"
            role={variant === 'menu' ? 'menuitem' : undefined}
            disabled={$controller.phase === 'disconnecting'}
            aria-busy={$controller.phase === 'disconnecting' ? 'true' : undefined}
            aria-label={$t_store('mount.eject_drive')}
            onclick={() => {
                onMenuAction();
                void controller.disconnect();
            }}
        >
            <EjectIcon class="mount-action-icon" size={16} aria-hidden="true" />
            <span class="mount-menu-title">{ejectLabel}</span>
            {#if $controller.phase === 'disconnecting'}
                <span class="mount-menu-spinner" aria-hidden="true"></span>
            {/if}
        </button>
        {#if variant === 'sidebar' && $controller.phase === 'disconnecting' && $controller.mode === 'read-write' && $controller.writeState === 'draining'}
            <p class="mount-notice">{$t_store('mount.eject_waits')}</p>
        {/if}
    {:else}
        <button
            id="mount-drive-button"
            class={variant === 'menu' ? 'profile-menu-item mount-menu-item' : 'drive-action-btn mount-sidebar-action'}
            type="button"
            role={variant === 'menu' ? 'menuitem' : undefined}
            disabled={$loadingDrives || $controller.phase === 'mounting'}
            aria-busy={$loadingDrives || $controller.phase === 'mounting' ? 'true' : undefined}
            aria-label={$t_store('mount.mount_label', { label: $controller.label })}
            onclick={() => {
                void controller.requestMount({ loadDrives, onAction: onMenuAction });
            }}
        >
            <HardDriveIcon class="mount-action-icon" size={16} aria-hidden="true" />
            <span class="mount-menu-title">
                {$loadingDrives ? $t_store('mount.loading_drives') : $controller.phase === 'mounting' ? $t_store('mount.mounting_label', { label: $controller.label }) : $t_store('mount.mount_action')}
            </span>
            {#if $loadingDrives || $controller.phase === 'mounting'}
                <span class="mount-menu-spinner" aria-hidden="true"></span>
            {/if}
        </button>
    {/if}

    {#if $controller.phase === 'error'}
        <span class="mount-announcement" role="alert">
            {$controller.error || $t_store('mount.operation_failed')}
        </span>
    {/if}
</div>

<style>
    .mount-control {
        position: relative;
        display: block;
        width: 100%;
    }

    .mount-menu-item {
        min-width: 0;
    }

    .mount-menu-item:disabled {
        cursor: wait;
        opacity: 0.65;
    }

    .mount-sidebar-action {
        width: 100%;
        min-width: 0;
        height: var(--control-md);
        border-radius: var(--radius-sm);
        justify-content: flex-start;
        text-align: left;
    }

    #disconnect-mounted-drive-button.mount-sidebar-action { color: var(--danger); }

    .mount-sidebar-action:disabled {
        cursor: wait;
        opacity: 0.65;
    }

    .mount-control :global(.mount-action-icon) {
        width: 16px;
        height: 16px;
        flex: 0 0 auto;
    }

    .mount-menu-title {
        min-width: 0;
        flex: 1;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .mount-menu-spinner {
        width: 13px;
        height: 13px;
        flex: 0 0 auto;
        margin-left: auto;
        border: 2px solid currentColor;
        border-top-color: transparent;
        border-radius: var(--radius-pill);
        animation: mount-menu-spin 720ms linear infinite;
    }

    /* The mounted drive, as the sidebar draws it: a green dot, the name, and
       the location on the right, then Eject. */
    .mount-status-row {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        height: var(--control-md);
        margin-bottom: var(--space-1-5);
        padding: 0 var(--space-2);
        border-radius: var(--radius-sm);
        background: var(--overlay-accent-1);
        color: var(--color-text);
        font-size: var(--type-base);
    }

    .mount-status-dot {
        flex: 0 0 auto;
        width: 8px;
        height: 8px;
        margin: 0 var(--space-1);
        border-radius: var(--radius-pill);
        background: var(--color-success);
    }

    .mount-status-label {
        flex: 1 1 auto;
        min-width: 0;
        overflow: hidden;
        font-weight: var(--weight-semibold);
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .mount-status-location {
        flex: 0 1 auto;
        min-width: 0;
        max-width: 50%;
        overflow: hidden;
        color: var(--text-muted);
        font-size: var(--type-xs);
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .mount-notice {
        margin: var(--space-2) 0 0;
        color: var(--text-muted);
        font-size: var(--type-xs);
        line-height: 1.5;
    }

    .mount-announcement {
        position: absolute;
        width: 1px;
        height: 1px;
        padding: 0;
        margin: -1px;
        overflow: hidden;
        clip: rect(0, 0, 0, 0);
        white-space: nowrap;
        border: 0;
    }

    @keyframes mount-menu-spin {
        to { transform: rotate(360deg); }
    }

    @media (prefers-reduced-motion: reduce) {
        .mount-menu-spinner {
            animation: none;
        }
    }
</style>
