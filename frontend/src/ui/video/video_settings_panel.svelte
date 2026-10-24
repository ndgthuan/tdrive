<script lang="ts">
    import { DEFAULT_PLAYBACK_PREFERENCES, normalizePlaybackPreferences, subtitleBackgroundColors, type PlaybackPreferences, type PictureMode } from '../../modules/video/playback_preferences';
    import { t_store } from '../../i18n/i18n';

    let {
        initialPreferences = DEFAULT_PLAYBACK_PREFERENCES,
        onPreferencesChange = () => {},
    }: {
        initialPreferences?: PlaybackPreferences;
        onPreferencesChange?: (value: PlaybackPreferences) => void;
    } = $props();
    let preferences = $derived(normalizePlaybackPreferences(initialPreferences));
    type SubtitleAppearance = Omit<PlaybackPreferences, 'pictureMode' | 'overrideStyledSubtitles'>;
    let subtitleDraft = $state<Partial<SubtitleAppearance>>({});
    let subtitlePreferences = $derived(normalizePlaybackPreferences({ ...preferences, ...subtitleDraft }));
    // Message keys, resolved in the template so the labels follow the language.
    const modes: { value: PictureMode; label: string; detail: string }[] = [
        { value: 'fit', label: 'video.mode_fit', detail: 'video.mode_fit_detail' },
        { value: 'fill', label: 'video.mode_fill', detail: 'video.mode_fill_detail' },
        { value: 'original', label: 'video.mode_original', detail: 'video.mode_original_detail' },
        { value: '16:9', label: 'video.mode_16_9', detail: 'video.mode_16_9_detail' },
        { value: '4:3', label: 'video.mode_4_3', detail: 'video.mode_4_3_detail' },
    ];

    function updateSubtitle(change: Partial<SubtitleAppearance>) {
        subtitleDraft = { ...subtitleDraft, ...change };
    }

    function updatePicture(pictureMode: PictureMode) {
        onPreferencesChange(normalizePlaybackPreferences({ ...preferences, pictureMode }));
    }

    function saveSubtitles() {
        onPreferencesChange(normalizePlaybackPreferences({ ...subtitlePreferences, overrideStyledSubtitles: true }));
        subtitleDraft = {};
    }

    function resetSubtitles() {
        onPreferencesChange(normalizePlaybackPreferences({ ...DEFAULT_PLAYBACK_PREFERENCES, pictureMode: preferences.pictureMode }));
        subtitleDraft = {};
    }

</script>

<div id="video-picture-settings" class="video-settings-section" hidden>
    <h3>{$t_store('video.picture')}</h3>
    <p class="video-settings-note">{$t_store('video.picture_note')}</p>
    <div class="video-picture-options" role="group" aria-label={$t_store('video.picture_mode_label')}>
        {#each modes as mode (mode.value)}
            <button type="button" data-picture-mode={mode.value} class:is-selected={preferences.pictureMode === mode.value} aria-pressed={preferences.pictureMode === mode.value} onclick={() => updatePicture(mode.value)}>
                <span>{$t_store(mode.label)}</span><small>{$t_store(mode.detail)}</small>
            </button>
        {/each}
    </div>
</div>
<div id="video-subtitle-settings" class="video-settings-section" hidden>
    <div class="video-subtitle-appearance">
    <h3>{$t_store('video.subtitle_appearance')}</h3>
    <p id="video-subtitle-format-note" class="video-settings-note" role="status" hidden>{$t_store('video.subtitle_image_note')}</p>
    <div class="video-subtitle-preview" aria-label={$t_store('video.subtitle_preview_label')} style:color={subtitlePreferences.subtitleColor} style:font-size={`${subtitlePreferences.subtitleFontSize * .55}px`} style:text-shadow={`0 1px ${subtitlePreferences.subtitleOutlineSize}px #000, 1px 0 ${subtitlePreferences.subtitleOutlineSize}px #000, -1px 0 ${subtitlePreferences.subtitleOutlineSize}px #000`}>
        <span style:background={subtitleBackgroundColors(subtitlePreferences).css}>{$t_store('video.subtitle_sample')}</span>
    </div>
    <label class="video-settings-field" for="video-subtitle-size"><span>{$t_store('video.size')} <output>{subtitlePreferences.subtitleFontSize}</output></span>
        <input id="video-subtitle-size" class="video-settings-range" style:--range-fill={`${(subtitlePreferences.subtitleFontSize - 20) / 52 * 100}%`} type="range" min="20" max="72" step="1" value={subtitlePreferences.subtitleFontSize} oninput={(event) => updateSubtitle({ subtitleFontSize: Number(event.currentTarget.value) })} />
    </label>
    <label class="video-settings-field video-settings-inline" for="video-subtitle-color"><span>{$t_store('video.text_color')}</span>
        <input id="video-subtitle-color" type="color" value={subtitlePreferences.subtitleColor} oninput={(event) => updateSubtitle({ subtitleColor: event.currentTarget.value })} />
    </label>
    <label class="video-settings-field" for="video-subtitle-outline"><span>{$t_store('video.outline')} <output>{subtitlePreferences.subtitleOutlineSize}</output></span>
        <input id="video-subtitle-outline" class="video-settings-range" style:--range-fill={`${subtitlePreferences.subtitleOutlineSize / 6 * 100}%`} type="range" min="0" max="6" step="0.05" value={subtitlePreferences.subtitleOutlineSize} oninput={(event) => updateSubtitle({ subtitleOutlineSize: Number(event.currentTarget.value) })} />
    </label>
    <label class="video-settings-check"><input id="video-subtitle-background" type="checkbox" checked={subtitlePreferences.subtitleBackground} onchange={(event) => updateSubtitle({ subtitleBackground: event.currentTarget.checked })} /> {$t_store('video.background')}</label>
    {#if subtitlePreferences.subtitleBackground}
        <label class="video-settings-field video-settings-inline" for="video-subtitle-background-color"><span>{$t_store('video.background_color')}</span>
            <input id="video-subtitle-background-color" type="color" value={subtitlePreferences.subtitleBackgroundColor} oninput={(event) => updateSubtitle({ subtitleBackgroundColor: event.currentTarget.value })} />
        </label>
        <label class="video-settings-field" for="video-subtitle-background-transparency"><span>{$t_store('video.transparency')} <output>{subtitlePreferences.subtitleBackgroundTransparency}%</output></span>
            <input id="video-subtitle-background-transparency" class="video-settings-range" style:--range-fill={`${subtitlePreferences.subtitleBackgroundTransparency}%`} type="range" min="0" max="100" step="1" value={subtitlePreferences.subtitleBackgroundTransparency} oninput={(event) => updateSubtitle({ subtitleBackgroundTransparency: Number(event.currentTarget.value) })} />
        </label>
    {/if}
    <div class="video-subtitle-actions">
        <button id="video-subtitle-save" class="video-settings-save" type="button" onclick={saveSubtitles}>{$t_store('video.save')}</button>
        <button id="video-subtitle-reset" class="video-settings-reset" type="button" onclick={resetSubtitles}>{$t_store('video.reset')}</button>
    </div>
    </div>
</div>
