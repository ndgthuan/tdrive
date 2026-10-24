package mediadl

import (
	"strconv"
	"strings"
)

// PROGRESS_PREFIX tags the lines produced by PROGRESS_TEMPLATE so they cannot be
// confused with yt-dlp's own output.
const PROGRESS_PREFIX = "TDLP|"

// ProgressSample is one parsed progress tick. Unknown numeric fields are -1
// (bytes and seconds) or 0 (speed), matching how yt-dlp prints "NA".
type ProgressSample struct {
	Downloaded int64
	Total      int64
	Speed      float64
	ETA        int64
}

// Percent returns 0-100 from the best known total, or -1 when no total is known.
func (s ProgressSample) Percent() float64 {
	total := s.Total
	if total <= 0 {
		return -1
	}
	if s.Downloaded < 0 {
		return 0
	}
	percent := float64(s.Downloaded) / float64(total) * 100
	if percent > 100 {
		return 100
	}
	return percent
}

// parse_progress_line reads one PROGRESS_TEMPLATE line. Other lines return false.
func parse_progress_line(line string) (ProgressSample, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, PROGRESS_PREFIX) {
		return ProgressSample{}, false
	}
	fields := strings.Split(strings.TrimPrefix(trimmed, PROGRESS_PREFIX), "|")
	if len(fields) != 5 {
		return ProgressSample{}, false
	}
	downloaded := parse_int_field(fields[0], -1)
	total := parse_int_field(fields[1], -1)
	if total <= 0 {
		total = parse_int_field(fields[2], -1)
	}
	speed, ok := parse_float_field(fields[3])
	if !ok {
		speed = 0
	}
	eta := parse_int_field(fields[4], -1)
	return ProgressSample{Downloaded: downloaded, Total: total, Speed: speed, ETA: eta}, true
}

func parse_int_field(raw string, fallback int64) int64 {
	value, ok := parse_float_field(raw)
	if !ok {
		return fallback
	}
	return int64(value)
}

func parse_float_field(raw string) (float64, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || value == "NA" || value == "None" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

// merging_prefixes are yt-dlp post-processing tags. Any of them means the
// download bytes are done and ffmpeg is combining or converting them.
var merging_prefixes = []string{
	"[Merger]",
	"[ExtractAudio]",
	"[VideoConvertor]",
	"[VideoRemuxer]",
	"[Fixup",
	"[FixupM3u8]",
}

// is_merging_line reports whether a yt-dlp output line marks the merge stage.
func is_merging_line(line string) bool {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range merging_prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// error_from_lines returns the last "ERROR:" message yt-dlp printed, without the
// prefix, or "" when none was printed.
func error_from_lines(lines []string) string {
	message := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, "ERROR:"); ok {
			message = strings.TrimSpace(rest)
		}
	}
	return message
}
