<script lang="ts">
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import ImagesIcon from 'phosphor-svelte/lib/ImagesIcon';
    import ArrowDownUpIcon from 'phosphor-svelte/lib/ArrowsDownUpIcon';
    import CircleUserIcon from 'phosphor-svelte/lib/UserCircleIcon';
    import type { Component } from 'svelte';
    import Avatar from '../chrome/avatar.svelte';
    import { profileLoaded, profileUser } from '../chrome/profile_store';
    import type { MobileTab } from './mobile_shell_store';
    import { t_store } from '../../i18n/i18n';

    interface Props {
        active: MobileTab;
        /**
         * How many transfers need a person -- not how many are running. A badge
         * is an interrupt, so it only appears when there is something to act on.
         * Progress is ambient and belongs to the header ring.
         */
        transferBadge: number;
        onSelect: (tab: MobileTab) => void;
    }

    let { active, transferBadge, onSelect }: Props = $props();

    // Past nine the exact number stops changing the decision -- the user is
    // going to open the queue either way -- so the badge stops counting and
    // stays a fixed width instead of pushing the icon around.
    const BADGE_CAP = 9;

    // Keys, not text: the labels are translated in the template so they follow the language.
    const tabs: Array<{ id: MobileTab; label_key: string; icon: Component }> = [
        { id: 'files', label_key: 'mobile.tab_files', icon: FolderIcon },
        { id: 'photos', label_key: 'mobile.tab_photos', icon: ImagesIcon },
        { id: 'transfers', label_key: 'mobile.tab_transfers', icon: ArrowDownUpIcon },
        { id: 'account', label_key: 'mobile.tab_account', icon: CircleUserIcon },
    ];

    // The account tab is the one that is about the person using the app, so it
    // shows them rather than a drawing of a person. The icon stays until the
    // profile arrives: an empty disc in the meantime would read as a fault
    // rather than as loading.
    const showAvatar = $derived($profileLoaded && $profileUser !== null);

    // Only the transfers tab carries an attention suffix in its accessible name.
    function attentionSuffix(tab: MobileTab): string {
        if (tab !== 'transfers' || transferBadge <= 0) return '';
        return ', ' + $t_store('mobile.tab_attention', { count: transferBadge });
    }
</script>

<nav class="tab-bar" aria-label={$t_store('leftovers.primary_nav')}>
    {#each tabs as tab (tab.id)}
        {@const Icon = tab.icon}
        <button
            type="button"
            class="tab-item"
            class:active={active === tab.id}
            aria-current={active === tab.id ? 'page' : undefined}
            aria-label={`${$t_store(tab.label_key)}${attentionSuffix(tab.id)}`}
            onclick={() => onSelect(tab.id)}
        >
            <span class="tab-icon" class:is-avatar={tab.id === 'account' && showAvatar}>
                <!-- The pill grows behind the icon rather than cutting in, so a
                     tab change reads as one object settling (motion-17). -->
                <span class="tab-pill" aria-hidden="true"></span>
                {#if tab.id === 'account' && showAvatar}
                    <Avatar user={$profileUser} />
                {:else}
                    <Icon size={24} aria-hidden="true" />
                {/if}
                {#if tab.id === 'transfers' && transferBadge > 0}
                    <span class="tab-badge" aria-hidden="true">{transferBadge > BADGE_CAP ? `${BADGE_CAP}+` : transferBadge}</span>
                {/if}
            </span>
            <span class="tab-label">{$t_store(tab.label_key)}</span>
        </button>
    {/each}
</nav>
