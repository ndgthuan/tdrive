<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircleIcon';
    import ClipboardTextIcon from 'phosphor-svelte/lib/ClipboardTextIcon';
    import WarningCircleIcon from 'phosphor-svelte/lib/WarningCircleIcon';
    import XCircleIcon from 'phosphor-svelte/lib/XCircleIcon';
    import { onDestroy, onMount } from 'svelte';
    import {
        cancel_download,
        install_tool,
        list_downloads,
        probe_link,
        read_tool_status,
        set_tool_path,
        start_download,
        subscribe_media_progress,
        subscribe_media_setup,
        update_ytdlp,
        type media_progress_event,
        type media_setup_event,
        type ProbeFormat,
        type ProbeResult,
        type Status,
    } from '../../api/media_download';
    import { t_store } from '../../i18n/i18n';
    import { refreshFiles } from '../../modules/file_list';
    import { navigateToFolder, navigateToIndex } from '../../modules/navigation';
    import ModalShell from '../modals/modal_shell.svelte';
    import ProgressBar from '../progress_bar.svelte';
    import { sidebarState } from '../sidebar/sidebar_store';
    import DestinationPicker from './destination_picker.svelte';
    import {
        destination_at_root,
        read_saved_destination,
        starting_destination,
        type DownloadDestination,
    } from './downloader_destination';
    import {
        clamp_percent,
        describe_format,
        format_clock,
        format_eta,
        format_speed,
        is_active_stage,
        is_valid_link,
        join_parts,
        link_host,
        setup_stage_key,
        stage_key,
        is_terminal_stage,
    } from './downloader_format';
    import {
        apply_progress_rows,
        attach_request,
        close_downloader,
        download_rows,
        downloader_open,
        load_download_rows,
        type download_row,
    } from './downloader_state';

    // Quick options, in the order the backend's presets use. The labels are
    // translated here; the values are what Start accepts.
    const PRESET_OPTIONS: Array<{ value: string; label_key: string; audio: boolean }> = [
        { value: 'best', label_key: 'downloader.preset_best', audio: false },
        { value: '1080p', label_key: 'downloader.preset_1080p', audio: false },
        { value: '720p', label_key: 'downloader.preset_720p', audio: false },
        { value: '480p', label_key: 'downloader.preset_480p', audio: false },
        { value: 'audio_mp3', label_key: 'downloader.preset_audio_mp3', audio: true },
        { value: 'audio_m4a', label_key: 'downloader.preset_audio_m4a', audio: true },
    ];

    let tool_status = $state<Status | null>(null);
    let status_error = $state('');
    let setup_busy = $state('');
    let setup_stage = $state('');
    let setup_percent = $state(0);
    let setup_error = $state('');
    let update_message = $state('');
    let custom_path = $state('');
    let path_error = $state('');
    // Where the next download is saved. The picker edits it; it starts at My Drive's root.
    let destination = $state<DownloadDestination>(destination_at_root(0));

    let url = $state('');
    let url_error = $state('');
    let probe = $state<ProbeResult | null>(null);
    let probe_for = $state('');
    let probe_loading = $state(false);
    let probe_error = $state('');
    let choice = $state('best');
    let starting = $state(false);
    let start_error = $state('');
    let action_error = $state('');

    const yt_dlp_ready = $derived(tool_status?.yt_dlp.found ?? false);
    const ffmpeg_ready = $derived(tool_status?.ffmpeg.found ?? false);
    const can_install = $derived(tool_status?.can_install ?? false);
    // Go sends an empty slice as null; the list is normalised once here.
    const formats = $derived(probe?.formats ?? []);
    const chosen_format = $derived(formats.find((format) => format.choice === choice) ?? null);
    const start_disabled = $derived(starting || !yt_dlp_ready || !is_valid_link(url) || !choice);
    const active_drive_id = $derived($sidebarState.activeChannelId ?? 0);

    function message_of(cause: unknown, fallback: string): string {
        if (cause instanceof Error && cause.message) return cause.message;
        return fallback;
    }

    async function load_status(): Promise<void> {
        status_error = '';
        try {
            tool_status = await read_tool_status();
        } catch (cause) {
            status_error = message_of(cause, $t_store('downloader.error_generic'));
        }
    }

    async function load_jobs(): Promise<void> {
        try {
            load_download_rows(await list_downloads());
        } catch (cause) {
            action_error = message_of(cause, $t_store('downloader.error_generic'));
        }
    }

    function reset_form(): void {
        url_error = '';
        probe_error = '';
        start_error = '';
        action_error = '';
        setup_error = '';
        path_error = '';
        update_message = '';
        probe = null;
        probe_for = '';
        choice = 'best';
        destination = starting_destination(read_saved_destination(), $sidebarState);
    }

    let was_open = false;
    $effect(() => {
        const open = $downloader_open;
        if (open && !was_open) {
            reset_form();
            void load_status();
            void load_jobs();
        }
        was_open = open;
    });

    function on_progress(event: media_progress_event): void {
        download_rows.update((rows) => apply_progress_rows(rows, event));
        // A finished file is already in the drive; refresh the list so it shows.
        if (event.stage === 'done') refreshFiles({ background: true });
    }

    function on_setup(event: media_setup_event): void {
        setup_stage = event.stage;
        setup_percent = event.percent;
        if (event.stage === 'error') {
            setup_error = event.error ?? $t_store('downloader.error_generic');
            setup_busy = '';
            return;
        }
        if (event.stage === 'done') {
            setup_busy = '';
            void load_status();
        }
    }

    let unsubscribers: Array<() => void> = [];
    onMount(() => {
        unsubscribers = [subscribe_media_progress(on_progress), subscribe_media_setup(on_setup)];
    });
    onDestroy(() => {
        for (const stop of unsubscribers) stop();
        unsubscribers = [];
    });

    function close(): void {
        close_downloader();
    }

    async function paste_link(): Promise<void> {
        url_error = '';
        try {
            url = (await navigator.clipboard.readText()).trim();
        } catch {
            url_error = $t_store('downloader.paste_failed');
        }
    }

    async function fetch_link(): Promise<void> {
        const link = url.trim();
        if (!is_valid_link(link)) {
            url_error = $t_store('downloader.invalid_link');
            return;
        }
        url_error = '';
        probe_error = '';
        probe_loading = true;
        probe = null;
        try {
            probe = await probe_link(link);
            probe_for = link;
            choice = 'best';
        } catch (cause) {
            probe_error = message_of(cause, $t_store('downloader.error_generic'));
        } finally {
            probe_loading = false;
        }
    }

    async function submit_download(): Promise<void> {
        const link = url.trim();
        if (!is_valid_link(link)) {
            url_error = $t_store('downloader.invalid_link');
            return;
        }
        start_error = '';
        starting = true;
        try {
            const target = {
                drive_id: destination.drive_id,
                folder_id: destination.folder_id,
                folder_name: destination.path[destination.path.length - 1]?.name ?? '',
            };
            const job_id = await start_download({
                url: link,
                preset_or_format_id: choice,
                target_folder_id: target.folder_id,
                drive_id: target.drive_id,
            });
            download_rows.update((rows) => attach_request(rows, job_id, link, choice, target));
        } catch (cause) {
            start_error = message_of(cause, $t_store('downloader.error_generic'));
        } finally {
            starting = false;
        }
    }

    async function cancel_job(job_id: string): Promise<void> {
        action_error = '';
        try {
            await cancel_download(job_id);
        } catch (cause) {
            action_error = message_of(cause, $t_store('downloader.error_generic'));
        }
    }

    // The file list only shows the active drive. A job saved to another drive, or
    // to a folder whose name this session did not keep, has nothing to open.
    function can_show_in_drive(row: download_row): boolean {
        if (row.stage !== 'done' || row.drive_id !== active_drive_id) return false;
        return row.folder_id === '' || row.folder_name !== '';
    }

    function show_in_drive(row: download_row): void {
        if (!can_show_in_drive(row)) return;
        navigateToIndex(0);
        if (row.folder_id) navigateToFolder(row.folder_id, row.folder_name);
        close();
    }

    async function install(tool: 'yt-dlp' | 'ffmpeg'): Promise<void> {
        setup_error = '';
        setup_busy = tool;
        setup_stage = 'downloading';
        setup_percent = 0;
        try {
            await install_tool(tool);
        } catch (cause) {
            setup_error = message_of(cause, $t_store('downloader.error_generic'));
        } finally {
            setup_busy = '';
            await load_status();
        }
    }

    async function update_tool(): Promise<void> {
        setup_error = '';
        update_message = '';
        setup_busy = 'yt-dlp';
        setup_stage = 'downloading';
        try {
            update_message = await update_ytdlp();
        } catch (cause) {
            setup_error = message_of(cause, $t_store('downloader.error_generic'));
        } finally {
            setup_busy = '';
            await load_status();
        }
    }

    async function save_path(tool: 'yt-dlp' | 'ffmpeg'): Promise<void> {
        path_error = '';
        try {
            await set_tool_path(tool, custom_path.trim());
            custom_path = '';
            await load_status();
        } catch (cause) {
            path_error = message_of(cause, $t_store('downloader.error_generic'));
        }
    }

    function option_label(format: ProbeFormat): string {
        const parts = describe_format(format);
        const kind =
            parts.kind === 'combined'
                ? $t_store('downloader.format_combined')
                : parts.kind === 'video_only'
                  ? $t_store('downloader.format_video_only')
                  : $t_store('downloader.format_audio_only');
        return join_parts([parts.resolution, parts.ext, parts.size, kind]);
    }

    function format_disabled(format: ProbeFormat): boolean {
        return !ffmpeg_ready && format.has_video && !format.has_audio;
    }

    function job_detail(row: download_row): string {
        if (row.stage === 'error' || row.stage === 'cancelled') return row.error;
        if (row.stage !== 'downloading') return '';
        const speed = format_speed(row.speed);
        const eta = format_eta(row.eta);
        if (speed && eta) return $t_store('downloader.speed_eta', { speed, eta });
        return speed;
    }

    function safe_thumbnail(src: string): string {
        return src.startsWith('https://') ? src : '';
    }

    // ModalShell looks up its overlay host by id. The host lives on <body> so no
    // header ancestor with a transform or backdrop filter can clip the fixed overlay.
    function portal(node: HTMLElement): { destroy: () => void } {
        document.body.appendChild(node);
        return {
            destroy() {
                node.remove();
            },
        };
    }
</script>

<div id="downloader-modal" class="modal-overlay" style="display: none;" aria-hidden="true" use:portal>
<ModalShell
    hostId="downloader-modal"
    open={$downloader_open}
    title={$t_store('downloader.title')}
    titleId="downloader-dialog-title"
    subtitle={$t_store('downloader.subtitle')}
    cardClass="downloader-card"
    presentation="dialog"
    initialFocus="#downloader-url"
    restoreFocus="#downloader-open-btn"
    onClose={close}
>
    <div class="downloader-body">
        {#if status_error}
            <p class="downloader-error" role="alert">{status_error}</p>
        {/if}

        {#if !tool_status && !status_error}
            <p class="downloader-muted">{$t_store('downloader.checking')}</p>
        {:else if tool_status && !yt_dlp_ready}
            <section class="downloader-setup" aria-labelledby="downloader-setup-title">
                <h3 id="downloader-setup-title" class="downloader-section-title">{$t_store('downloader.setup_title')}</h3>
                <p class="downloader-muted">{$t_store('downloader.setup_intro')}</p>

                <div class="downloader-tool">
                    <div class="downloader-tool-head">
                        <span class="downloader-tool-name">yt-dlp</span>
                        <span class="downloader-tool-state">{$t_store('downloader.not_installed')}</span>
                        {#if can_install}
                            <button class="primary-btn" type="button" disabled={setup_busy !== ''} onclick={() => install('yt-dlp')}>
                                {$t_store('downloader.install_ytdlp')}
                            </button>
                        {/if}
                    </div>
                    {#if setup_busy === 'yt-dlp'}
                        <ProgressBar
                            indeterminate={setup_stage !== 'downloading'}
                            value={setup_percent}
                            label={$t_store(setup_stage_key(setup_stage))}
                            showValue={setup_stage === 'downloading'}
                        />
                    {/if}
                </div>

                <div class="downloader-tool">
                    <div class="downloader-tool-head">
                        <span class="downloader-tool-name">ffmpeg</span>
                        <span class="downloader-tool-state">
                            {ffmpeg_ready ? $t_store('downloader.tool_ready') : $t_store('downloader.not_installed')}
                        </span>
                        {#if can_install && !ffmpeg_ready}
                            <button class="secondary-btn" type="button" disabled={setup_busy !== ''} onclick={() => install('ffmpeg')}>
                                {$t_store('downloader.install_ffmpeg')}
                            </button>
                        {/if}
                    </div>
                    {#if setup_busy === 'ffmpeg'}
                        <ProgressBar
                            indeterminate={setup_stage !== 'downloading'}
                            value={setup_percent}
                            label={$t_store(setup_stage_key(setup_stage))}
                            showValue={setup_stage === 'downloading'}
                        />
                    {/if}
                    <p class="downloader-muted downloader-small">{$t_store('downloader.ffmpeg_hint')}</p>
                </div>

                {#if setup_error}
                    <p class="downloader-error" role="alert">{setup_error}</p>
                {/if}

                <div class="downloader-field">
                    <label class="field-label" for="downloader-custom-path">{$t_store('downloader.custom_path_label')}</label>
                    <div class="downloader-inline">
                        <input
                            id="downloader-custom-path"
                            type="text"
                            autocomplete="off"
                            spellcheck="false"
                            placeholder={$t_store('downloader.custom_path_placeholder')}
                            bind:value={custom_path}
                        />
                        <button class="secondary-btn" type="button" disabled={!custom_path.trim()} onclick={() => save_path('yt-dlp')}>
                            {$t_store('downloader.custom_path_save')}
                        </button>
                    </div>
                    {#if path_error}
                        <p class="downloader-error" role="alert">{path_error}</p>
                    {/if}
                </div>
            </section>
        {:else if tool_status}
            <section class="downloader-field" aria-labelledby="downloader-url-label">
                <label id="downloader-url-label" class="field-label" for="downloader-url">{$t_store('downloader.link_label')}</label>
                <div class="downloader-inline">
                    <input
                        id="downloader-url"
                        type="url"
                        inputmode="url"
                        autocomplete="off"
                        spellcheck="false"
                        placeholder={$t_store('downloader.link_placeholder')}
                        bind:value={url}
                        onkeydown={(event) => {
                            if (event.key === 'Enter') {
                                event.preventDefault();
                                void fetch_link();
                            }
                        }}
                    />
                    <button class="secondary-btn downloader-icon-btn" type="button" aria-label={$t_store('downloader.paste')} title={$t_store('downloader.paste')} onclick={paste_link}>
                        <ClipboardTextIcon size={16} aria-hidden="true" />
                    </button>
                    <button class="secondary-btn" type="button" disabled={probe_loading || !url.trim()} onclick={fetch_link}>
                        {probe_loading ? $t_store('downloader.fetching') : $t_store('downloader.fetch')}
                    </button>
                </div>
                {#if url_error}
                    <p class="downloader-error" role="alert">{url_error}</p>
                {/if}
                {#if probe_error}
                    <p class="downloader-error" role="alert">{probe_error}</p>
                {/if}
            </section>

            {#if probe}
                <section class="downloader-preview" class:has-thumb={safe_thumbnail(probe.thumbnail) !== ''} aria-label={$t_store('downloader.preview_label')}>
                    {#if safe_thumbnail(probe.thumbnail)}
                        <img class="downloader-thumb" src={safe_thumbnail(probe.thumbnail)} alt="" referrerpolicy="no-referrer" loading="lazy" />
                    {/if}
                    <div class="downloader-preview-text">
                        <p class="downloader-preview-title">{probe.title || $t_store('downloader.untitled')}</p>
                        <p class="downloader-muted downloader-small">
                            {join_parts([
                                probe.uploader ? $t_store('downloader.by_uploader', { uploader: probe.uploader }) : '',
                                probe.duration_s > 0 ? format_clock(probe.duration_s) : '',
                                link_host(probe_for),
                            ])}
                        </p>
                    </div>
                </section>
            {/if}

            <section class="downloader-field">
                <label class="field-label" for="downloader-quality">{$t_store('downloader.quality_label')}</label>
                <select id="downloader-quality" class="select-field" bind:value={choice} disabled={starting}>
                    <optgroup label={$t_store('downloader.group_presets')}>
                        {#each PRESET_OPTIONS as preset (preset.value)}
                            <option value={preset.value}>{$t_store(preset.label_key)}</option>
                        {/each}
                    </optgroup>
                    {#if formats.length > 0}
                        <optgroup label={$t_store('downloader.group_formats')}>
                            {#each formats as format (format.choice)}
                                <option value={format.choice} disabled={format_disabled(format)}>{option_label(format)}</option>
                            {/each}
                        </optgroup>
                    {/if}
                </select>
                {#if chosen_format && format_disabled(chosen_format)}
                    <p class="downloader-error" role="alert">{$t_store('downloader.needs_ffmpeg')}</p>
                {/if}
                {#if !ffmpeg_ready}
                    <p class="downloader-muted downloader-small">
                        {$t_store('downloader.no_ffmpeg_note')}
                        {#if can_install}
                            <button class="downloader-link-btn" type="button" disabled={setup_busy !== ''} onclick={() => install('ffmpeg')}>
                                {$t_store('downloader.install_ffmpeg')}
                            </button>
                        {/if}
                    </p>
                {/if}
            </section>

            <div class="downloader-submit">
                <DestinationPicker bind:destination disabled={starting} />
                <button class="primary-btn" type="button" disabled={start_disabled} onclick={submit_download}>
                    {starting ? $t_store('downloader.starting') : $t_store('downloader.download')}
                </button>
            </div>
            {#if start_error}
                <p class="downloader-error" role="alert">{start_error}</p>
            {/if}
            <div class="downloader-update">
                <button class="downloader-link-btn" type="button" disabled={setup_busy !== ''} onclick={update_tool}>
                    <ArrowsClockwiseIcon size={14} aria-hidden="true" />
                    {$t_store('downloader.update_ytdlp')}
                </button>
                {#if update_message}
                    <span class="downloader-muted downloader-small">{update_message}</span>
                {/if}
                {#if setup_error}
                    <span class="downloader-error">{setup_error}</span>
                {/if}
            </div>
        {/if}

        {#if tool_status && yt_dlp_ready}
            <section class="downloader-jobs" aria-labelledby="downloader-jobs-title">
                <h3 id="downloader-jobs-title" class="downloader-section-title">{$t_store('downloader.jobs_title')}</h3>
                {#if action_error}
                    <p class="downloader-error" role="alert">{action_error}</p>
                {/if}
                {#if $download_rows.length === 0}
                    <p class="downloader-muted">{$t_store('downloader.jobs_empty')}</p>
                {:else}
                    <ul class="downloader-job-list">
                        {#each $download_rows as row (row.job_id)}
                            <li class="downloader-job" data-stage={row.stage}>
                                <div class="downloader-job-head">
                                    <div class="downloader-job-title">
                                        <span>{row.file_name || row.title || link_host(row.url) || $t_store('downloader.untitled')}</span>
                                        <span class="downloader-muted downloader-small">{$t_store(stage_key(row.stage))}</span>
                                    </div>
                                    <div class="downloader-job-actions">
                                        {#if row.stage === 'done'}
                                            {#if can_show_in_drive(row)}
                                                <button class="secondary-btn" type="button" onclick={() => show_in_drive(row)}>
                                                    {$t_store('downloader.show_in_drive')}
                                                </button>
                                            {/if}
                                            <CheckCircleIcon size={18} aria-hidden="true" class="downloader-ok" />
                                        {:else if is_active_stage(row.stage)}
                                            <button class="secondary-btn" type="button" onclick={() => cancel_job(row.job_id)}>
                                                {$t_store('downloader.cancel')}
                                            </button>
                                        {:else if row.stage === 'error'}
                                            <WarningCircleIcon size={18} aria-hidden="true" class="downloader-bad" />
                                        {:else}
                                            <XCircleIcon size={18} aria-hidden="true" />
                                        {/if}
                                    </div>
                                </div>
                                {#if is_active_stage(row.stage)}
                                    <ProgressBar
                                        indeterminate={row.stage === 'uploading' || row.stage === 'merging'}
                                        value={clamp_percent(row.percent)}
                                        showValue={row.stage === 'downloading'}
                                    />
                                {/if}
                                {#if job_detail(row)}
                                    <p class="downloader-muted downloader-small">{job_detail(row)}</p>
                                {/if}
                                {#if is_terminal_stage(row.stage) && row.stage === 'error' && row.error === ''}
                                    <p class="downloader-error">{$t_store('downloader.error_generic')}</p>
                                {/if}
                            </li>
                        {/each}
                    </ul>
                {/if}
            </section>
        {/if}
    </div>

    <p class="downloader-notice">{$t_store('downloader.footer_notice')}</p>

    {#snippet actions()}
        <button class="secondary-btn" type="button" onclick={close}>{$t_store('downloader.close')}</button>
    {/snippet}
</ModalShell>
</div>

<style>
    .downloader-body {
        display: grid;
        gap: 18px;
        min-width: 0;
    }

    .downloader-section-title {
        margin: 0 0 6px;
        font-size: 13px;
        font-weight: 600;
        color: var(--text-main);
    }

    .downloader-muted {
        margin: 0;
        color: var(--text-muted);
        font-size: 13px;
        line-height: 1.4;
    }

    .downloader-small {
        font-size: 12px;
    }

    .downloader-error {
        margin: 0;
        color: var(--danger, #e06c75);
        font-size: 12px;
        line-height: 1.4;
    }

    .downloader-notice {
        margin: 12px 0 0;
        color: var(--text-muted);
        font-size: 12px;
    }

    .downloader-setup,
    .downloader-field,
    .downloader-preview,
    .downloader-jobs {
        display: grid;
        gap: 8px;
        min-width: 0;
    }

    .downloader-tool {
        display: grid;
        gap: 8px;
        padding: 12px;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        background: var(--color-surface-2, transparent);
    }

    .downloader-tool-head,
    .downloader-inline,
    .downloader-submit,
    .downloader-job-head,
    .downloader-job-actions {
        display: flex;
        align-items: center;
        gap: 8px;
        min-width: 0;
        flex-wrap: wrap;
    }

    .downloader-tool-name {
        font-weight: 600;
        color: var(--text-main);
    }

    .downloader-tool-state {
        color: var(--text-muted);
        font-size: 12px;
        flex: 1;
        min-width: 0;
    }

    .downloader-inline input {
        flex: 1;
        min-width: 0;
    }

    .downloader-icon-btn {
        display: inline-grid;
        place-items: center;
        padding: 0 10px;
    }

    .downloader-preview {
        grid-template-columns: minmax(0, 1fr);
        align-items: start;
    }

    .downloader-preview.has-thumb {
        grid-template-columns: 120px minmax(0, 1fr);
    }

    .downloader-thumb {
        width: 120px;
        aspect-ratio: 16 / 9;
        object-fit: cover;
        border-radius: var(--radius-sm);
        background: #000;
    }

    .downloader-preview-text {
        min-width: 0;
        display: grid;
        gap: 4px;
    }

    .downloader-preview-title {
        margin: 0;
        font-size: 14px;
        font-weight: 600;
        color: var(--text-main);
        overflow-wrap: anywhere;
    }

    .downloader-submit {
        justify-content: space-between;
    }

    /* The disabled submit reads as off, not broken: the accent muted, no hover. */
    .downloader-submit .primary-btn:disabled {
        opacity: 1;
        background: rgb(127 156 196 / 0.3);
        color: rgb(255 255 255 / 0.55);
        border-color: transparent;
        cursor: not-allowed;
        transform: none;
    }

    .downloader-jobs {
        border-top: 1px solid var(--border);
        padding-top: 14px;
    }

    .downloader-job-list {
        list-style: none;
        margin: 0;
        padding: 0;
        display: grid;
        gap: 10px;
    }

    .downloader-job {
        display: grid;
        gap: 6px;
        padding: 10px 12px;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        min-width: 0;
    }

    .downloader-job-head {
        justify-content: space-between;
    }

    .downloader-job-title {
        display: grid;
        gap: 2px;
        min-width: 0;
        overflow-wrap: anywhere;
        color: var(--text-main);
        font-size: 13px;
    }

    .downloader-job-actions :global(.downloader-ok) {
        color: var(--color-accent);
    }

    .downloader-job-actions :global(.downloader-bad) {
        color: var(--danger, #e06c75);
    }

    .downloader-update {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 8px;
    }

    /* A quiet ghost button: muted text, no underline, the accent only on hover. */
    .downloader-link-btn {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        min-height: 26px;
        padding: 0 8px;
        margin-left: -8px;
        border: 0;
        border-radius: var(--radius-sm);
        background: none;
        color: var(--text-muted);
        font: inherit;
        font-size: 12px;
        cursor: pointer;
        text-decoration: none;
        transition: color var(--motion-fast) var(--ease-standard);
    }

    .downloader-link-btn:hover:not(:disabled) {
        color: var(--color-accent);
    }

    .downloader-link-btn:disabled {
        opacity: 0.45;
        cursor: not-allowed;
    }

    @media (max-width: 560px) {
        .downloader-preview.has-thumb {
            grid-template-columns: 1fr;
        }
        .downloader-thumb {
            width: 100%;
        }
    }
</style>
