<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import CaretDownIcon from 'phosphor-svelte/lib/CaretDownIcon';
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import EyeIcon from 'phosphor-svelte/lib/EyeIcon';
    import PlusIcon from 'phosphor-svelte/lib/PlusIcon';
    import { t_store } from '../../i18n/i18n';
    import BrandIcon from './brand_icon.svelte';
    import { OPEN_SETTINGS_EVENT } from '../settings/settings_store';
    import { choose_model, configured_models, resolve_model, selected_model_id } from './ai_models_store';

    let open = $state(false);
    let root = $state<HTMLElement | null>(null);
    let trigger = $state<HTMLButtonElement | null>(null);

    const models = $derived($configured_models);
    // "Auto" is the default model; the trigger names the model that will answer.
    const current = $derived(resolve_model(models, $selected_model_id));
    const auto_checked = $derived($selected_model_id === '');

    function pick(id: string): void {
        choose_model(id);
        open = false;
    }

    function manage(): void {
        open = false;
        window.dispatchEvent(new CustomEvent(OPEN_SETTINGS_EVENT, { detail: { section: 'ai' } }));
    }

    function add_model(): void {
        window.dispatchEvent(new CustomEvent(OPEN_SETTINGS_EVENT, { detail: { section: 'ai' } }));
    }

    function on_pointer_down(event: PointerEvent): void {
        if (open && root && !root.contains(event.target as Node)) open = false;
    }

    function on_key_down(event: KeyboardEvent): void {
        if (!open || event.key !== 'Escape') return;
        open = false;
        trigger?.focus();
    }
</script>

<svelte:window onpointerdown={on_pointer_down} onkeydown={on_key_down} />

{#if models.length > 0}
    <div class="ai-model" bind:this={root}>
        <button
            bind:this={trigger}
            type="button"
            class="ai-model-btn"
            aria-haspopup="menu"
            aria-expanded={open ? 'true' : 'false'}
            aria-label={`${$t_store('ai.model_label')}: ${current?.display_name ?? ''}`}
            title={$t_store('ai.model_label')}
            onclick={() => (open = !open)}
        >
            {#if current}
                <BrandIcon provider={current.provider} size={16} />
                <span class="ai-model-name">{current.display_name}</span>
            {/if}
            <CaretDownIcon size={10} aria-hidden="true" />
        </button>

        {#if open}
            <div class="ai-model-menu" role="menu" aria-label={$t_store('ai.model_label')}>
                <button
                    type="button"
                    class="ai-model-item"
                    role="menuitemradio"
                    aria-checked={auto_checked ? 'true' : 'false'}
                    onclick={() => pick('')}
                >
                    <span class="ai-model-check" aria-hidden="true">{#if auto_checked}<CheckIcon size={12} />{/if}</span>
                    {#if current}<BrandIcon provider={current.provider} size={20} />{/if}
                    <span class="ai-model-text">
                        <span class="ai-model-name">{$t_store('ai.model_auto')}</span>
                        {#if current}<span class="ai-model-id">{current.display_name}</span>{/if}
                    </span>
                </button>

                {#each models as model (model.id)}
                    {@const checked = $selected_model_id === model.id}
                    <button
                        type="button"
                        class="ai-model-item"
                        role="menuitemradio"
                        aria-checked={checked ? 'true' : 'false'}
                        onclick={() => pick(model.id)}
                    >
                        <span class="ai-model-check" aria-hidden="true">{#if checked}<CheckIcon size={12} />{/if}</span>
                        <BrandIcon provider={model.provider} size={20} />
                        <span class="ai-model-text">
                            <span class="ai-model-line">
                                <span class="ai-model-name">{model.display_name}</span>
                                {#if model.is_default}
                                    <span class="ai-model-badge">{$t_store('ai.model_default_tag')}</span>
                                {/if}
                            </span>
                            <span class="ai-model-id">{model.model}</span>
                        </span>
                        {#if model.vision_enabled}
                            <span class="ai-model-vision" title={$t_store('settings_ai.vision_yes')}>
                                <EyeIcon size={14} aria-hidden="true" />
                                <span class="sr-only">{$t_store('settings_ai.vision_yes')}</span>
                            </span>
                        {/if}
                    </button>
                {/each}

                <div class="ai-model-sep" role="separator"></div>
                <button type="button" class="ai-model-item ai-model-manage" role="menuitem" onclick={manage}>
                    <span class="ai-model-check" aria-hidden="true"></span>
                    <span class="ai-model-text">
                        <span class="ai-model-name">{$t_store('ai.model_manage')}</span>
                    </span>
                </button>
            </div>
        {/if}
    </div>
{:else}
    <button type="button" class="ai-cta" onclick={add_model}>
        <PlusIcon size={10} aria-hidden="true" />
        {$t_store('ai.add_model_cta')}
    </button>
{/if}

<style>
    .ai-model {
        position: relative;
        min-width: 0;
        max-width: 200px;
    }

    .ai-model-btn {
        height: 24px;
        max-width: 100%;
        padding: 0 var(--space-1-5);
        display: inline-flex;
        align-items: center;
        gap: var(--space-1);
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        font: inherit;
        font-size: var(--type-xs);
        font-weight: var(--weight-medium);
        cursor: pointer;
    }

    .ai-model-btn:hover,
    .ai-model-btn[aria-expanded='true'] {
        color: var(--text-main);
        background: var(--color-surface-2);
    }

    .ai-model-btn:focus-visible,
    .ai-model-item:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    .ai-model-name {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .ai-model-menu {
        position: absolute;
        bottom: calc(100% + 6px);
        left: 0;
        z-index: 20;
        width: max-content;
        min-width: 240px;
        max-width: min(320px, 80vw);
        max-height: 60vh;
        overflow-y: auto;
        padding: var(--space-1);
        display: flex;
        flex-direction: column;
        gap: 2px;
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-surface-1);
        box-shadow: 0 8px 24px rgb(0 0 0 / 0.25);
    }

    .ai-model-item {
        width: 100%;
        min-height: 40px;
        padding: var(--space-1) var(--space-2);
        display: flex;
        align-items: center;
        gap: var(--space-2);
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-sm);
        text-align: left;
        cursor: pointer;
    }

    .ai-model-item:hover {
        background: var(--color-surface-2);
    }

    .ai-model-check {
        width: 12px;
        flex: none;
        display: inline-grid;
        place-items: center;
        color: var(--color-accent);
    }

    .ai-model-text {
        min-width: 0;
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 1px;
    }

    .ai-model-line {
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-1-5);
    }

    .ai-model-id {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: var(--text-muted);
        font-size: var(--type-xs);
    }

    .ai-model-badge {
        padding: 0 var(--space-1);
        border-radius: var(--radius-pill);
        background: var(--color-surface-2);
        color: var(--text-muted);
        font-size: var(--type-xs);
    }

    .ai-model-vision {
        flex: none;
        display: inline-grid;
        color: var(--text-muted);
    }

    .ai-model-sep {
        height: 1px;
        margin: var(--space-1) 0;
        background: var(--color-border-soft);
    }

    .ai-model-manage .ai-model-name {
        color: var(--color-accent);
    }

    .ai-cta {
        height: 24px;
        padding: 0 var(--space-2);
        display: inline-flex;
        align-items: center;
        gap: var(--space-1);
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--color-accent);
        font: inherit;
        font-size: var(--type-xs);
        font-weight: var(--weight-medium);
        cursor: pointer;
        white-space: nowrap;
    }

    .sr-only {
        position: absolute;
        width: 1px;
        height: 1px;
        overflow: hidden;
        clip: rect(0, 0, 0, 0);
        white-space: nowrap;
    }
</style>
