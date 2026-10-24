<script lang="ts">
    import SquaresFourIcon from 'phosphor-svelte/lib/SquaresFourIcon';
    import RowsIcon from 'phosphor-svelte/lib/RowsIcon';
    import { t_store } from '../../i18n/i18n';
    import type { SourceViewMode } from './channel_view_mode';

    interface Props {
        value: SourceViewMode;
        onChange: (mode: SourceViewMode) => void;
    }

    let { value, onChange }: Props = $props();

    const OPTIONS = [
        { mode: 'grid', label_key: 'sources.view_grid', Icon: SquaresFourIcon },
        { mode: 'list', label_key: 'sources.view_list', Icon: RowsIcon },
    ] as const;
</script>

<div class="segmented channel-view-toggle" role="group" aria-label={$t_store('sources.view_mode')}>
    {#each OPTIONS as option (option.mode)}
        {@const Icon = option.Icon}
        <button
            type="button"
            class="channel-view-toggle-btn"
            aria-pressed={value === option.mode}
            aria-label={$t_store(option.label_key)}
            title={$t_store(option.label_key)}
            onclick={() => onChange(option.mode)}
        >
            <Icon size={16} aria-hidden="true" />
        </button>
    {/each}
</div>

<style>
    .channel-view-toggle {
        flex: 0 0 auto;
    }

    .channel-view-toggle .channel-view-toggle-btn {
        display: grid;
        width: 32px;
        height: 32px;
        padding: 0;
        place-items: center;
    }

    .channel-view-toggle .channel-view-toggle-btn:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }
</style>
