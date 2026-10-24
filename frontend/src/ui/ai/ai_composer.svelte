<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import PaperPlaneRightIcon from 'phosphor-svelte/lib/PaperPlaneRightIcon';
    import MicrophoneIcon from 'phosphor-svelte/lib/MicrophoneIcon';
    import PlusIcon from 'phosphor-svelte/lib/PlusIcon';
    import CheckIcon from 'phosphor-svelte/lib/CheckIcon';
    import { breadcrumbPath } from '../chrome/breadcrumb_store';
    import { access_from_snapshot, drive_roles } from '../members/members_store';
    import { drive_root_name, sidebar_drives } from '../chrome/drive_root_name';
    import { sidebarState } from '../sidebar/sidebar_store';
    import { send_text } from './ai_chat';
    import { open_drive_id, pick_scope, scope_drive } from './ai_scope';
    import AiModelPicker from './ai_model_picker.svelte';
    import { busy, mode } from './ai_store';
    import { t_store } from '../../i18n/i18n';

    let text = $state('');
    let input = $state<HTMLTextAreaElement | null>(null);
    let scope_open = $state(false);
    let scope_root = $state<HTMLElement | null>(null);
    let scope_trigger = $state<HTMLButtonElement | null>(null);

    const folder = $derived($breadcrumbPath.length > 0 ? $breadcrumbPath[$breadcrumbPath.length - 1] : null);
    const can_send = $derived(text.trim() !== '' && !$busy);
    // Drives the user can change: the personal drive and every shared drive the sidebar lists.
    const drives = $derived(sidebar_drives($sidebarState));
    const chosen_drive_name = $derived(drive_root_name($scope_drive.drive, $t_store));
    // A shared scope the caller can only read gets the same View only pill as the drive header.
    const scope_view_only = $derived(
        Boolean($scope_drive.drive && $scope_drive.drive.kind === 'shared')
        && access_from_snapshot($scope_drive.drive_id, true, $drive_roles[$scope_drive.drive_id]).viewOnly,
    );
    // The folder chip only means something inside the open drive.
    const folder_in_scope = $derived(Boolean(folder) && $scope_drive.folder && $scope_drive.drive_id === open_drive_id());
    const folder_is_open = $derived(Boolean(folder) && $scope_drive.drive_id === open_drive_id());

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

    function toggle_scope(): void {
        scope_open = !scope_open;
    }

    function pick_drive(drive_id: number): void {
        pick_scope(drive_id, false);
        scope_open = false;
    }

    function pick_folder(): void {
        pick_scope(open_drive_id(), true);
        scope_open = false;
    }

    function pick_root(): void {
        pick_scope($scope_drive.drive_id, false);
        scope_open = false;
    }

    function on_pointer_down(event: PointerEvent): void {
        if (scope_open && scope_root && !scope_root.contains(event.target as Node)) scope_open = false;
    }

    function on_key_down(event: KeyboardEvent): void {
        if (!scope_open || event.key !== 'Escape') return;
        scope_open = false;
        scope_trigger?.focus();
    }

    function remove_folder_chip(): void {
        pick_scope($scope_drive.drive_id, false);
    }

    function add_folder_chip(): void {
        pick_scope(open_drive_id(), true);
    }

</script>

<svelte:window onpointerdown={on_pointer_down} onkeydown={on_key_down} />

<form class="ai-composer" onsubmit={(event) => { event.preventDefault(); void submit(); }}>
    <div class="ai-context" aria-label={$t_store('ai.context_label')}>
        <div class="ai-scope" bind:this={scope_root}>
            <button
                bind:this={scope_trigger}
                type="button"
                class="ai-chip is-current"
                aria-haspopup="menu"
                aria-expanded={scope_open ? 'true' : 'false'}
                aria-label={$t_store('ai.scope_menu_label')}
                title={$t_store('ai.scope_menu_label')}
                onclick={toggle_scope}
            >
                <FolderIcon size={12} aria-hidden="true" />
                <span class="ai-chip-name">{chosen_drive_name}</span>
            </button>
            {#if scope_view_only}
                <span class="ai-scope-view-only" title={$t_store('members.view_only_tooltip')}>{$t_store('members.view_only')}</span>
            {/if}

            {#if scope_open}
                <div class="ai-scope-menu" role="menu" aria-label={$t_store('ai.scope_menu_label')}>
                    {#if folder_is_open}
                        <button type="button" class="ai-scope-item" role="menuitemradio" aria-checked={folder_in_scope ? 'true' : 'false'} onclick={pick_folder}>
                            <span class="ai-scope-check" aria-hidden="true">{#if folder_in_scope}<CheckIcon size={12} />{/if}</span>
                            <span class="ai-scope-name">{$t_store('ai.scope_current_folder')}</span>
                            <span class="ai-scope-sub">{folder?.name}</span>
                        </button>
                    {/if}
                    <button
                        type="button"
                        class="ai-scope-item"
                        role="menuitemradio"
                        aria-checked={!folder_in_scope ? 'true' : 'false'}
                        onclick={pick_root}
                    >
                        <span class="ai-scope-check" aria-hidden="true">{#if !folder_in_scope}<CheckIcon size={12} />{/if}</span>
                        <span class="ai-scope-name">{$t_store('ai.scope_root_item')}</span>
                        <span class="ai-scope-sub">{chosen_drive_name}</span>
                    </button>

                    <div class="ai-scope-heading" role="presentation">{$t_store('ai.scope_drives_heading')}</div>
                    {#each drives as drive (drive.id)}
                        {@const checked = drive.id === $scope_drive.drive_id}
                        <button type="button" class="ai-scope-item" role="menuitemradio" aria-checked={checked ? 'true' : 'false'} onclick={() => pick_drive(drive.id)}>
                            <span class="ai-scope-check" aria-hidden="true">{#if checked}<CheckIcon size={12} />{/if}</span>
                            <span class="ai-scope-name">{drive_root_name(drive, $t_store)}</span>
                        </button>
                    {/each}
                </div>
            {/if}
        </div>

        {#if folder && folder_in_scope}
            <span class="ai-chip is-current">
                <FolderIcon size={12} aria-hidden="true" />
                <span class="ai-chip-name">{folder.name}</span>
                <button
                    type="button"
                    class="ai-chip-x"
                    aria-label={$t_store('ai.scope_remove_folder')}
                    title={$t_store('ai.scope_root_item')}
                    onclick={remove_folder_chip}
                >
                    <XIcon size={10} aria-hidden="true" />
                </button>
            </span>
        {:else if folder && folder_is_open}
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

        <AiModelPicker />

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
    .ai-scope-view-only {
        display: inline-flex;
        align-items: center;
        padding: 2px 8px;
        border: 1px solid var(--border);
        border-radius: var(--radius-pill);
        color: var(--text-muted);
        font-size: 0.7rem;
        white-space: nowrap;
    }
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
        transition:
            border-color var(--motion-fast) var(--ease-standard),
            box-shadow var(--motion-fast) var(--ease-standard);
    }

    .ai-context {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-1-5);
        min-width: 0;
    }

    .ai-scope {
        position: relative;
        min-width: 0;
        max-width: 100%;
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

    .ai-scope-menu {
        position: absolute;
        bottom: calc(100% + 6px);
        left: 0;
        z-index: 20;
        width: max-content;
        min-width: 220px;
        max-width: min(300px, 80vw);
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

    .ai-scope-item {
        width: 100%;
        min-height: 32px;
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

    .ai-scope-item:hover {
        background: var(--color-surface-2);
    }

    .ai-scope-check {
        width: 12px;
        flex: none;
        color: var(--color-accent);
        font-size: var(--type-xs);
    }

    .ai-scope-name {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .ai-scope-sub {
        margin-left: auto;
        padding-left: var(--space-2);
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: var(--text-muted);
        font-size: var(--type-xs);
    }

    .ai-scope-heading {
        margin-top: var(--space-1);
        padding: var(--space-1) var(--space-2) 2px;
        color: var(--text-muted);
        font-size: var(--type-xs);
        font-weight: var(--weight-medium);
    }

    /* The composer is the focus target: its border takes the muted accent and
       a soft ring, and the textarea inside draws nothing of its own. The same
       treatment is used by every text field in the panel and the settings. */
    .ai-composer:focus-within {
        border-color: rgb(127 156 196 / 0.5);
        box-shadow: 0 0 0 3px rgb(127 156 196 / 0.12);
    }

    .ai-chip:focus-visible,
    .ai-chip-x:focus-visible,
    .ai-icon-btn:focus-visible,
    .ai-send:focus-visible,
    .ai-scope-item:focus-visible {
        outline: none;
        box-shadow: 0 0 0 2px var(--color-accent);
    }

    /* Inside the composer the container's ring is the only focus mark. */
    .ai-select:focus,
    .ai-select:focus-visible {
        outline: none;
        box-shadow: none;
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
        box-shadow: none;
    }

    .ai-input:focus,
    .ai-input:focus-visible {
        outline: none;
        box-shadow: none;
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
