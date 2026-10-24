<script lang="ts">
    import CloudIcon from 'phosphor-svelte/lib/CloudIcon';
    import DownloadIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import ClockIcon from 'phosphor-svelte/lib/ClockIcon';
    import CircleCheckIcon from 'phosphor-svelte/lib/CheckCircleIcon';
    import RefreshCwIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import TriangleAlertIcon from 'phosphor-svelte/lib/WarningIcon';
    import CircleAlertIcon from 'phosphor-svelte/lib/WarningCircleIcon';
    import { itemStateDescriptor, opensQueue, type ItemState } from './item_state';
    import { current_locale, t_store } from '../../i18n/i18n';

    interface Props {
        state: ItemState;
        /** Opens the transfer queue. Omitted where there is no queue to open. */
        onOpenQueue?: () => void;
        size?: number;
    }

    let { state, onOpenQueue, size = 16 }: Props = $props();

    // One glyph family, resolved from the state table rather than chosen at
    // each call site, so a new state cannot arrive wearing a borrowed icon.
    const GLYPHS = {
        cloud: CloudIcon,
        download: DownloadIcon,
        clock: ClockIcon,
        'circle-check': CircleCheckIcon,
        'refresh-cw': RefreshCwIcon,
        'triangle-alert': TriangleAlertIcon,
        'circle-alert': CircleAlertIcon,
    } as const;

    // Reading $current_locale makes the descriptor's text re-resolve when the language changes.
    const info = $derived.by(() => {
        void $current_locale;
        return itemStateDescriptor(state);
    });
    const Glyph = $derived(GLYPHS[info.glyph as keyof typeof GLYPHS] ?? CloudIcon);
    const tappable = $derived(Boolean(onOpenQueue) && opensQueue(state));
</script>

{#if !info.marked}
    <!-- The resting state draws nothing: see ItemStateDescriptor.marked. -->
{:else if tappable}
    <button
        class="item-status is-tappable"
        data-tone={info.tone}
        data-spins={info.spins ? 'true' : 'false'}
        type="button"
        aria-label={$t_store('mobile.open_transfers_label', { label: info.label })}
        onclick={(event) => { event.stopPropagation(); onOpenQueue?.(); }}
    >
        <Glyph {size} aria-hidden="true" />
    </button>
{:else}
    <span
        class="item-status"
        data-tone={info.tone}
        data-spins={info.spins ? 'true' : 'false'}
        role="img"
        aria-label={info.label}
    >
        <Glyph {size} aria-hidden="true" />
    </span>
{/if}
