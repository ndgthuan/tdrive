package mediadl

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// MAX_LISTED_FORMATS caps the detailed list so a site with hundreds of
// renditions cannot flood the dialog.
const MAX_LISTED_FORMATS = 60

// ProbeFormat is one entry of the detailed quality list. Choice is the value to
// pass back to Start.
type ProbeFormat struct {
	ID         string `json:"id"`
	Choice     string `json:"choice"`
	Note       string `json:"note"`
	Height     int    `json:"height"`
	Ext        string `json:"ext"`
	ApproxSize int64  `json:"approx_size"`
	HasAudio   bool   `json:"has_audio"`
	HasVideo   bool   `json:"has_video"`
}

// ProbeResult is what the dialog shows after Fetch.
type ProbeResult struct {
	Title     string        `json:"title"`
	Uploader  string        `json:"uploader"`
	DurationS int64         `json:"duration_s"`
	Thumbnail string        `json:"thumbnail"`
	Formats   []ProbeFormat `json:"formats"`
	Presets   []Preset      `json:"presets"`
}

type raw_probe_format struct {
	FormatID       string  `json:"format_id"`
	Ext            string  `json:"ext"`
	Height         float64 `json:"height"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
	FileSize       float64 `json:"filesize"`
	FileSizeApprox float64 `json:"filesize_approx"`
	FormatNote     string  `json:"format_note"`
	Protocol       string  `json:"protocol"`
}

type raw_probe struct {
	Title     string             `json:"title"`
	Uploader  string             `json:"uploader"`
	Duration  float64            `json:"duration"`
	Thumbnail string             `json:"thumbnail"`
	Formats   []raw_probe_format `json:"formats"`
	Type      string             `json:"_type"`
}

// parse_probe_output reads the lines `yt-dlp -J` printed. The JSON object is the
// line that starts with "{"; ERROR lines explain a failed probe.
func parse_probe_output(lines []string) (ProbeResult, error) {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") {
			return parse_probe_json([]byte(trimmed))
		}
	}
	if message := error_from_lines(lines); message != "" {
		return ProbeResult{}, errors.New(message)
	}
	return ProbeResult{}, errors.New("yt-dlp returned no metadata for this link")
}

// parse_probe_json converts yt-dlp's single-video JSON into the dialog model.
// Storyboards and formats with neither audio nor video are dropped.
func parse_probe_json(data []byte) (ProbeResult, error) {
	var raw raw_probe
	if err := json.Unmarshal(data, &raw); err != nil {
		return ProbeResult{}, fmt.Errorf("cannot read yt-dlp output: %w", err)
	}
	if raw.Type == "playlist" {
		return ProbeResult{}, errors.New("playlists are not supported; paste a link to one video")
	}
	result := ProbeResult{
		Title:     strings.TrimSpace(raw.Title),
		Uploader:  strings.TrimSpace(raw.Uploader),
		DurationS: int64(raw.Duration),
		Thumbnail: raw.Thumbnail,
		Presets:   PRESETS,
	}
	formats := make([]ProbeFormat, 0, len(raw.Formats))
	for _, format := range raw.Formats {
		listed, ok := to_probe_format(format)
		if ok {
			formats = append(formats, listed)
		}
	}
	// Tallest first, then the larger file, so the best options lead the list.
	sort.SliceStable(formats, func(i, j int) bool {
		if formats[i].Height != formats[j].Height {
			return formats[i].Height > formats[j].Height
		}
		return formats[i].ApproxSize > formats[j].ApproxSize
	})
	if len(formats) > MAX_LISTED_FORMATS {
		formats = formats[:MAX_LISTED_FORMATS]
	}
	result.Formats = formats
	return result, nil
}

func to_probe_format(raw raw_probe_format) (ProbeFormat, bool) {
	id := strings.TrimSpace(raw.FormatID)
	if !format_id_pattern.MatchString(id) {
		return ProbeFormat{}, false
	}
	if strings.Contains(strings.ToLower(raw.FormatNote), "storyboard") || raw.Ext == "mhtml" {
		return ProbeFormat{}, false
	}
	has_video := raw.VCodec != "" && raw.VCodec != "none"
	has_audio := raw.ACodec != "" && raw.ACodec != "none"
	if !has_video && !has_audio {
		return ProbeFormat{}, false
	}
	choice := id
	if has_video && !has_audio {
		choice = id + AUDIO_SUFFIX_MERGE
	}
	size := int64(raw.FileSize)
	if size <= 0 {
		size = int64(raw.FileSizeApprox)
	}
	return ProbeFormat{
		ID:         id,
		Choice:     choice,
		Note:       strings.TrimSpace(raw.FormatNote),
		Height:     int(raw.Height),
		Ext:        raw.Ext,
		ApproxSize: size,
		HasAudio:   has_audio,
		HasVideo:   has_video,
	}, true
}
