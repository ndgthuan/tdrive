// Copyright (c) 2026 Nguyễn Dương Gia Thuận

// Package mediadl downloads a single online video with yt-dlp and hands the
// result to the caller's upload path. It owns tool discovery, argument
// construction, progress parsing, checksum-verified tool installs and the job
// state machine. It never runs through a shell: every argument is a discrete
// element of an exec argv, and the URL always follows a "--" terminator.
package mediadl

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Preset identifiers the UI may send in place of a yt-dlp format id.
const (
	PRESET_BEST      = "best"
	PRESET_1080P     = "1080p"
	PRESET_720P      = "720p"
	PRESET_480P      = "480p"
	PRESET_AUDIO_MP3 = "audio_mp3"
	PRESET_AUDIO_M4A = "audio_m4a"
)

// AUDIO_SUFFIX_MERGE marks a video-only format that must be merged with the best
// audio stream. The UI sends it as "<format_id>+ba"; it needs ffmpeg.
const AUDIO_SUFFIX_MERGE = "+ba"

// MAX_URL_LENGTH bounds what is accepted from the UI before it reaches yt-dlp.
const MAX_URL_LENGTH = 2048

var format_id_pattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// preset_heights maps the height-limited presets to their pixel cap.
var preset_heights = map[string]int{
	PRESET_1080P: 1080,
	PRESET_720P:  720,
	PRESET_480P:  480,
}

// Preset describes one quality entry the dialog offers before any probe.
type Preset struct {
	ID    string `json:"id"`
	Audio bool   `json:"audio"`
}

// PRESETS is the fixed list the dialog shows above the detailed format list.
var PRESETS = []Preset{
	{ID: PRESET_BEST},
	{ID: PRESET_1080P},
	{ID: PRESET_720P},
	{ID: PRESET_480P},
	{ID: PRESET_AUDIO_MP3, Audio: true},
	{ID: PRESET_AUDIO_M4A, Audio: true},
}

// validate_url accepts only absolute http or https URLs without embedded
// credentials. The returned string is the trimmed input.
func validate_url(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("enter a link to download")
	}
	if len(trimmed) > MAX_URL_LENGTH {
		return "", errors.New("link is too long")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", errors.New("link is not a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("only http and https links can be downloaded")
	}
	if parsed.Host == "" {
		return "", errors.New("link has no host")
	}
	if parsed.User != nil {
		return "", errors.New("links with embedded credentials are not supported")
	}
	return trimmed, nil
}

// validate_choice checks the preset-or-format value from the UI. It returns the
// bare format id and whether the audio merge suffix was requested.
func validate_choice(choice string) (format_id string, merge_audio bool, err error) {
	value := strings.TrimSpace(choice)
	if value == "" {
		return "", false, errors.New("choose a quality first")
	}
	if isPreset(value) {
		return value, false, nil
	}
	merge_audio = strings.HasSuffix(value, AUDIO_SUFFIX_MERGE)
	if merge_audio {
		value = strings.TrimSuffix(value, AUDIO_SUFFIX_MERGE)
	}
	if !format_id_pattern.MatchString(value) {
		return "", false, fmt.Errorf("invalid format id %q", choice)
	}
	return value, merge_audio, nil
}

func isPreset(value string) bool {
	for _, preset := range PRESETS {
		if preset.ID == value {
			return true
		}
	}
	return false
}

// format_selector returns the yt-dlp -f expression and whether the job needs
// ffmpeg for merging or audio extraction. ffmpeg_ok says whether ffmpeg is
// available; without it, merge and extraction choices are refused and the
// selector falls back to single-file formats where that is possible.
func format_selector(choice string, ffmpeg_ok bool) (selector string, needs_ffmpeg bool, err error) {
	format_id, merge_audio, err := validate_choice(choice)
	if err != nil {
		return "", false, err
	}
	switch {
	case format_id == PRESET_BEST:
		if ffmpeg_ok {
			return "bv*+ba/b", false, nil
		}
		return "b", false, nil
	case preset_heights[format_id] > 0:
		height := preset_heights[format_id]
		if ffmpeg_ok {
			return fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]", height, height), false, nil
		}
		return fmt.Sprintf("b[height<=%d]", height), false, nil
	case format_id == PRESET_AUDIO_MP3 || format_id == PRESET_AUDIO_M4A:
		if ffmpeg_ok {
			return "ba/b", true, nil
		}
		return "ba", false, nil
	}
	if merge_audio {
		if !ffmpeg_ok {
			return "", true, errors.New("this format has no audio and needs ffmpeg to add it; install ffmpeg or pick a combined format")
		}
		return format_id + "+ba/" + format_id, true, nil
	}
	return format_id, false, nil
}

// audio_format returns the -x --audio-format value for an audio preset, or "".
func audio_format(choice string) string {
	switch choice {
	case PRESET_AUDIO_MP3:
		return "mp3"
	case PRESET_AUDIO_M4A:
		return "m4a"
	}
	return ""
}

// download_request is everything build_download_args needs. FFmpegPath is empty
// when ffmpeg is not available.
type download_request struct {
	URL        string
	Choice     string
	FFmpegPath string
	OutDir     string
}

// PROGRESS_TEMPLATE prints one machine-readable line per progress tick.
const PROGRESS_TEMPLATE = "download:TDLP|%(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.total_bytes_estimate)s|%(progress.speed)s|%(progress.eta)s"

// OUTPUT_TEMPLATE names the file inside the job directory. yt-dlp still
// sanitises path separators in titles.
const OUTPUT_TEMPLATE = "%(title).120B [%(id)s].%(ext)s"

// build_download_args returns the argv (excluding the executable) for one job.
// Every flag is fixed here; the user supplies only the URL and the choice.
func build_download_args(request download_request) ([]string, error) {
	url_value, err := validate_url(request.URL)
	if err != nil {
		return nil, err
	}
	if request.OutDir == "" {
		return nil, errors.New("download directory is missing")
	}
	ffmpeg_ok := request.FFmpegPath != ""
	selector, needs_ffmpeg, err := format_selector(request.Choice, ffmpeg_ok)
	if err != nil {
		return nil, err
	}
	if needs_ffmpeg && !ffmpeg_ok {
		return nil, errors.New("this quality needs ffmpeg; install ffmpeg first")
	}
	args := []string{
		"--ignore-config",
		"--no-update",
		"--no-cache-dir",
		"--no-playlist",
		"--no-colors",
		"--newline",
		"--socket-timeout", "30",
		"--retries", "3",
		"--progress-template", PROGRESS_TEMPLATE,
		"-P", request.OutDir,
		"-o", OUTPUT_TEMPLATE,
		"-f", selector,
	}
	if ffmpeg_ok {
		args = append(args, "--ffmpeg-location", request.FFmpegPath)
	}
	if format := audio_format(request.Choice); format != "" && ffmpeg_ok {
		args = append(args, "-x", "--audio-format", format)
	} else if ffmpeg_ok {
		// Only used when yt-dlp actually merges two streams; harmless otherwise.
		args = append(args, "--merge-output-format", "mp4")
	}
	args = append(args, "--", url_value)
	return args, nil
}

// probe_args returns the argv for a metadata-only probe of one link.
func probe_args(raw_url string) ([]string, error) {
	url_value, err := validate_url(raw_url)
	if err != nil {
		return nil, err
	}
	return []string{"--ignore-config", "--no-update", "--no-cache-dir", "--no-playlist", "--no-warnings", "--no-colors", "-J", "--", url_value}, nil
}
