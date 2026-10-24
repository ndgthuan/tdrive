import { writable } from 'svelte/store';
import { loadPlaybackPreferences, type PlaybackPreferences } from '../../modules/video/playback_preferences';

export const videoPlaybackPreferences = writable<PlaybackPreferences>(loadPlaybackPreferences());
