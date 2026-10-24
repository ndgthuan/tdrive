<!-- Copyright (c) 2026 Nguyễn Dương Gia Thuận -->

<script lang="ts">
    import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwiseIcon';
    import ClipboardTextIcon from 'phosphor-svelte/lib/ClipboardTextIcon';
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
    import { format_clock, is_valid_link, setup_stage_key } from '../downloader/downloader_format';
    import { attach_request, download_rows } from '../downloader/downloader_state';
    import { native_engine_present } from '../../modules/media_download/native_engine';
    import { start_mobile_downloads } from '../../modules/media_download/mobile_jobs';

    interface Props {
        open: boolean;
        onclose: () => void;
    }

    let { open, onclose }: Props = $props();

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
    const can_start = $derived(yt_dlp_ready && is_valid_link(url) && choice !== '' && !starting && destination.drive_id !== 0);

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
            choice = 'best';
        } catch (cause) {
            probe_error = message_of(cause);
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
    <div class="scrim" role="presentation" onclick={onclose}></div>
    <div class="sheet" role="dialog" aria-modal="true" aria-labelledby="download-link-title">
        <header class="head">
            <button type="button" class="text-btn" onclick={onclose}>{$t_store('downloader.cancel')}</button>
            <h2 id="download-link-title">{$t_store('downloader.title')}</h2>
            <span class="spacer" aria-hidden="true"></span>
        </header>

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
            <div class="grp" role="radiogroup" aria-label={$t_store('downloader.quality_label')}>
                {#each PRESET_OPTIONS as option (option.value)}
                    <label class="opt">
                        <input type="radio" name="download-quality" value={option.value} bind:group={choice} />
                        <span>{$t_store(option.label_key)}</span>
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
{/if}

<style>
    .scrim {
        position: fixed;
        inset: 0;
        background: rgb(0 0 0 / 0.55);
        z-index: 60;
    }

    .sheet {
        position: fixed;
        left: 0;
        right: 0;
        bottom: 0;
        max-height: 92dvh;
        display: flex;
        flex-direction: column;
        background: #000;
        color: #e8eaed;
        border-radius: 20px 20px 0 0;
        z-index: 61;
        padding-bottom: env(safe-area-inset-bottom, 0px);
    }

    .head {
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
    }

    .spacer {
        width: 60px;
    }

    .body {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        padding: 4px 16px 16px;
        display: flex;
        flex-direction: column;
        gap: 10px;
    }

    .foot {
        padding: 10px 16px 16px;
    }

    .sec {
        margin: 6px 4px 0;
        font-size: 12px;
        font-weight: 600;
        letter-spacing: 0.06em;
        text-transform: uppercase;
        color: #a0a7b1;
    }

    .grp,
    .card {
        background: #1e2227;
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
        outline: 2px solid #7f9cc4;
        outline-offset: 2px;
    }

    .opt {
        display: flex;
        align-items: center;
        gap: 12px;
        min-height: 50px;
        padding: 8px 14px;
        border-top: 1px solid #2a2f36;
        font-size: 15px;
    }

    .opt:first-child {
        border-top: 0;
    }

    .opt input {
        accent-color: #7f9cc4;
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
        color: #a0a7b1;
    }

    .error {
        margin: 0;
        font-size: 14px;
        color: #e8a0a0;
    }

    .notice {
        margin: 0;
        padding: 14px 16px;
        background: #1e2227;
        border-radius: 16px;
        color: #a0a7b1;
    }

    .bar {
        height: 6px;
        border-radius: 3px;
        background: #22272e;
        overflow: hidden;
    }

    .bar i {
        display: block;
        height: 100%;
        background: #7f9cc4;
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
        color: #7f9cc4;
        font: inherit;
        font-size: 15px;
        font-weight: 600;
        cursor: pointer;
    }

    .chip,
    .secondary {
        background: #1e2a3d;
    }

    .primary {
        height: 52px;
        width: 100%;
        border: 0;
        border-radius: 26px;
        background: #7f9cc4;
        color: #0b0f14;
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
