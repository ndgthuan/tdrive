<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import CaretLeftIcon from 'phosphor-svelte/lib/CaretLeftIcon';
    import { installModalA11y } from '../modals/modal_a11y';
    import ClipboardTextIcon from 'phosphor-svelte/lib/ClipboardTextIcon';
    import { Clipboard } from '@wailsio/runtime';
    import { get } from 'svelte/store';
    import {
        install_tool,
        probe_link,
        read_tool_status,
        start_download,
        subscribe_media_setup,
        update_ytdlp,
        type ProbeResult,
        type Status,
    } from '../../api/media_download';
    import { t_store } from '../../i18n/i18n';
    import { sidebarState } from '../sidebar/sidebar_store';
    import DestinationPicker from '../downloader/destination_picker.svelte';
    import {
        destination_at_root,
        read_saved_destination,
        starting_destination,
        type DownloadDestination,
    } from '../downloader/downloader_destination';
    import { format_clock, is_video_link, link_from_clipboard, setup_stage_key } from '../downloader/downloader_format';
    import { attach_request, download_rows } from '../downloader/downloader_state';
    import { native_engine_present } from '../../modules/media_download/native_engine';
    import { start_mobile_downloads } from '../../modules/media_download/mobile_jobs';

    interface Props {
        open: boolean;
        page?: boolean;
        onclose: () => void;
    }

    let { open, page = false, onclose }: Props = $props();

    // Keep this overlay outside the Account card and its clipped scrollport.
    function portal(node: HTMLElement): { destroy: () => void } {
        if (page) return { destroy: () => undefined };
        document.body.appendChild(node);
        const a11y = installModalA11y(node, { requestClose: onclose });
        a11y.activate();
        return { destroy: () => { a11y.deactivate(); node.remove(); } };
    }

    // The quick options, in the order the backend's presets use. Values are what
    // Start accepts; labels come from the desktop downloader strings.
    const PRESET_OPTIONS: Array<{ value: string; label_key: string }> = [
        { value: 'best', label_key: 'downloader.preset_best' },
        { value: '1080p', label_key: 'downloader.preset_1080p' },
        { value: '720p', label_key: 'downloader.preset_720p' },
        { value: '480p', label_key: 'downloader.preset_480p' },
        { value: 'audio_mp3', label_key: 'downloader.preset_audio_mp3' },
        { value: 'audio_m4a', label_key: 'downloader.preset_audio_m4a' },
    ];

    const engine_present = native_engine_present();

    let tool_status = $state<Status | null>(null);
    let status_error = $state('');
    let setup_busy = $state(false);
    let setup_stage = $state('');
    let setup_percent = $state(0);
    let setup_error = $state('');
    let update_message = $state('');
    // Where the next download is saved. Starts at the saved choice or My Drive's root.
    let destination = $state<DownloadDestination>(destination_at_root(0));

    let url = $state('');
    let url_error = $state('');
    let probe = $state<ProbeResult | null>(null);
    let probe_loading = $state(false);
    let probe_error = $state('');
    let choice = $state('best');
    let starting = $state(false);
    let start_error = $state('');

    const yt_dlp_ready = $derived(tool_status?.yt_dlp.found ?? false);
    const ffmpeg_ready = $derived(tool_status?.ffmpeg.found ?? false);
    const available_formats = $derived((probe?.formats ?? []).filter((format) => ffmpeg_ready || !format.has_video || format.has_audio));
    const can_start = $derived(yt_dlp_ready && probe !== null && !probe_loading && !probe_error && is_video_link(url) && choice !== '' && !starting && destination.drive_id !== 0);

    function changed_link(): void {
        probe = null;
        probe_error = '';
        choice = '';
    }

    // Reads the engine state and the saved destination each time the sheet opens.
    $effect(() => {
        if (!open) return;
        start_mobile_downloads();
        destination = starting_destination(read_saved_destination(), get(sidebarState));
        void load_status();
    });

    function message_of(cause: unknown): string {
        return cause instanceof Error && cause.message ? cause.message : $t_store('downloader.error_generic');
    }

    async function load_status(): Promise<void> {
        status_error = '';
        if (!engine_present) return;
        try {
            tool_status = await read_tool_status();
        } catch (cause) {
            status_error = message_of(cause);
        }
    }

    async function paste_link(): Promise<void> {
        url_error = '';
        try {
            // Use the native pasteboard in Wails; WebKit's Clipboard API can
            // be unavailable in the app's custom-origin web view.
            url = link_from_clipboard(await Clipboard.Text());
            changed_link();
        } catch {
            url_error = $t_store('downloader.paste_failed');
        }
    }

    async function fetch_link(): Promise<void> {
        const link = url.trim();
        if (!is_video_link(link)) {
            url_error = $t_store('downloader.video_link_required');
            return;
        }
        url_error = '';
        probe_error = '';
        probe_loading = true;
        probe = null;
        try {
            const result = await probe_link(link);
            if (url.trim() !== link) return;
            probe = result;
            choice = ffmpeg_ready ? 'best' : (result.formats ?? []).find((format) => format.has_video && format.has_audio)?.choice ?? '';
        } catch (cause) {
            if (url.trim() === link) probe_error = message_of(cause);
        } finally {
            probe_loading = false;
        }
    }

    async function submit_download(): Promise<void> {
        const link = url.trim();
        if (!is_video_link(link)) {
            url_error = $t_store('downloader.video_link_required');
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
            url = '';
            probe = null;
            onclose();
        } catch (cause) {
            start_error = message_of(cause);
        } finally {
            starting = false;
        }
    }

    async function install_engine(): Promise<void> {
        setup_error = '';
        setup_busy = true;
        setup_stage = 'downloading';
        setup_percent = 0;
        const stop = subscribe_media_setup((event) => {
            setup_stage = event.stage;
            setup_percent = event.percent;
            if (event.error) setup_error = event.error;
        });
        try {
            await install_tool('yt-dlp');
        } catch (cause) {
            setup_error = message_of(cause);
        } finally {
            stop();
            setup_busy = false;
            await load_status();
        }
    }

    async function update_engine(): Promise<void> {
        setup_error = '';
        update_message = '';
        setup_busy = true;
        try {
            update_message = await update_ytdlp();
        } catch (cause) {
            setup_error = message_of(cause);
        } finally {
            setup_busy = false;
            await load_status();
        }
    }

    function on_keydown(event: KeyboardEvent): void {
        if (event.key === 'Escape' && open) onclose();
    }
</script>

<svelte:window onkeydown={on_keydown} />

{#if open}
    <div class="download-overlay" class:is-page={page} use:portal>
    {#if !page}<div class="scrim" role="presentation" onclick={onclose}></div>{/if}
    <div class="sheet" class:is-page={page} role={page ? undefined : 'dialog'} aria-modal={page ? undefined : true} aria-labelledby={page ? undefined : 'download-link-title'}>
        {#if !page}
        <header class="head">
            <button type="button" class="text-btn" aria-label={$t_store('mobile.back')} onclick={onclose}><CaretLeftIcon size={22} aria-hidden="true" /></button>
            <h2 id="download-link-title">{$t_store('downloader.title')}</h2>
            <span class="spacer" aria-hidden="true"></span>
        </header>
        {/if}

        <div class="body">
            {#if !engine_present}
                <p class="notice" role="status">{$t_store('mobile.download_engine_missing')}</p>
            {:else if !yt_dlp_ready}
                <div class="card" aria-live="polite">
                    <p class="card-title">{$t_store('downloader.setup_title')}</p>
                    <p class="muted">{$t_store('mobile.download_setup_phone')}</p>
                    {#if setup_busy}
                        <div class="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.max(0, setup_percent)}>
                            <i style="width: {Math.max(0, setup_percent)}%"></i>
                        </div>
                        <p class="muted">{$t_store(setup_stage_key(setup_stage))}</p>
                    {:else}
                        <button type="button" class="primary" onclick={install_engine}>{$t_store('mobile.download_install_action')}</button>
                    {/if}
                    {#if setup_error}<p class="error" role="alert">{setup_error}</p>{/if}
                    {#if status_error}<p class="error" role="alert">{status_error}</p>{/if}
                </div>
            {/if}

            <p class="sec">{$t_store('downloader.link_label')}</p>
            <div class="grp">
                <div class="field">
                    <input
                        type="url"
                        inputmode="url"
                        autocapitalize="off"
                        autocomplete="off"
                        spellcheck="false"
                        placeholder={$t_store('downloader.link_placeholder')}
                        aria-label={$t_store('downloader.link_label')}
                        bind:value={url}
                        oninput={changed_link}
                        disabled={!yt_dlp_ready}
                    />
                    <button type="button" class="chip" onclick={paste_link} disabled={!yt_dlp_ready}>
                        <ClipboardTextIcon size={16} aria-hidden="true" />
                        {$t_store('downloader.paste')}
                    </button>
                </div>
            </div>
            {#if url_error}<p class="error" role="alert">{url_error}</p>{/if}
            <button type="button" class="secondary" onclick={fetch_link} disabled={!yt_dlp_ready || probe_loading}>
                {probe_loading ? $t_store('downloader.fetching') : $t_store('downloader.fetch')}
            </button>
            {#if probe_error}<p class="error" role="alert">{probe_error}</p>{/if}

            {#if probe}
                <div class="card">
                    <p class="card-title">{probe.title || $t_store('downloader.untitled')}</p>
                    <p class="muted">{probe.uploader ? `${probe.uploader} · ` : ''}{format_clock(probe.duration_s)}</p>
                </div>
            {/if}

            <p class="sec">{$t_store('downloader.quality_label')}</p>
            {#if probe && !ffmpeg_ready && available_formats.length === 0}
                <p class="error" role="status">{$t_store('downloader.no_compatible_formats')}</p>
            {/if}
            <div class="grp" role="radiogroup" aria-label={$t_store('downloader.quality_label')}>
                {#each (ffmpeg_ready ? PRESET_OPTIONS : []) as option (option.value)}
                    <label class="opt">
                        <input type="radio" name="download-quality" value={option.value} bind:group={choice} />
                        <span>{$t_store(option.label_key)}</span>
                    </label>
                {/each}
                {#each available_formats as format (format.choice)}
                    <label class="opt">
                        <input type="radio" name="download-quality" value={format.choice} bind:group={choice} />
                        <span>{format.height ? `${format.height}p · ` : ''}{format.ext.toUpperCase()} · {$t_store(!format.has_video ? 'downloader.format_audio_only' : format.has_audio ? 'downloader.format_combined' : 'downloader.format_video_only')}</span>
                    </label>
                {/each}
            </div>
            {#if tool_status && !tool_status.ffmpeg.found}
                <p class="muted">{$t_store('downloader.no_ffmpeg_note')}</p>
            {/if}

            <p class="sec">{$t_store('downloader.destination_save_to')}</p>
            <DestinationPicker bind:destination disabled={starting} />

            {#if yt_dlp_ready}
                <div class="card">
                    <p class="sec">{$t_store('downloader.update_ytdlp')}</p>
                    <p class="muted">{$t_store('mobile.download_update_hint')}</p>
                    {#if update_message}<p class="muted">{update_message}</p>{/if}
                    <button type="button" class="secondary" onclick={update_engine} disabled={setup_busy}>
                        <ArrowsClockwiseIcon size={16} aria-hidden="true" />
                        {$t_store('downloader.update_ytdlp')}
                    </button>
                </div>
            {/if}

            <p class="muted">{$t_store('downloader.footer_notice')}</p>
            <p class="muted">{$t_store('mobile.download_background_note')}</p>
            {#if start_error}<p class="error" role="alert">{start_error}</p>{/if}
        </div>

        <footer class="foot">
            <button type="button" class="primary" onclick={submit_download} disabled={!can_start}>
                {starting ? $t_store('downloader.starting') : $t_store('mobile.download_to_drive')}
            </button>
        </footer>
    </div>
    </div>
{/if}

<style>
    .download-overlay { position: fixed; inset: 0; z-index: var(--z-modal); }
    .download-overlay.is-page { position: relative; inset: auto; z-index: auto; }
    .sheet.is-page { position: relative; inset: auto; max-height: none; border-radius: 0; z-index: auto; }
    .sheet.is-page .body { overflow: visible; }
    .scrim {
        position: fixed;
        inset: 0;
        background: var(--scrim);
        z-index: 60;
    }

    .sheet {
        position: fixed;
        left: 0;
        right: 0;
        bottom: 0;
        top: max(var(--inset-top, 0px), 12px);
        max-height: calc(100dvh - max(var(--inset-top, 0px), 12px));
        display: flex;
        flex-direction: column;
        background: var(--color-canvas);
        color: var(--color-text);
        border-radius: 20px 20px 0 0;
        z-index: 61;
        padding-bottom: env(safe-area-inset-bottom, 0px);
    }

    .head {
        flex: 0 0 auto;
        display: flex;
        align-items: center;
        justify-content: space-between;
        min-height: 52px;
        padding: 8px 16px;
    }

    .head h2 {
        margin: 0;
        font-size: 17px;
        font-weight: 600;
        min-width: 0;
        text-align: center;
    }

    .spacer {
        width: 44px;
        flex: none;
    }

    .body {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        padding: 4px 16px 16px;
        display: flex;
        flex-direction: column;
        gap: 10px;
        overscroll-behavior: contain;
        overflow-x: hidden;
    }

    .body > :global(*) { flex-shrink: 0; min-width: 0; }

    .foot {
        flex: 0 0 auto;
        padding: 10px 16px 16px;
    }

    .sec {
        margin: 6px 4px 0;
        font-size: 12px;
        font-weight: 600;
        letter-spacing: 0.06em;
        text-transform: uppercase;
        color: var(--color-text-muted);
    }

    .grp,
    .card {
        background: var(--color-surface-2);
        border-radius: 16px;
        overflow: hidden;
    }

    .card {
        padding: 14px 16px;
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .field {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 8px 12px;
    }

    .field input[type='url'] {
        flex: 1;
        min-width: 0;
        height: 40px;
        border: 0;
        background: transparent;
        color: inherit;
        font: inherit;
        font-size: 16px;
    }

    .field input:focus-visible,
    .opt input:focus-visible {
        outline: 2px solid var(--color-accent);
        outline-offset: 2px;
    }

    .opt {
        display: flex;
        align-items: center;
        gap: 12px;
        min-height: 50px;
        padding: 8px 14px;
        border-top: 1px solid var(--color-border-soft);
        font-size: 15px;
    }

    .opt:first-child {
        border-top: 0;
    }

    .opt input {
        accent-color: var(--color-accent);
        width: 20px;
        height: 20px;
        margin: 0;
    }

    .card-title {
        margin: 0;
        font-size: 16px;
        font-weight: 500;
    }

    .muted {
        margin: 0;
        font-size: 13px;
        line-height: 18px;
        color: var(--color-text-muted);
    }

    .error {
        margin: 0;
        font-size: 14px;
        color: var(--color-danger);
    }

    .notice {
        margin: 0;
        padding: 14px 16px;
        background: var(--color-surface-1);
        border-radius: 16px;
        color: var(--color-text-muted);
    }

    .bar {
        height: 6px;
        border-radius: 3px;
        background: var(--color-border-soft);
        overflow: hidden;
    }

    .bar i {
        display: block;
        height: 100%;
        background: var(--color-accent);
        border-radius: 3px;
    }

    .text-btn,
    .chip,
    .secondary {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: 6px;
        min-height: 36px;
        padding: 0 12px;
        border: 0;
        border-radius: 18px;
        background: transparent;
        color: var(--color-accent);
        font: inherit;
        font-size: 15px;
        font-weight: 600;
        cursor: pointer;
    }

    .text-btn { min-width: 44px; min-height: 44px; padding: 0; flex: none; }

    .chip,
    .secondary {
        background: var(--overlay-accent-1);
    }

    .primary {
        height: 52px;
        width: 100%;
        border: 0;
        border-radius: 26px;
        background: var(--color-accent);
        color: var(--color-on-accent);
        font: inherit;
        font-size: 17px;
        font-weight: 600;
        cursor: pointer;
    }

    .primary:disabled,
    .secondary:disabled,
    .chip:disabled {
        opacity: 0.45;
        cursor: default;
    }
</style>
