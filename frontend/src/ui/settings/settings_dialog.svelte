<script lang="ts">
    import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import GlobeIcon from 'phosphor-svelte/lib/GlobeIcon';
    import PaletteIcon from 'phosphor-svelte/lib/PaletteIcon';
    import SlidersIcon from 'phosphor-svelte/lib/SlidersIcon';
    import SparkleIcon from 'phosphor-svelte/lib/SparkleIcon';
    import UserCircleIcon from 'phosphor-svelte/lib/UserCircleIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import type { Component } from 'svelte';
    import { isMobilePlatform } from '../../api';
    import { t_store } from '../../i18n/i18n';
    import ModalShell from '../modals/modal_shell.svelte';
    import AccountPane from './account_pane.svelte';
    import AiModelsPane from './ai_models_pane.svelte';
    import AppearancePane from './appearance_pane.svelte';
    import GeneralPane from './general_pane.svelte';
    import LanguagePane from './language_pane.svelte';
    import UpdatesPane from './updates_pane.svelte';
    import { close_settings, settings_view, type settings_pane } from './settings_store';
    import './settings.css';

    interface NavItem {
        id: settings_pane;
        label: string;
        icon: typeof SlidersIcon;
    }

    // Updates only exist where the desktop updater runs; phones use their stores.
    const navItems = $derived.by((): NavItem[] => {
        const items: NavItem[] = [
            { id: 'general', label: $t_store('settings.nav_general'), icon: SlidersIcon },
            { id: 'appearance', label: $t_store('settings.nav_appearance'), icon: PaletteIcon },
            { id: 'language', label: $t_store('settings.nav_language'), icon: GlobeIcon },
            { id: 'ai', label: $t_store('settings.nav_ai'), icon: SparkleIcon },
        ];
        if (!isMobilePlatform()) items.push({ id: 'updates', label: $t_store('settings.nav_updates'), icon: ArrowsClockwiseIcon });
        items.push({ id: 'account', label: $t_store('settings.nav_account'), icon: UserCircleIcon });
        return items;
    });

    const activePane = $derived(
        navItems.some((item) => item.id === $settings_view.pane) ? $settings_view.pane : 'general',
    );

    const paneComponents: Record<settings_pane, Component> = {
        general: GeneralPane,
        appearance: AppearancePane,
        language: LanguagePane,
        ai: AiModelsPane,
        updates: UpdatesPane,
        account: AccountPane,
    };
    const ActivePane = $derived(paneComponents[activePane]);

    function select(pane: settings_pane): void {
        settings_view.update((current) => ({ ...current, pane }));
    }
</script>

<ModalShell
    hostId="settings-modal"
    open={$settings_view.open}
    titleId="settings-dialog-title"
    cardClass="settings-card"
    presentation="dialog"
    initialFocus=".settings-nav-item.on"
    restoreFocus="#profile-trigger"
    onClose={close_settings}
>
    {#snippet header()}
        <h2 id="settings-dialog-title" class="settings-visually-hidden">{$t_store('settings.title')}</h2>
    {/snippet}

        <div class="settings-shell">
            <nav class="settings-nav" aria-label={$t_store('settings.nav_label')}>
                <div class="settings-nav-title">{$t_store('settings.title')}</div>
                {#each navItems as item (item.id)}
                    {@const Icon = item.icon}
                    <button
                        type="button"
                        class="settings-nav-item"
                        class:on={activePane === item.id}
                        aria-current={activePane === item.id ? 'page' : undefined}
                        onclick={() => select(item.id)}
                    >
                        <Icon size={16} aria-hidden="true" />
                        <span>{item.label}</span>
                    </button>
                {/each}
            </nav>

            <div class="settings-body">
                <button type="button" class="settings-close" aria-label={$t_store('settings.close')} onclick={close_settings}>
                    <XIcon size={16} aria-hidden="true" />
                </button>
                <ActivePane />
            </div>
        </div>
</ModalShell>
