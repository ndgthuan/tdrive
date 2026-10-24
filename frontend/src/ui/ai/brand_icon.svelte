<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import PlugIcon from 'phosphor-svelte/lib/PlugIcon';
    import { Provider } from '../../../bindings/TDrive/backend/ai/models';
    import { brand_svg } from './brand_icon';

    interface Props {
        provider: Provider;
        /** Rendered square size in pixels. */
        size?: number;
    }

    let { provider, size = 20 }: Props = $props();

    const svg = $derived(brand_svg(provider));
</script>

<span class="brand-icon" style={`width:${size}px;height:${size}px;font-size:${size}px`} aria-hidden="true">
    {#if svg}
        {@html svg}
    {:else}
        <PlugIcon size={size} weight="regular" />
    {/if}
</span>

<style>
    .brand-icon {
        flex: none;
        display: inline-grid;
        place-items: center;
        color: var(--text-main);
        line-height: 1;
    }

    .brand-icon :global(svg) {
        display: block;
        width: 100%;
        height: 100%;
    }
</style>
