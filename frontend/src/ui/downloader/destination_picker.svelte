<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import CaretDownIcon from 'phosphor-svelte/lib/CaretDownIcon';
    import CaretLeftIcon from 'phosphor-svelte/lib/CaretLeftIcon';
    import CaretRightIcon from 'phosphor-svelte/lib/CaretRightIcon';
    import FolderIcon from 'phosphor-svelte/lib/FolderIcon';
    import FolderPlusIcon from 'phosphor-svelte/lib/FolderPlusIcon';
    import FolderSimpleIcon from 'phosphor-svelte/lib/FolderSimpleIcon';
    import {
        create_destination_folder,
        list_destination_folders,
        type DestinationFolder,
    } from '../../api/media_download';
    import { t_store } from '../../i18n/i18n';
    import { drive_root_name, sidebar_drives } from '../chrome/drive_root_name';
    import { sidebarState } from '../sidebar/sidebar_store';
    import {
        destination_place,
        save_destination,
        type DestinationCrumb,
        type DownloadDestination,
    } from './downloader_destination';

    interface Props {
        destination: DownloadDestination;
        disabled?: boolean;
    }

    let { destination = $bindable(), disabled = false }: Props = $props();

    let open = $state(false);
    // The drive and folder the popover is browsing. It is separate from the
    // chosen destination until "Select this folder" is pressed.
    let browse_drive = $state(0);
    let browse_path = $state<DestinationCrumb[]>([]);
    let folders = $state<DestinationFolder[]>([]);
    let loading = $state(false);
    let error = $state('');
    let creating = $state(false);
    let new_name = $state('');
    let creating_busy = $state(false);
    let root_el = $state<HTMLElement | null>(null);
    let trigger_el = $state<HTMLButtonElement | null>(null);
    // Each browse bumps the epoch, so a slow listing cannot overwrite a newer one.
    let browse_epoch = 0;

    const drives = $derived(sidebar_drives($sidebarState));

    function drive_name(drive_id: number): string {
        const drive = drives.find((entry) => entry.id === drive_id) ?? null;
        return drive_root_name(drive, $t_store);
    }

    const place = $derived(destination_place(drive_name(destination.drive_id), destination));
    const parent_id = $derived(browse_path[browse_path.length - 1]?.id ?? '');

    function message_of(cause: unknown): string {
        if (cause instanceof Error && cause.message) return cause.message;
        return $t_store('downloader.error_generic');
    }

    async function browse(drive_id: number, path: DestinationCrumb[]): Promise<void> {
        const epoch = ++browse_epoch;
        browse_drive = drive_id;
        browse_path = path;
        loading = true;
        error = '';
        const parent = path[path.length - 1]?.id ?? '';
        try {
            const listing = await list_destination_folders(drive_id, parent);
            if (epoch !== browse_epoch) return;
            folders = listing.folders ?? [];
        } catch (cause) {
            if (epoch !== browse_epoch) return;
            if (path.length > 0) {
                // The folder is gone or unreachable: show the drive root and say why.
                const message = message_of(cause);
                await browse(drive_id, []);
                error = message;
                return;
            }
            folders = [];
            error = message_of(cause);
        } finally {
            if (epoch === browse_epoch) loading = false;
        }
    }

    function toggle(): void {
        if (open) {
            close();
            return;
        }
        open = true;
        void browse(destination.drive_id, destination.path);
    }

    function close(): void {
        if (!open) return;
        open = false;
        creating = false;
        new_name = '';
        error = '';
        browse_epoch++;
        loading = false;
    }

    function close_to_trigger(): void {
        close();
        trigger_el?.focus();
    }

    function go_up(): void {
        if (browse_path.length === 0) return;
        void browse(browse_drive, browse_path.slice(0, -1));
    }

    function select_here(): void {
        destination = {
            drive_id: browse_drive,
            folder_id: parent_id,
            path: browse_path.map((crumb) => ({ id: crumb.id, name: crumb.name })),
        };
        save_destination(destination);
        close_to_trigger();
    }

    async function create_folder(): Promise<void> {
        const name = new_name.trim();
        if (!name || creating_busy) return;
        creating_busy = true;
        error = '';
        try {
            await create_destination_folder(browse_drive, parent_id, name);
            new_name = '';
            creating = false;
            await browse(browse_drive, browse_path);
        } catch (cause) {
            error = message_of(cause);
        } finally {
            creating_busy = false;
        }
    }

    // Escape closes the popover only. The listener runs in the capture phase and
    // stops the event there, so the dialog underneath does not close with it.
    $effect(() => {
        if (!open) return;
        const on_key = (event: KeyboardEvent): void => {
            if (event.key !== 'Escape') return;
            event.stopPropagation();
            event.preventDefault();
            close_to_trigger();
        };
        window.addEventListener('keydown', on_key, true);
        return () => window.removeEventListener('keydown', on_key, true);
    });

    function on_window_pointer(event: PointerEvent): void {
        if (!open || !root_el) return;
        if (event.target instanceof Node && root_el.contains(event.target)) return;
        close();
    }
</script>

<svelte:window onpointerdown={on_window_pointer} />

<div class="dest-picker" bind:this={root_el}>
    <button
        bind:this={trigger_el}
        class="dest-trigger"
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls="downloader-destination-panel"
        title={$t_store('downloader.destination_choose')}
        {disabled}
        onclick={toggle}
    >
        <FolderSimpleIcon size={16} aria-hidden="true" />
        <span class="dest-label">{$t_store('downloader.destination_save_to')}</span>
        <span class="dest-place">{place}</span>
        <CaretDownIcon size={12} aria-hidden="true" class="dest-caret" />
    </button>

    {#if open}
        <div
            id="downloader-destination-panel"
            class="dest-panel"
            role="dialog"
            aria-label={$t_store('downloader.destination_choose')}
            tabindex="-1"
        >
            <div class="dest-drives" role="group" aria-label={$t_store('downloader.destination_drives')}>
                {#each drives as drive (drive.id)}
                    <button
                        type="button"
                        class="dest-drive"
                        class:is-active={drive.id === browse_drive}
                        aria-pressed={drive.id === browse_drive}
                        onclick={() => browse(drive.id, [])}
                    >
                        {drive_root_name(drive, $t_store)}
                    </button>
                {/each}
            </div>

            <div class="dest-nav">
                <button
                    type="button"
                    class="dest-back"
                    aria-label={$t_store('common.back')}
                    disabled={browse_path.length === 0 || loading}
                    onclick={go_up}
                >
                    <CaretLeftIcon size={14} aria-hidden="true" />
                </button>
                <nav class="dest-crumbs" aria-label={$t_store('downloader.destination_choose')}>
                    <button
                        type="button"
                        class="dest-crumb"
                        disabled={browse_path.length === 0}
                        onclick={() => browse(browse_drive, [])}
                    >
                        {drive_name(browse_drive)}
                    </button>
                    {#each browse_path as segment, idx (segment.id)}
                        <span class="dest-sep" aria-hidden="true">/</span>
                        <button
                            type="button"
                            class="dest-crumb"
                            disabled={idx === browse_path.length - 1}
                            onclick={() => browse(browse_drive, browse_path.slice(0, idx + 1))}
                        >
                            {segment.name}
                        </button>
                    {/each}
                </nav>
            </div>

            <div class="dest-list" aria-live="polite">
                {#if loading}
                    <p class="dest-note">{$t_store('downloader.destination_loading')}</p>
                {:else if folders.length === 0}
                    <p class="dest-note">{$t_store('downloader.destination_empty')}</p>
                {:else}
                    {#each folders as folder (folder.id)}
                        <button
                            type="button"
                            class="dest-folder"
                            onclick={() => browse(browse_drive, [...browse_path, { id: folder.id, name: folder.name }])}
                        >
                            <FolderIcon size={16} weight="fill" aria-hidden="true" />
                            <span class="dest-folder-name">{folder.name}</span>
                            <CaretRightIcon size={12} aria-hidden="true" />
                        </button>
                    {/each}
                {/if}
            </div>

            {#if error}
                <p class="dest-error" role="alert">{error}</p>
            {/if}

            {#if creating}
                <form
                    class="dest-new"
                    onsubmit={(event) => {
                        event.preventDefault();
                        void create_folder();
                    }}
                >
                    <input
                        type="text"
                        autocomplete="off"
                        maxlength="255"
                        aria-label={$t_store('downloader.destination_folder_name')}
                        placeholder={$t_store('downloader.destination_folder_name')}
                        bind:value={new_name}
                    />
                    <button class="secondary-btn" type="submit" disabled={creating_busy || !new_name.trim()}>
                        {$t_store('downloader.destination_create')}
                    </button>
                </form>
            {/if}

            <div class="dest-actions">
                <button class="dest-ghost" type="button" aria-expanded={creating} onclick={() => (creating = !creating)}>
                    <FolderPlusIcon size={14} aria-hidden="true" />
                    {$t_store('downloader.destination_new_folder')}
                </button>
                <button class="primary-btn" type="button" disabled={loading} onclick={select_here}>
                    {$t_store('downloader.destination_select')}
                </button>
            </div>
        </div>
    {/if}
</div>

<style>
    .dest-picker {
        position: relative;
        min-width: 0;
    }

    .dest-trigger {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        max-width: 100%;
        min-height: var(--control-md);
        padding: 0 10px;
        border: 1px solid var(--color-border);
        border-radius: var(--radius-sm);
        background: var(--color-canvas);
        color: var(--text-muted);
        font: inherit;
        font-size: 12px;
        cursor: pointer;
    }

    .dest-trigger:hover:not(:disabled),
    .dest-trigger[aria-expanded='true'] {
        border-color: rgb(127 156 196 / 0.5);
    }

    .dest-trigger:focus-visible {
        outline: none;
        box-shadow: 0 0 0 3px rgb(127 156 196 / 0.12);
    }

    .dest-trigger:disabled {
        opacity: 0.45;
        cursor: not-allowed;
    }

    .dest-label {
        flex: none;
    }

    .dest-place {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: var(--text-main);
    }

    .dest-trigger :global(.dest-caret) {
        flex: none;
        opacity: 0.7;
    }

    .dest-panel {
        position: absolute;
        left: 0;
        bottom: calc(100% + 8px);
        z-index: 5;
        display: grid;
        gap: 10px;
        width: 340px;
        max-width: calc(100vw - 32px);
        padding: 12px;
        border: 1px solid var(--color-border);
        border-radius: 8px;
        background: var(--color-canvas);
        box-shadow: 0 12px 32px rgb(0 0 0 / 0.5);
    }

    .dest-panel:focus {
        outline: none;
    }

    .dest-drives {
        display: flex;
        flex-wrap: wrap;
        gap: 6px;
    }

    .dest-drive {
        max-width: 100%;
        min-height: 26px;
        padding: 0 10px;
        overflow: hidden;
        border: 1px solid var(--color-border-soft);
        border-radius: 999px;
        background: transparent;
        color: var(--text-muted);
        font: inherit;
        font-size: 12px;
        text-overflow: ellipsis;
        white-space: nowrap;
        cursor: pointer;
    }

    .dest-drive:hover {
        color: var(--text-main);
    }

    .dest-drive.is-active {
        border-color: rgb(127 156 196 / 0.5);
        color: var(--text-main);
        background: rgb(127 156 196 / 0.12);
    }

    .dest-nav {
        display: flex;
        align-items: center;
        gap: 6px;
        min-width: 0;
    }

    .dest-back {
        flex: none;
        display: inline-grid;
        place-items: center;
        width: 26px;
        height: 26px;
        border: 1px solid var(--color-border-soft);
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
    }

    .dest-back:disabled {
        opacity: 0.4;
        cursor: default;
    }

    .dest-crumbs {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 2px;
        min-width: 0;
        font-size: 12px;
    }

    .dest-crumb {
        padding: 2px 4px;
        border: 0;
        border-radius: var(--radius-sm);
        background: none;
        color: var(--text-muted);
        font: inherit;
        cursor: pointer;
    }

    .dest-crumb:hover:not(:disabled) {
        color: var(--text-main);
    }

    .dest-crumb:disabled {
        color: var(--text-main);
        cursor: default;
    }

    .dest-sep {
        color: var(--text-muted);
    }

    .dest-list {
        display: grid;
        gap: 2px;
        max-height: 200px;
        overflow-y: auto;
        min-height: 60px;
    }

    .dest-note {
        margin: 0;
        padding: 14px 4px;
        color: var(--text-muted);
        font-size: 12px;
    }

    .dest-folder {
        display: grid;
        grid-template-columns: auto minmax(0, 1fr) auto;
        align-items: center;
        gap: 8px;
        min-height: 30px;
        padding: 0 8px;
        border: 0;
        border-radius: var(--radius-sm);
        background: transparent;
        color: var(--text-main);
        font: inherit;
        font-size: 13px;
        text-align: left;
        cursor: pointer;
    }

    .dest-folder :global(svg) {
        color: var(--color-accent);
    }

    .dest-folder:hover {
        background: rgb(127 156 196 / 0.1);
    }

    .dest-folder-name {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .dest-folder :global(svg:last-child) {
        color: var(--text-muted);
    }

    .dest-error {
        margin: 0;
        color: var(--color-danger, #e5484d);
        font-size: 12px;
    }

    .dest-new {
        display: flex;
        gap: 6px;
    }

    .dest-new input {
        flex: 1;
        min-width: 0;
    }

    .dest-actions {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
    }

    .dest-ghost {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        white-space: nowrap;
        padding: 0 4px;
        border: 0;
        background: none;
        color: var(--text-muted);
        font: inherit;
        font-size: 12px;
        cursor: pointer;
    }

    .dest-ghost:hover {
        color: var(--color-accent);
    }

    @media (max-width: 560px) {
        .dest-panel {
            position: fixed;
            left: 12px;
            right: 12px;
            width: auto;
        }
    }
</style>
