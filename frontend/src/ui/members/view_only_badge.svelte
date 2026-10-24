<script lang="ts">
    import EyeIcon from 'phosphor-svelte/lib/EyeIcon';
    import { t_store } from '../../i18n/i18n';
    import { active_drive_access } from './members_store';

    // Shown beside the open shared drive's name when this account can only read it.
    // Personal drives and drives with an unknown role show nothing.
    const viewOnly = $derived($active_drive_access.shared && $active_drive_access.known && $active_drive_access.viewOnly);
</script>

{#if viewOnly}
    <span class="view-only-badge" title={$t_store('members.view_only_tooltip')} data-testid="view-only-badge">
        <EyeIcon size={13} aria-hidden="true" />
        <span>{$t_store('members.view_only')}</span>
    </span>
{/if}

<style>
    .view-only-badge {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        flex-shrink: 0;
        padding: 3px 9px;
        border-radius: var(--radius-pill);
        border: 1px solid var(--border);
        color: var(--text-muted);
        font-size: 0.74rem;
        font-weight: 500;
        white-space: nowrap;
    }
</style>
