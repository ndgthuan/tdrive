<script lang="ts">
    import ListVideoIcon from 'phosphor-svelte/lib/PlaylistIcon';
    import DownloadIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import EllipsisIcon from 'phosphor-svelte/lib/DotsThreeIcon';
    import VideoPlaylistPanel from "./video_playlist_panel.svelte";
    import VideoSettingsPanel from "./video_settings_panel.svelte";
    import type { PlaybackPreferences } from "../../modules/video/playback_preferences";
    import { videoPlaybackPreferences } from "./video_preferences_store";
    import { t_store } from "../../i18n/i18n";
    import { videoDownloadable } from "./video_download_store";

    interface Props {
        onPreferencesChange?: (value: PlaybackPreferences) => void;
        onHidePlaylist?: (restoreFocus?: boolean) => void;
        onSelectPlaylistItem?: (index: number) => void;
        onUpdateVideoAutoNext?: (enabled: boolean) => void;
        onDownload?: () => void;
    }

    let {
        onPreferencesChange = () => undefined,
        onHidePlaylist = () => undefined,
        onSelectPlaylistItem = () => undefined,
        onUpdateVideoAutoNext = () => undefined,
        onDownload = () => undefined,
    }: Props = $props();
    let preferences = $derived($videoPlaybackPreferences);

    // The '...' menu. Its Download entry is decided when the menu opens, so a
    // protected file never shows it.
    let moreOpen = $state(false);
    let moreWrap: HTMLElement | undefined = $state();

    function toggleMore(): void {
        moreOpen = !moreOpen;
    }

    function closeMore(): void {
        moreOpen = false;
    }

    function runDownload(): void {
        closeMore();
        onDownload();
    }

    function runMore(): void {
        closeMore();
        // The playback settings sheet is the rest of this menu; the player owns
        // that panel, so its existing trigger opens it.
        document.getElementById('video-picture-button')?.click();
    }

    function onWindowPointerDown(event: PointerEvent): void {
        if (moreOpen && moreWrap && !moreWrap.contains(event.target as Node)) closeMore();
    }

    function onMenuKeydown(event: KeyboardEvent): void {
        if (event.key === 'Escape' && moreOpen) {
            event.stopPropagation();
            closeMore();
            document.getElementById('video-more-button')?.focus();
        }
    }

    function updatePreferences(value: PlaybackPreferences): void {
        videoPlaybackPreferences.set(value);
        onPreferencesChange(value);
    }
    import AudioLinesIcon from 'phosphor-svelte/lib/WaveformIcon';
    import CaptionsIcon from 'phosphor-svelte/lib/ClosedCaptioningIcon';
    import MaximizeIcon from 'phosphor-svelte/lib/ArrowsOutIcon';
    import MinimizeIcon from 'phosphor-svelte/lib/ArrowsInIcon';
    import PauseIcon from 'phosphor-svelte/lib/PauseIcon';
    import PlayIcon from 'phosphor-svelte/lib/PlayIcon';
    import SettingsIcon from 'phosphor-svelte/lib/GearIcon';
    import Volume2Icon from 'phosphor-svelte/lib/SpeakerHighIcon';
    import VolumeXIcon from 'phosphor-svelte/lib/SpeakerXIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
</script>

<svelte:window onpointerdown={onWindowPointerDown} />
<div id="video-shell" class="video-shell" role="dialog" aria-modal="true" aria-labelledby="video-filename" tabindex="-1">
    <div class="video-topbar">
        <div class="video-title-group">
            <div id="video-filename" class="video-filename"></div>
            <div id="video-meta" class="video-meta"></div>
            <span id="video-protected" class="video-protected" title={$t_store('video.protected_hint')} hidden>{$t_store('video.protected')}</span>
        </div>
        <div class="video-more-wrap" bind:this={moreWrap} onkeydown={onMenuKeydown} role="presentation">
            <button
                id="video-more-button"
                class="video-icon-btn video-more-btn"
                type="button"
                aria-label={$t_store('video.more_actions')}
                aria-haspopup="menu"
                aria-expanded={moreOpen}
                aria-controls="video-more-menu"
                title={$t_store('video.more_actions')}
                onclick={toggleMore}
            >
                <EllipsisIcon size={22} aria-hidden="true" />
            </button>
            {#if moreOpen}
                <div id="video-more-menu" class="video-more-menu" role="menu" aria-label={$t_store('video.more_actions')}>
                    {#if $videoDownloadable}
                        <button id="video-menu-download" class="video-more-item" type="button" role="menuitem" onclick={runDownload}>
                            <DownloadIcon size={18} aria-hidden="true" />
                            <span>{$t_store('messages.video_download')}</span>
                        </button>
                    {/if}
                    <button id="video-menu-more" class="video-more-item" type="button" role="menuitem" onclick={runMore}>
                        <SettingsIcon size={18} aria-hidden="true" />
                        <span>{$t_store('video.playback_settings')}</span>
                    </button>
                </div>
            {/if}
        </div>
        <button id="video-close" class="video-icon-btn video-close-btn" type="button" aria-label={$t_store('video.close_video')} title={$t_store('video.close')}>
            <XIcon size={22} aria-hidden="true" />
        </button>
    </div>

    <div id="video-stage" class="video-stage">
        <div id="video-native-viewport" class="video-native-viewport" aria-hidden="true"></div>
        <video id="video-player" class="video-player" playsinline preload="metadata"></video>

        <div id="video-center-controls" class="video-center-controls" aria-hidden="true" inert>
            <button id="video-center-skip-back" class="video-center-skip video-center-skip-back" type="button" aria-label={$t_store('video.back_10')} title={$t_store('video.back_10')}>
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M11 5H6v5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><path d="M6.6 8.3A7 7 0 1112 19" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><text x="12" y="15.2" text-anchor="middle" fill="currentColor" font-size="6.2" font-weight="800">10</text></svg>
            </button>
            <button id="video-center-play" class="video-center-play" type="button" aria-label={$t_store('video.play_video')} title={$t_store('video.play')}>
                <PlayIcon size={24} aria-hidden="true" weight="fill" />
            </button>
            <button id="video-center-skip-forward" class="video-center-skip video-center-skip-forward" type="button" aria-label={$t_store('video.forward_10')} title={$t_store('video.forward_10')}>
                <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M13 5h5v5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><path d="M17.4 8.3A7 7 0 1012 19" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><text x="12" y="15.2" text-anchor="middle" fill="currentColor" font-size="6.2" font-weight="800">10</text></svg>
            </button>
        </div>

        <div id="video-skip-feedback" class="video-skip-feedback" aria-hidden="true"><span></span></div>

        <div id="video-standalone" class="video-standalone" role="status" aria-live="polite" hidden>
            <strong>{$t_store('video.standalone_title')}</strong>
            <span>{$t_store('video.standalone_body')}</span>
        </div>

        <div id="video-loading" class="video-loading" aria-hidden="true" style="display: none;">
            <div class="video-spinner"></div>
            <div id="video-loading-status" class="video-loading-status" role="status" aria-live="polite" aria-atomic="true">{$t_store('video.opening_video')}</div>
        </div>

        <div id="video-error" class="video-error" role="alert" aria-atomic="true" style="display: none;">
            <div class="video-error-copy">
                <strong class="video-error-title">{$t_store('video.error_title')}</strong>
                <p id="video-error-message" class="video-error-message"></p>
            </div>
            <div class="video-error-actions">
                <button id="video-error-retry" class="video-error-action video-error-retry" type="button">{$t_store('video.retry')}</button>
                <button id="video-error-close" class="video-error-action video-error-close" type="button">{$t_store('video.close')}</button>
            </div>
        </div>
    </div>

    <aside id="video-settings-panel" class="video-popover video-settings-panel" aria-label={$t_store('video.playback_settings')} aria-hidden="true" inert hidden>
        <header class="video-popover-header">
            <div class="video-popover-heading"><h2 class="video-popover-title">{$t_store('video.playback_title')}</h2></div>
            <button id="video-settings-close" class="video-popover-close" type="button" aria-label={$t_store('video.close_settings')}><XIcon size={16} aria-hidden="true" /></button>
        </header>
        <nav class="video-settings-tabs" aria-label={$t_store('video.settings_sections')}>
            <button type="button" data-settings-section="picture">{$t_store('video.section_picture')}</button>
            <button type="button" data-settings-section="audio">{$t_store('video.section_audio')}</button>
            <button type="button" data-settings-section="subtitle">{$t_store('video.section_subtitles')}</button>
            <button type="button" data-settings-section="speed">{$t_store('video.section_speed')}</button>
        </nav>
        <div class="video-popover-body video-settings-body">
            <div id="video-audio-menu" class="video-menu video-track-menu" role="group" aria-label={$t_store('video.audio_track')}></div>
            <div id="video-subtitle-menu" class="video-menu video-track-menu" role="group" aria-label={$t_store('video.subtitles')}></div>
            <div id="video-speed-menu" class="video-menu video-speed-menu" role="group" aria-label={$t_store('video.playback_speed')}></div>
            <VideoSettingsPanel initialPreferences={preferences} onPreferencesChange={updatePreferences} />
        </div>
    </aside>

    <VideoPlaylistPanel
        onClose={() => onHidePlaylist(true)}
        onSelect={onSelectPlaylistItem}
        onAutoNextChange={onUpdateVideoAutoNext}
    />

    <div class="video-controls" aria-label={$t_store('video.video_controls')}>
        <div class="video-timeline-row">
            <div id="video-scrubber" class="video-scrubber" role="slider" tabindex="0" aria-label={$t_store('video.seek')} aria-valuemin="0" aria-valuemax="0" aria-valuenow="0" aria-valuetext="0:00">
                <div class="video-scrubber-hit">
                    <span class="video-scrubber-track"></span>
                    <span id="video-scrubber-buffered" class="video-scrubber-buffered"></span>
                    <span id="video-scrubber-played" class="video-scrubber-played"></span>
                    <span id="video-scrubber-thumb" class="video-scrubber-thumb"></span>
                    <span id="video-scrubber-tooltip" class="video-scrubber-tooltip">
                        <img id="video-scrubber-tooltip-image" class="video-scrubber-tooltip-image" alt="" aria-hidden="true">
                        <span id="video-scrubber-tooltip-time" class="video-scrubber-tooltip-time">0:00</span>
                    </span>
                </div>
            </div>
        </div>

        <div class="video-command-row">
            <div class="video-command-cluster video-primary-cluster">
                <button id="video-skip-back" class="video-icon-btn video-skip-btn" type="button" aria-label={$t_store('video.back_10')} title={$t_store('video.back_10')}>
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M11 5H6v5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><path d="M6.6 8.3A7 7 0 1112 19" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><text x="12" y="15.2" text-anchor="middle" fill="currentColor" font-size="6.2" font-weight="800">10</text></svg>
                </button>
                <button id="video-play" class="video-icon-btn video-play-btn" type="button" data-state="paused" aria-label={$t_store('video.play')} title={$t_store('video.play')}>
                    <PlayIcon size={24} class="video-play-symbol video-symbol-play" aria-hidden="true" weight="fill" />
                    <PauseIcon size={24} class="video-play-symbol video-symbol-pause" aria-hidden="true" weight="fill" />
                </button>
                <button id="video-skip-forward" class="video-icon-btn video-skip-btn" type="button" aria-label={$t_store('video.forward_10')} title={$t_store('video.forward_10')}>
                    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M13 5h5v5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/><path d="M17.4 8.3A7 7 0 1012 19" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><text x="12" y="15.2" text-anchor="middle" fill="currentColor" font-size="6.2" font-weight="800">10</text></svg>
                </button>
                <div class="video-volume-group">
                    <button id="video-mute" class="video-icon-btn video-mute-btn" type="button" data-state="unmuted" aria-label={$t_store('video.mute')}>
                        <Volume2Icon size={24} class="video-mute-symbol video-symbol-volume" aria-hidden="true" />
                        <VolumeXIcon size={24} class="video-mute-symbol video-symbol-muted" aria-hidden="true" />
                    </button>
                    <div id="video-volume-slider" class="video-volume-slider" role="slider" tabindex="0" aria-label={$t_store('video.volume')} aria-valuemin="0" aria-valuemax="100" aria-valuenow="100" aria-valuetext="100%">
                        <span class="video-volume-track"></span>
                        <span id="video-volume-fill" class="video-volume-fill"></span>
                        <span id="video-volume-thumb" class="video-volume-thumb"></span>
                    </div>
                </div>

                <button id="video-time-display" class="video-time-display" type="button" aria-pressed="false" aria-label={$t_store('video.show_finish_time')} aria-describedby="video-time video-duration">
                    <span id="video-time" class="video-time">0:00</span><span class="video-time-separator"> / </span><span id="video-duration" class="video-time">--:--</span>
                    <span id="video-end-time" class="video-end-time" aria-hidden="true"><span></span></span>
                </button>
            </div>

            <div class="video-command-cluster video-secondary-cluster">
                <div id="video-audio-wrap" class="video-menu-wrap video-track-wrap" hidden>
                    <button id="video-audio-button" class="video-pill-button video-track-button" type="button" aria-label={$t_store('video.audio_track')} title={$t_store('video.audio_track')}>
                        <AudioLinesIcon size={15} aria-hidden="true" />
                        <span id="video-audio-label" class="video-track-value"></span>
                    </button>
                </div>

                <div id="video-subtitle-wrap" class="video-menu-wrap video-track-wrap" hidden>
                    <button id="video-subtitle-button" class="video-pill-button video-track-button" type="button" data-state="off" aria-label={$t_store('video.subtitles')} title={$t_store('video.subtitles')}>
                        <CaptionsIcon size={15} aria-hidden="true" />
                        <span id="video-subtitle-label" class="video-track-value"></span>
                    </button>
                </div>

                <div class="video-menu-wrap video-speed-wrap">
                    <button id="video-speed-button" class="video-pill-button video-speed-button" type="button">1x</button>
                </div>

                <button id="video-aspect-button" class="video-pill-button video-aspect-button" type="button" aria-label={$t_store('video.video_fit', { mode: $t_store('video.mode_fit') })} title={$t_store('video.video_fit', { mode: $t_store('video.mode_fit') })}>{$t_store('video.mode_fit')}</button>

                <button id="video-picture-button" class="video-pill-button video-picture-button" type="button" aria-label={$t_store('video.playback_settings')} aria-expanded="false" aria-controls="video-settings-panel" title={$t_store('video.playback_settings')}>
                    <SettingsIcon size={18} aria-hidden="true" />
                </button>

                <button id="video-playlist-button" class="video-pill-button video-playlist-button" type="button" aria-label={$t_store('video.open_playlist')} aria-expanded="false" aria-controls="video-playlist-panel" title={$t_store('video.open_playlist')} hidden>
                    <ListVideoIcon size={18} aria-hidden="true" />
                </button>

                <button id="video-fullscreen" class="video-icon-btn video-fullscreen-btn" type="button" data-state="windowed" aria-label={$t_store('video.enter_fullscreen')} title={$t_store('video.enter_fullscreen')}>
                    <MaximizeIcon size={24} class="video-fullscreen-symbol video-symbol-fullscreen-enter" aria-hidden="true" />
                    <MinimizeIcon size={24} class="video-fullscreen-symbol video-symbol-fullscreen-exit" aria-hidden="true" />
                </button>
            </div>
        </div>
    </div>
</div>


<style>
    .video-protected {
        color: var(--viewer-text-muted);
        font-size: var(--type-xs);
    }
    .video-shell:global([data-protected="true"]) {
        user-select: none;
        -webkit-user-select: none;
        -webkit-touch-callout: none;
    }
    @media print {
        .video-shell:global([data-protected="true"]) { visibility: hidden; }
    }
</style>
