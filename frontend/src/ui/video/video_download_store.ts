import { writable } from 'svelte/store';

// Whether the video on screen may be downloaded. Set by the video module when a
// file opens and cleared when it closes, so the '...' menu shows Download only
// where a download is allowed.
export const videoDownloadable = writable(false);
