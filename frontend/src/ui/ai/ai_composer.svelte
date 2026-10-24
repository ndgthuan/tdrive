<script lang="ts">
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import PaperPlaneRightIcon from 'phosphor-svelte/lib/PaperPlaneRightIcon';
    import MicrophoneIcon from 'phosphor-svelte/lib/MicrophoneIcon';
    import PlusIcon from 'phosphor-svelte/lib/PlusIcon';
    import { breadcrumbPath } from '../chrome/breadcrumb_store';
    import { send_text } from './ai_chat';
    import { drive_name } from './ai_scope';
    import { busy, include_folder, mode, models, selected_model_id } from './ai_store';
    import { t_store } from '../../i18n/i18n';

    let text = $state('');
    let input = $state<HTMLTextAreaElement | null>(null);

    const folder = $derived($breadcrumbPath.length > 0 ? $breadcrumbPath[$breadcrumbPath.length - 1] : null);
    const can_send = $derived(text.trim() !== '' && !$busy);

    // The textarea grows with its content up to a limit, then scrolls.
    function autosize(): void {
        if (!input) return;
        input.style.height = 'auto';
        input.style.height = `${Math.min(input.scrollHeight, 160)}px`;
    }

    async function submit(): Promise<void> {
        if (!can_send) return;
        const message = text;
        text = '';
        autosize();
        await send_text(message);
    }

    function on_keydown(event: KeyboardEvent): void {
        if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
        event.preventDefault();
        void submit();
    }

    function drive_chip(): void {
        include_folder.set(false);
    }

    function remove_folder_chip(): void {
        include_folder.set(false);
    }

    function add_folder_chip(): void {
        include_folder.set(true);
    }

    function open_settings(): void {
        window.dispatchEvent(new CustomEvent('tdrive:open_settings', { detail: { section: 'ai' } }));
    }

</script>

<form class="ai-composer" onsubmit={(event) => { event.preventDefault(); void submit(); }}>
    <div class="ai-context" aria-label={$t_store('ai.context_label')}>
        <button
            type="button"
            class="ai-chip"
            class:is-current={!$include_folder || !folder}
            aria-pressed={!$include_folder || !folder ? 'true' : 'false'}
            title={$t_store('ai.scope_drive_root')}
            onclick={drive_chip}
        >
            <FolderIcon size={12} aria-hidden="true" />
            {drive_name()}
        </button>
        {#if folder && $include_folder}
            <span class="ai-chip is-current">
                <FolderIcon size={12} aria-hidden="true" />
                <span class="ai-chip-name">{folder.name}</span>
                <button
                    type="button"
                    class="ai-chip-x"
                    aria-label={$t_store('ai.scope_remove_folder')}
                    title={$t_store('ai.scope_drive_root')}
                    onclick={remove_folder_chip}
                >
                    <XIcon size={10} aria-hidden="true" />
                </button>
            </span>
        {:else if folder}
            <button type="button" class="ai-chip is-add" onclick={add_folder_chip}>
                <PlusIcon size={10} aria-hidden="true" />
                {folder.name}
            </button>
        {/if}
    </div>

    <textarea
        bind:this={input}
        bind:value={text}
        class="ai-input"
        rows="2"
        placeholder={$t_store('ai.composer_placeholder')}
        aria-label={$t_store('ai.composer_placeholder')}
        onkeydown={on_keydown}
        oninput={autosize}
    ></textarea>

    <div class="ai-controls">
        <select class="ai-select" bind:value={$mode} aria-label={$t_store('ai.mode_label')} title={$t_store(`ai.mode_${$mode}_hint`)}>
            <option value="agent">{$t_store('ai.mode_agent')}</option>
            <option value="ask">{$t_store('ai.mode_ask')}</option>
        </select>

        {#if $models.length > 0}
            <select class="ai-select" bind:value={$selected_model_id} aria-label={$t_store('ai.model_label')}>
                <option value="">{$t_store('ai.model_auto')}</option>
                {#each $models as model (model.id)}
                    <option value={model.id}>
                        {model.display_name}{model.is_default ? ` (${$t_store('ai.model_default_tag')})` : ''}
                    </option>
                {/each}
            </select>
        {:else}
            <button type="button" class="ai-cta" onclick={open_settings}>{$t_store('ai.add_model_cta')}</button>
        {/if}

        <span class="ai-spacer"></span>

        <button
            type="button"
            class="ai-icon-btn"
            disabled
            aria-label={$t_store('ai.voice_label')}
            title={$t_store('ai.voice_coming_soon')}
        >
            <MicrophoneIcon size={14} aria-hidden="true" />
        </button>
        <button
            type="submit"
            class="ai-send"
            aria-label={$t_store('ai.send')}
            title={$t_store('ai.send')}
            disabled={!can_send}
        >
            <PaperPlaneRightIcon size={14} aria-hidden="true" />
        </button>
    </div>
</form>

<style>
    .ai-composer {
        flex: none;
        margin: 0 var(--space-3) var(--space-3);
        padding: var(--space-2) var(--space-2-5);
        display: flex;
        flex-direction: column;
        gap: var(--space-1-5);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-md);
        background: var(--color-canvas);
    }

    .ai-context {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-1-5);
        min-width: 0;
    }

    .ai-chip {
        height: 24px;
        max-width: 180px;
        padding: 0 var(--space-2);
        display: inline-flex;
        align-items: center;
        gap: var(--space-1-5);
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        background: var(--color-surface-2);
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-xs);
        font-weight: var(--weight-medium);
        white-space: nowrap;
        cursor: pointer;
    }

    .ai-chip.is-current {
        border-color: color-mix(in srgb, var(--color-accent) 45%, transparent);
    }

    .ai-chip.is-add {
        color: var(--text-muted);
        border-style: dashed;
    }

    .ai-chip-name {
        overflow: hidden;
        text-overflow: ellipsis;
        min-width: 0;
    }

    .ai-chip-x {
        width: 14px;
        height: 14px;
        display: grid;
        place-items: center;
        padding: 0;
        border: 0;
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
    }

    .ai-composer:focus-within {
        border-color: color-mix(in srgb, var(--color-accent) 55%, var(--color-border-soft));
    }

    .ai-chip:focus-visible,
    .ai-chip-x:focus-visible,
    .ai-icon-btn:focus-visible,
    .ai-send:focus-visible,
    .ai-select:focus-visible,
    .ai-cta:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    .ai-input {
        width: 100%;
        min-height: 40px;
        max-height: 160px;
        padding: 0;
        border: 0;
        background: transparent;
        color: var(--text-main);
        font: inherit;
        font-size: var(--type-base);
        line-height: var(--leading-normal);
        resize: none;
        outline: none;
    }

    .ai-input::placeholder {
        color: var(--text-muted);
    }

    .ai-controls {
        display: flex;
        align-items: center;
        gap: var(--space-1);
        min-width: 0;
    }

    .ai-select {
        height: 24px;
        max-width: 140px;
        padding: 0 var(--space-1-5);
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        font: inherit;
        font-size: var(--type-xs);
        font-weight: var(--weight-medium);
        cursor: pointer;
    }

    .ai-select:hover {
        color: var(--text-main);
        background: var(--color-surface-2);
    }

    .ai-cta {
        height: 24px;
        padding: 0 var(--space-2);
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

    .ai-spacer {
        flex: 1;
    }

    .ai-icon-btn {
        width: 24px;
        height: 24px;
        display: grid;
        place-items: center;
        padding: 0;
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
    }

    .ai-icon-btn:disabled {
        opacity: 0.45;
        cursor: not-allowed;
    }

    .ai-send {
        width: 26px;
        height: 26px;
        display: grid;
        place-items: center;
        padding: 0;
        border: 0;
        border-radius: var(--radius-sm);
        background: var(--color-accent);
        color: var(--color-on-accent);
        cursor: pointer;
    }

    .ai-send:disabled {
        background: var(--color-border-soft);
        color: var(--text-muted);
        cursor: default;
    }
</style>
