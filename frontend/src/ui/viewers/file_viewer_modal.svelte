<script lang="ts">
    import DownloadIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import FileIcon from 'phosphor-svelte/lib/FileIcon';
    import FileTextIcon from 'phosphor-svelte/lib/FileTextIcon';
    import ImageIcon from 'phosphor-svelte/lib/ImageIcon';
    import Music2Icon from 'phosphor-svelte/lib/MusicNotesIcon';
    import PauseIcon from 'phosphor-svelte/lib/PauseIcon';
    import PlayIcon from 'phosphor-svelte/lib/PlayIcon';
    import Volume2Icon from 'phosphor-svelte/lib/SpeakerHighIcon';
    import VolumeXIcon from 'phosphor-svelte/lib/SpeakerXIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import { isMobilePlatform } from '../../api';
    import { createZoomPanController } from '../preview/zoom_pan';
    import { bindTouchGestures } from '../preview/touch_gestures';
    import ModalShell from '../modals/modal_shell.svelte';
    import { isPdfFrameMessage, pdfViewerFrameSrc } from './pdf_frame';
    import { bind_no_save_guards, bindViewOnlyGuards } from './view_only_guards';
    import { fileViewerState } from './file_viewer_store';
    import { t, t_store } from '../../i18n/i18n';
    import {
        structuredTextLanguageForName,
        textViewerModeForName,
        type TextViewerMode,
    } from './text_viewer_types';

    interface Props {
        onClose: () => void;
        onDownload: () => void;
    }

    let { onClose, onDownload }: Props = $props();

    // The phone header is a 44px bar: close, the file name, download. The
    // desktop header below is untouched.
    const mobile = isMobilePlatform();

    const TEXT_CHUNK_BYTES = 512 * 1024;
    const TEXT_MAX_BYTES = 5 * 1024 * 1024;

    let imageLoading = $state(false);
    let imageError = $state('');
    let activeImageUrl = '';

    let audioEl = $state<HTMLAudioElement | null>(null);
    let audioCurrent = $state(0);
    let audioDuration = $state(0);
    let audioPaused = $state(true);
    let audioWaiting = $state(false);
    let audioVolume = $state(1);
    let audioError = $state('');
    let activeAudioUrl = '';

    let pdfFrameEl = $state<HTMLIFrameElement | null>(null);
    let pdfLoading = $state(false);
    let pdfError = $state('');
    let pdfPageCount = $state(0);
    let activePdfUrl = '';

    let textContent = $state('');
    let textOffset = $state(0);
    let textTotal = $state(0);
    let textLoading = $state(false);
    let textError = $state('');
    let textDone = $state(false);
    let textRenderedHtml = $state('');
    let textDecoder = new TextDecoder('utf-8', { fatal: false });
    let textGeneration = 0;
    let activeTextSource = '';
    let markdownView = $state<'rendered' | 'raw'>('rendered');

    function bindProtectedViewer(stage: HTMLElement) {
        const host = stage.closest<HTMLElement>('#viewer-modal') ?? stage;
        const cleanup = bindViewOnlyGuards(host, () => Boolean($fileViewerState.protected));
        const cleanup_no_save = bind_no_save_guards(host, () => Boolean($fileViewerState.readOnly));
        return { destroy: () => { cleanup(); cleanup_no_save(); } };
    }

    // Share the drive preview's zoom geometry and touch recognizer. Source
    // images keep their own capability lifecycle and have no drive actions.
    function bindSourceImage(image: HTMLImageElement) {
        const zoomPan = createZoomPanController(() => image);
        const zoomAt = (x: number, y: number, factor: number) => {
            if (image.naturalWidth > 0) zoomPan.zoomAt(x, y, factor);
        };
        const toggleZoom = (x: number, y: number) => {
            if (zoomPan.scale > 1) zoomPan.reset();
            else zoomAt(x, y, 2.5);
        };
        const wheel = (event: WheelEvent) => {
            event.preventDefault();
            zoomAt(event.clientX, event.clientY, event.deltaY < 0 ? 1.18 : 1 / 1.18);
        };
        const doubleClick = (event: MouseEvent) => {
            if (!mobile) toggleZoom(event.clientX, event.clientY);
        };
        image.addEventListener('wheel', wheel, { passive: false });
        image.addEventListener('dblclick', doubleClick);
        image.addEventListener('pointerdown', zoomPan.pointerDown);
        image.addEventListener('pointermove', zoomPan.pointerMove);
        image.addEventListener('pointerup', zoomPan.pointerUp);
        image.addEventListener('pointercancel', zoomPan.pointerUp);
        const unbindTouch = bindTouchGestures(image, {
            doubleTap: toggleZoom,
            pinchStart: () => zoomPan.endPan(),
            pinch: (factor, x, y) => zoomAt(x, y, factor),
        });
        return {
            destroy() {
                unbindTouch();
                zoomPan.endPan();
                zoomPan.reset();
                image.removeEventListener('wheel', wheel);
                image.removeEventListener('dblclick', doubleClick);
                image.removeEventListener('pointerdown', zoomPan.pointerDown);
                image.removeEventListener('pointermove', zoomPan.pointerMove);
                image.removeEventListener('pointerup', zoomPan.pointerUp);
                image.removeEventListener('pointercancel', zoomPan.pointerUp);
            },
        };
    }

    function formatTime(seconds: number): string {
        if (!Number.isFinite(seconds) || seconds <= 0) return '0:00';
        const whole = Math.floor(seconds);
        const h = Math.floor(whole / 3600);
        const m = Math.floor((whole % 3600) / 60);
        const s = whole % 60;
        if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
        return `${m}:${String(s).padStart(2, '0')}`;
    }

    function viewerLabel(): string {
        switch ($fileViewerState.kind) {
            case 'image':
                return $t_store('viewer.kind_image');
            case 'audio':
                return $t_store('viewer.kind_audio');
            case 'pdf':
                return $t_store('viewer.kind_pdf');
            case 'text':
                return $t_store('viewer.kind_text');
            default:
                return $t_store('viewer.kind_file');
        }
    }

    function viewerStatus(): string {
        if ($fileViewerState.loading) return $t_store('viewer.status_opening');
        if ($fileViewerState.error) return $t_store('viewer.status_unavailable');
        if ($fileViewerState.kind === 'image') return imageError ? $t_store('viewer.status_unavailable') : imageLoading ? $t_store('viewer.status_opening_image') : $t_store('viewer.status_ready');
        if ($fileViewerState.kind === 'audio') return audioError ? $t_store('viewer.status_unavailable') : audioWaiting ? $t_store('viewer.status_buffering') : audioPaused ? $t_store('viewer.status_ready') : $t_store('viewer.status_playing');
        if ($fileViewerState.kind === 'pdf') return pdfError ? $t_store('viewer.status_unavailable') : pdfLoading ? $t_store('viewer.status_opening_pdf') : pdfPageCount ? $t_store('viewer.status_pages', { count: pdfPageCount }) : $t_store('viewer.status_ready');
        if ($fileViewerState.kind === 'text') {
            const mode = currentTextMode();
            if (textLoading && !textOffset) return mode === 'plain' ? $t_store('viewer.status_opening_text') : $t_store('viewer.status_rendering');
            if (mode === 'plain') return textDone ? textCompletionLabel() : textOffset ? $t_store('viewer.status_partial') : $t_store('viewer.status_ready');
            return textDone && textOffset && textTotal > 0 && textOffset >= textTotal ? $t_store('viewer.status_preview_ready') : textOffset ? $t_store('viewer.status_preview_ready') : $t_store('viewer.status_ready');
        }
        return $t_store('viewer.status_ready');
    }

    function syncAudio(): void {
        if (!audioEl) return;
        audioCurrent = Number.isFinite(audioEl.currentTime) ? audioEl.currentTime : 0;
        audioDuration = Number.isFinite(audioEl.duration) ? audioEl.duration : 0;
        audioPaused = audioEl.paused;
        audioWaiting = audioEl.readyState < HTMLMediaElement.HAVE_FUTURE_DATA && !audioEl.paused;
        audioVolume = audioEl.volume;
        if (audioEl.readyState >= HTMLMediaElement.HAVE_METADATA) audioError = '';
    }

    function resetAudioState(): void {
        audioCurrent = 0;
        audioDuration = 0;
        audioPaused = true;
        audioWaiting = false;
        audioVolume = 1;
        audioError = '';
    }

    async function toggleAudio(): Promise<void> {
        if (!audioEl) return;
        if (audioEl.paused) {
            try {
                await audioEl.play();
                audioError = '';
            } catch (error) {
                audioError = String(error || t('viewer.audio_play_failed'));
            }
        } else {
            audioEl.pause();
        }
        syncAudio();
    }

    function seekAudio(event: Event): void {
        if (!audioEl) return;
        const value = Number((event.currentTarget as HTMLInputElement).value);
        if (!Number.isFinite(value)) return;
        audioEl.currentTime = value;
        audioCurrent = value;
    }

    function setAudioVolume(event: Event): void {
        if (!audioEl) return;
        const value = Number((event.currentTarget as HTMLInputElement).value);
        if (!Number.isFinite(value)) return;
        audioEl.volume = Math.max(0, Math.min(1, value));
        audioEl.muted = audioEl.volume <= 0;
        syncAudio();
    }

    function toggleAudioMute(): void {
        if (!audioEl) return;
        audioEl.muted = !audioEl.muted;
        syncAudio();
    }

    function parseContentRangeTotal(header: string | null): number {
        if (!header) return 0;
        const match = /\/(\d+)$/.exec(header.trim());
        return match ? Number(match[1]) : 0;
    }

    function isTextResponseOK(response: Response): boolean {
        return response.ok && (response.status === 200 || response.status === 206);
    }

    async function loadTextChunk(reset = false): Promise<void> {
        const url = $fileViewerState.url;
        if (!$fileViewerState.open || $fileViewerState.kind !== 'text' || !url || currentTextMode() !== 'plain') return;
        if (textLoading || textDone && !reset) return;

        const generation = textGeneration;
        const offset = reset ? 0 : textOffset;
        const end = Math.min(offset + TEXT_CHUNK_BYTES, TEXT_MAX_BYTES) - 1;
        const requestedBytes = Math.max(0, end - offset + 1);
        textLoading = true;
        textError = '';
        try {
            const response = await fetch(url, {
                headers: { Range: `bytes=${offset}-${end}` },
            });
            if (!isTextResponseOK(response)) {
                throw new Error(t('viewer.text_request_failed', { status: response.status }));
            }
            const buffer = await response.arrayBuffer();
            if (generation !== textGeneration) return;

            const isPartial = response.status === 206;
            const total = parseContentRangeTotal(response.headers.get('Content-Range'));
            const nextOffset = offset + buffer.byteLength;
            const reachedFileEnd = total > 0
                ? nextOffset >= total
                : !isPartial || buffer.byteLength < requestedBytes;
            const reachedViewerCap = nextOffset >= TEXT_MAX_BYTES;
            const hasMore = !reachedFileEnd && !reachedViewerCap;
            const chunk = textDecoder.decode(buffer, { stream: hasMore });

            textContent = reset ? chunk : textContent + chunk;
            textOffset = nextOffset;
            textTotal = total;
            textDone = buffer.byteLength === 0 || !hasMore;
        } catch (error) {
            if (generation === textGeneration) textError = String(error || t('viewer.text_open_failed'));
        } finally {
            if (generation === textGeneration) textLoading = false;
        }
    }

    async function loadStructuredText(): Promise<void> {
        const url = $fileViewerState.url;
        const mode = currentTextMode();
        if (!$fileViewerState.open || $fileViewerState.kind !== 'text' || !url || mode === 'plain') return;

        const generation = textGeneration;
        textLoading = true;
        textError = '';
        try {
            const response = await fetch(url, {
                headers: { Range: `bytes=0-${TEXT_MAX_BYTES - 1}` },
            });
            if (!isTextResponseOK(response)) {
                throw new Error(t('viewer.text_request_failed', { status: response.status }));
            }
            const buffer = await response.arrayBuffer();
            if (generation !== textGeneration) return;

            const isPartial = response.status === 206;
            const total = parseContentRangeTotal(response.headers.get('Content-Range'));
            const nextOffset = buffer.byteLength;
            const reachedFileEnd = total > 0
                ? nextOffset >= total
                : !isPartial || nextOffset < TEXT_MAX_BYTES;
            const reachedViewerCap = nextOffset >= TEXT_MAX_BYTES;
            const source = textDecoder.decode(buffer, { stream: false });
            const { renderStructuredText } = await import('./text_viewer');
            if (generation !== textGeneration) return;

            textContent = source;
            textRenderedHtml = renderStructuredText({
                mode,
                source,
                language: structuredTextLanguageForName($fileViewerState.title) ?? undefined,
            });
            textOffset = nextOffset;
            textTotal = total;
            textDone = buffer.byteLength === 0 || reachedFileEnd || reachedViewerCap;
        } catch (error) {
            if (generation === textGeneration) textError = String(error || t('viewer.text_open_failed'));
        } finally {
            if (generation === textGeneration) textLoading = false;
        }
    }

    function resetText(): void {
        textGeneration += 1;
        activeTextSource = '';
        textDecoder = new TextDecoder('utf-8', { fatal: false });
        textContent = '';
        textOffset = 0;
        textTotal = 0;
        textLoading = false;
        textError = '';
        textDone = false;
        textRenderedHtml = '';
        markdownView = 'rendered';
    }

    function textCompletionLabel(): string {
        return textTotal > 0 && textOffset >= textTotal ? $t_store('viewer.text_end_of_file') : $t_store('viewer.text_preview_limit');
    }

    function currentTextMode(): TextViewerMode {
        return textViewerModeForName($fileViewerState.title);
    }

    function textProgressLabel(): string {
        const mode = currentTextMode();
        if (mode !== 'plain') return '';
        if (textOffset > 0) return $t_store('viewer.text_loaded', { size: textOffset < 1024 ? `${textOffset} B` : `${(textOffset / 1024).toFixed(0)} KB` });
        if (!textLoading) return '';
        return $t_store('viewer.opening_text_more');
    }

    function shouldShowMarkdownToggle(): boolean {
        return currentTextMode() === 'markdown' && (textRenderedHtml.length > 0 || textContent.length > 0);
    }

    function shouldShowTextFooter(): boolean {
        return currentTextMode() === 'plain';
    }

    $effect(() => {
        const { open, kind, url } = $fileViewerState;
        const nextUrl = open && kind === 'image' ? url : '';
        if (nextUrl === activeImageUrl) return;
        activeImageUrl = nextUrl;
        imageLoading = Boolean(nextUrl);
        imageError = '';
    });

    $effect(() => {
        const { open, kind, url } = $fileViewerState;
        const nextUrl = open && kind === 'audio' ? url : '';
        if (nextUrl === activeAudioUrl) return;
        activeAudioUrl = nextUrl;
        resetAudioState();
    });

    $effect(() => {
        const { open, kind, url } = $fileViewerState;
        const nextUrl = open && kind === 'pdf' ? url : '';
        if (nextUrl === activePdfUrl) return;
        activePdfUrl = nextUrl;
        pdfLoading = Boolean(nextUrl);
        pdfError = '';
        pdfPageCount = 0;
    });

    $effect(() => {
        const { open, kind, url, title } = $fileViewerState;
        const nextSource = open && kind === 'text' && url ? `${url}\n${title}` : '';
        if (nextSource === activeTextSource) return;
        resetText();
        activeTextSource = nextSource;
        if (!nextSource) return;
        if (currentTextMode() === 'plain') {
            void loadTextChunk(true);
            return;
        }
        void loadStructuredText();
    });

    $effect(() => {
        if (typeof window === 'undefined') return;

        const handleMessage = (event: MessageEvent): void => {
            if (!pdfFrameEl || event.source !== pdfFrameEl.contentWindow) return;
            if (!isPdfFrameMessage(event.data)) return;

            if (event.data.type === 'loaded') {
                pdfLoading = false;
                pdfError = '';
                pdfPageCount = Math.max(0, Math.floor(event.data.pages));
                return;
            }
            pdfLoading = false;
            pdfPageCount = 0;
            pdfError = event.data.message || t('viewer.pdf_open_failed');
        };

        window.addEventListener('message', handleMessage);
        return () => window.removeEventListener('message', handleMessage);
    });
</script>

{#snippet markdownToggle()}
    <div class="text-viewer-toggle" role="tablist" aria-label={$t_store('viewer.markdown_view_label')}>
        <button
            class:active={markdownView === 'rendered'}
            type="button"
            role="tab"
            aria-selected={markdownView === 'rendered'}
            onclick={() => { markdownView = 'rendered'; }}
        >
            {$t_store('viewer.markdown_preview')}
        </button>
        <button
            class:active={markdownView === 'raw'}
            type="button"
            role="tab"
            aria-selected={markdownView === 'raw'}
            onclick={() => { markdownView = 'raw'; }}
        >
            {$t_store('viewer.markdown_raw')}
        </button>
    </div>
{/snippet}

{#snippet protectedBadge()}
    {#if $fileViewerState.protected}
        <span class="file-viewer-protected" title={$t_store('viewer.protected_hint')}>{$t_store('viewer.protected')}</span>
    {/if}
{/snippet}

{#snippet viewerHeader()}
    {#if mobile}
    <div class="file-viewer-head">
        <div class="file-viewer-topbar is-phone">
            <button class="file-viewer-close" type="button" onclick={onClose} aria-label={$t_store('viewer.close_file')} title={$t_store('viewer.close')}>
                <XIcon size={24} aria-hidden="true" />
            </button>
            <h3 id="file-viewer-title" class="file-viewer-title" title={$fileViewerState.title}>
                {$fileViewerState.title || $t_store('viewer.open_file')}
            </h3>
            {#if !$fileViewerState.readOnly && !$fileViewerState.protected}<button class="file-viewer-action is-icon-only" type="button" onclick={onDownload} aria-label={$t_store('viewer.download')} title={$t_store('viewer.download')}>
                <DownloadIcon size={24} aria-hidden="true" />
            </button>{/if}
        </div>
        {#if $fileViewerState.protected}<div class="file-viewer-subbar">{@render protectedBadge()}</div>{/if}
        {#if shouldShowMarkdownToggle()}
            <div class="file-viewer-subbar">
                {@render markdownToggle()}
            </div>
        {/if}
    </div>
    {:else}
    <div class="file-viewer-topbar">
        <div class="file-viewer-identity">
            <div class={`file-viewer-kind-mark is-${$fileViewerState.kind || 'file'}`} aria-hidden="true">
                {#if $fileViewerState.kind === 'image'}
                    <ImageIcon size={24} />
                {:else if $fileViewerState.kind === 'audio'}
                    <Music2Icon size={24} />
                {:else if $fileViewerState.kind === 'text'}
                    <FileTextIcon size={24} />
                {:else}
                    <FileIcon size={24} />
                {/if}
            </div>
            <div class="file-viewer-title-group">
                <h3 id="file-viewer-title" class="file-viewer-title" title={$fileViewerState.title}>
                    {$fileViewerState.title || $t_store('viewer.open_file')}
                </h3>
                <div class="file-viewer-meta">
                    <span>{viewerLabel()}</span>
                    {@render protectedBadge()}
                    {#if $fileViewerState.meta}
                        <span aria-hidden="true">·</span>
                        <span>{$fileViewerState.meta}</span>
                    {/if}
                    <span aria-hidden="true">·</span>
                    <span>{viewerStatus()}</span>
                </div>
            </div>
        </div>
        <div class="file-viewer-actions">
            {#if shouldShowMarkdownToggle()}
                {@render markdownToggle()}
            {/if}
            {#if !$fileViewerState.readOnly && !$fileViewerState.protected}<button class="file-viewer-action" type="button" onclick={onDownload} aria-label={$t_store('viewer.download')} title={$t_store('viewer.download')}>
                <DownloadIcon size={24} aria-hidden="true" />
            </button>{/if}
            <button class="file-viewer-close" type="button" onclick={onClose} aria-label={$t_store('viewer.close_file')} title={$t_store('viewer.close')}>
                <XIcon size={24} aria-hidden="true" />
            </button>
        </div>
    </div>
    {/if}
{/snippet}

<ModalShell
    hostId="viewer-modal"
    open={$fileViewerState.open}
    titleId="file-viewer-title"
    presentation={mobile ? 'dialog' : undefined}
    cardClass={`file-viewer-shell ${$fileViewerState.kind ? `is-${$fileViewerState.kind}` : ''}`}
    initialFocus=".file-viewer-close"
    restoreFocus="#file-list"
    header={viewerHeader}
    onClose={onClose}
>
    <div class="file-viewer-stage" class:is-protected={$fileViewerState.protected} use:bindProtectedViewer>
        {#if $fileViewerState.error}
            <div class="file-viewer-state" role="alert">{$fileViewerState.error}</div>
        {:else if $fileViewerState.kind === 'image'}
            <div class="source-image-viewer">
                {#if imageError}
                    <div class="file-viewer-state" role="alert">{imageError}</div>
                {:else if $fileViewerState.url}
                    {#key $fileViewerState.url}
                        <img
                            use:bindSourceImage
                            src={$fileViewerState.url}
                            alt={$fileViewerState.title}
                            draggable={$fileViewerState.protected || $fileViewerState.readOnly ? false : undefined}
                            onload={() => { imageLoading = false; }}
                            onerror={() => { imageLoading = false; imageError = t('viewer.image_load_failed'); }}
                        />
                    {/key}
                    {#if imageLoading}<div class="source-image-loading" role="status">{$t_store('viewer.opening_image_more')}</div>{/if}
                {:else}
                    <div class="file-viewer-state" role="status">{$t_store('viewer.opening_image_more')}</div>
                {/if}
            </div>
        {:else if $fileViewerState.kind === 'audio'}
            <div class="audio-viewer">
                <audio
                    bind:this={audioEl}
                    src={$fileViewerState.url}
                    preload="metadata"
                    controlslist={$fileViewerState.protected || $fileViewerState.readOnly ? 'nodownload' : undefined}
                    onloadedmetadata={syncAudio}
                    ontimeupdate={syncAudio}
                    onplay={syncAudio}
                    onpause={syncAudio}
                    onwaiting={() => { audioWaiting = true; }}
                    oncanplay={syncAudio}
                    onended={syncAudio}
                    onerror={() => {
                        audioWaiting = false;
                        audioError = t('viewer.audio_stream_failed');
                    }}
                ></audio>
                <div class="audio-hero">
                    <div class="audio-art" aria-hidden="true">
                        <Music2Icon size={24} />
                    </div>
                    <div class="audio-copy">
                        <div class="audio-title" title={$fileViewerState.title}>{$fileViewerState.title}</div>
                        <div class="audio-subtitle">{audioError || (audioWaiting ? $t_store('viewer.audio_buffering') : audioPaused ? $t_store('viewer.audio_ready') : $t_store('viewer.audio_streaming'))}</div>
                    </div>
                </div>
                <div class="audio-timeline">
                    <span class="audio-time">{formatTime(audioCurrent)}</span>
                    <input class="audio-seek" type="range" min="0" max={audioDuration || 0} step="0.1" value={audioCurrent} oninput={seekAudio} aria-label={$t_store('viewer.seek_audio')} />
                    <span class="audio-time">{formatTime(audioDuration)}</span>
                </div>
                <div class="audio-controls" aria-label={$t_store('viewer.audio_controls')}>
                    <button class="audio-play" type="button" onclick={() => void toggleAudio()} aria-label={audioPaused ? $t_store('viewer.play_audio') : $t_store('viewer.pause_audio')}>
                        {#if audioPaused}
                            <PlayIcon size={24} aria-hidden="true" weight="fill" />
                        {:else}
                            <PauseIcon size={24} aria-hidden="true" weight="fill" />
                        {/if}
                    </button>
                    <div class="audio-volume-group">
                        <button class="audio-mute" type="button" onclick={toggleAudioMute} aria-label={audioEl?.muted ? $t_store('viewer.unmute') : $t_store('viewer.mute')}>
                            {#if audioEl?.muted || audioVolume <= 0}
                                <VolumeXIcon size={24} aria-hidden="true" />
                            {:else}
                                <Volume2Icon size={24} aria-hidden="true" />
                            {/if}
                        </button>
                        <input class="audio-volume" type="range" min="0" max="1" step="0.01" value={audioVolume} oninput={setAudioVolume} aria-label={$t_store('viewer.volume')} />
                    </div>
                </div>
            </div>
        {:else if $fileViewerState.kind === 'pdf'}
            <div class="pdf-frame-shell">
                {#if pdfError}
                    <div class="file-viewer-state" role="alert">{pdfError}</div>
                {:else}
                    <iframe
                        bind:this={pdfFrameEl}
                        class="pdf-frame"
                        title={$fileViewerState.title || $t_store('viewer.pdf_viewer_title')}
                        src={pdfViewerFrameSrc($fileViewerState.url, Boolean($fileViewerState.protected), Boolean($fileViewerState.readOnly))}
                    ></iframe>
                    {#if pdfLoading}
                        <div class="pdf-frame-overlay" aria-live="polite">{$t_store('viewer.opening_pdf_more')}</div>
                    {/if}
                {/if}
            </div>
        {:else if $fileViewerState.kind === 'text'}
            <div class="text-viewer">
                {#if textError}
                    <div class="file-viewer-state" role="alert">{textError}</div>
                {/if}
                <div class="text-viewer-scroll">
                    {#if currentTextMode() === 'plain'}
                        <pre class="text-viewer-body">{textContent}</pre>
                    {:else if currentTextMode() === 'markdown' && markdownView === 'raw'}
                        <pre class="text-viewer-body">{textContent}</pre>
                    {:else if textRenderedHtml}
                        <div class={`text-viewer-body is-structured is-${currentTextMode()}`}>
                            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                            {@html textRenderedHtml}
                        </div>
                    {:else}
                        <div class="file-viewer-state">{textLoading ? $t_store('viewer.rendering_preview_more') : $t_store('viewer.no_preview')}</div>
                    {/if}
                </div>
                {#if shouldShowTextFooter()}
                    <div class="text-viewer-footer">
                        <span>{textProgressLabel()}</span>
                        <button class="file-viewer-action" type="button" disabled={textLoading || textDone} onclick={() => void loadTextChunk()}>
                            {textDone ? textCompletionLabel() : textLoading ? $t_store('viewer.loading_more') : $t_store('viewer.load_more')}
                        </button>
                    </div>
                {/if}
            </div>
        {:else}
            <div class="file-viewer-state">{$t_store('viewer.opening_file_more')}</div>
        {/if}
    </div>
</ModalShell>


<style>
    @media print {
        .file-viewer-stage.is-protected { visibility: hidden; }
    }
    .file-viewer-protected {
        display: inline-flex;
        align-items: center;
        padding: 2px 8px;
        border-radius: var(--radius-pill);
        background: var(--viewer-surface-2);
        color: var(--viewer-text-muted);
        font-size: var(--type-xs);
    }
    .file-viewer-stage.is-protected,
    .file-viewer-stage.is-protected :global(*) {
        user-select: none;
        -webkit-user-select: none;
        -webkit-touch-callout: none;
    }
    .source-image-viewer {
        position: relative;
        display: grid;
        place-items: center;
        height: 100%;
        min-height: 0;
        overflow: hidden;
    }
    .source-image-viewer img {
        position: absolute;
        inset: 0;
        display: block;
        width: 100%;
        height: 100%;
        object-fit: contain;
        touch-action: none;
        cursor: zoom-in;
    }
    .source-image-loading {
        position: absolute;
        inset: 0;
        display: grid;
        place-items: center;
        color: var(--viewer-text-muted);
        background: var(--viewer-surface-0);
    }
</style>
