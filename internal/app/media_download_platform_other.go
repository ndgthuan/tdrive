//go:build !ios && !android

package app

// media_download_mobile is false on desktop, where yt-dlp is an executable.
const media_download_mobile = false
