// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"errors"
	"testing"
)

func TestParseProgressLine(t *testing.T) {
	sample, ok := parse_progress_line("TDLP|512|2048|NA|1048576.5|7")
	if !ok {
		t.Fatal("progress line not recognised")
	}
	if sample.Downloaded != 512 || sample.Total != 2048 || sample.Speed != 1048576.5 || sample.ETA != 7 {
		t.Fatalf("unexpected sample %+v", sample)
	}
	if got := sample.Percent(); got != 25 {
		t.Fatalf("percent = %v, want 25", got)
	}
}

func TestParseProgressLineUsesEstimateAndHandlesUnknowns(t *testing.T) {
	sample, ok := parse_progress_line("TDLP|100|NA|400|NA|NA")
	if !ok {
		t.Fatal("line not recognised")
	}
	if sample.Total != 400 || sample.Percent() != 25 {
		t.Fatalf("estimate not used: %+v", sample)
	}
	if sample.Speed != 0 || sample.ETA != -1 {
		t.Fatalf("NA fields should be unknown: %+v", sample)
	}
	none, ok := parse_progress_line("TDLP|NA|NA|NA|NA|NA")
	if !ok || none.Percent() != -1 {
		t.Fatalf("no total should report unknown percent: %+v", none)
	}
}

func TestParseProgressLineRejectsOtherOutput(t *testing.T) {
	for _, line := range []string{"[download] 10% of 5MiB", "TDLP|1|2|3", ""} {
		if _, ok := parse_progress_line(line); ok {
			t.Fatalf("line %q parsed as progress", line)
		}
	}
}

func TestMergingAndErrorLines(t *testing.T) {
	if !is_merging_line(`[Merger] Merging formats into "x.mp4"`) || !is_merging_line("[ExtractAudio] Destination: x.mp3") {
		t.Fatal("post-processing lines must start the merge stage")
	}
	if is_merging_line("[download] Destination: x.mp4") {
		t.Fatal("download line must not start the merge stage")
	}
	lines := []string{"WARNING: x", "ERROR: first problem", "ERROR: Video unavailable"}
	if got := error_from_lines(lines); got != "Video unavailable" {
		t.Fatalf("error_from_lines = %q", got)
	}
	if got := error_from_lines([]string{"noise"}); got != "" {
		t.Fatalf("no error expected, got %q", got)
	}
}

func TestErrorFromMobileBridgeError(t *testing.T) {
	err := errors.New("Binding call failed: Bound method returned an error: WARNING: stream note\nERROR: unable to download video data: HTTP Error 403: Forbidden")
	if got := error_from_error(err); got != "unable to download video data: HTTP Error 403: Forbidden" {
		t.Fatalf("error_from_error = %q", got)
	}
	if got := error_from_error(errors.New("bridge failed without yt-dlp output")); got != "" {
		t.Fatalf("error_from_error without ERROR line = %q", got)
	}
}

func TestLineWriterSplitsCarriageReturnsAndNewlines(t *testing.T) {
	var got []string
	w := &line_writer{on_line: func(line string) { got = append(got, line) }}
	_, _ = w.Write([]byte("one\r\ntwo\rthr"))
	_, _ = w.Write([]byte("ee\n\n"))
	w.flush()
	want := []string{"one", "two", "three"}
	if len(got) != len(want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("lines = %q, want %q", got, want)
		}
	}
}
