// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"slices"
	"strings"
	"testing"
)

func TestValidateURLAcceptsOnlyPlainHTTP(t *testing.T) {
	good := []string{"https://example.com/watch?v=1", "  http://example.org/a  "}
	for _, raw := range good {
		if _, err := validate_url(raw); err != nil {
			t.Fatalf("validate_url(%q) rejected: %v", raw, err)
		}
	}
	bad := []string{
		"",
		"ftp://example.com/file",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"https://",
		"https://user:secret@example.com/",
		"-o/tmp/x https://example.com",
		"https://example.com/" + strings.Repeat("a", MAX_URL_LENGTH),
	}
	for _, raw := range bad {
		if _, err := validate_url(raw); err == nil {
			t.Fatalf("validate_url(%q) accepted an unsafe link", raw)
		}
	}
}

func TestValidateChoiceSeparatesPresetsFromFormatIDs(t *testing.T) {
	id, merge, err := validate_choice("137+ba")
	if err != nil || id != "137" || !merge {
		t.Fatalf("merge choice = (%q, %v, %v)", id, merge, err)
	}
	if id, merge, err := validate_choice("1080p"); err != nil || id != "1080p" || merge {
		t.Fatalf("preset choice = (%q, %v, %v)", id, merge, err)
	}
	for _, bad := range []string{"", "137;rm", "a b", "$(x)", "../x", strings.Repeat("9", 65)} {
		if _, _, err := validate_choice(bad); err == nil {
			t.Fatalf("validate_choice(%q) accepted an invalid value", bad)
		}
	}
}

func TestFormatSelectorWithAndWithoutFFmpeg(t *testing.T) {
	cases := []struct {
		choice  string
		ffmpeg  bool
		want    string
		needsFF bool
	}{
		{PRESET_BEST, true, "bv*+ba/b", false},
		{PRESET_BEST, false, "b", false},
		{PRESET_1080P, true, "bv*[height<=1080]+ba/b[height<=1080]", false},
		{PRESET_720P, false, "b[height<=720]", false},
		{PRESET_AUDIO_MP3, true, "ba/b", true},
		{PRESET_AUDIO_M4A, false, "ba", false},
		{"22", false, "22", false},
		{"137+ba", true, "137+ba/137", true},
	}
	for _, c := range cases {
		got, needs, err := format_selector(c.choice, c.ffmpeg, false)
		if err != nil {
			t.Fatalf("format_selector(%q, %v) error: %v", c.choice, c.ffmpeg, err)
		}
		if got != c.want || needs != c.needsFF {
			t.Fatalf("format_selector(%q, %v) = (%q, %v), want (%q, %v)", c.choice, c.ffmpeg, got, needs, c.want, c.needsFF)
		}
	}
	if _, _, err := format_selector("137+ba", false, false); err == nil {
		t.Fatal("video-only merge without ffmpeg must be refused")
	}
	if got, needs, err := format_selector("137+ba", false, true); err != nil || needs || got != "137+ba[ext=m4a][acodec^=mp4a]" {
		t.Fatalf("native video merge selector = (%q, %v, %v)", got, needs, err)
	}
	if got, needs, err := format_selector(PRESET_BEST, false, true); err != nil || needs || !strings.Contains(got, "vcodec^=avc1") || !strings.Contains(got, "acodec^=mp4a") {
		t.Fatalf("native best selector = (%q, %v, %v)", got, needs, err)
	}
	if _, _, err := format_selector(PRESET_AUDIO_MP3, false, true); err == nil {
		t.Fatal("native muxer must not claim to encode MP3")
	}
}

func TestBuildDownloadArgsKeepsURLAfterTerminatorAndFixedFlags(t *testing.T) {
	args, err := build_download_args(download_request{
		URL:        "https://example.com/v?a=1&b=2",
		Choice:     PRESET_720P,
		FFmpegPath: `C:\tools\bin\ffmpeg.exe`,
		OutDir:     `D:\data\tmp_dl\abc`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) < 2 || args[len(args)-2] != "--" || args[len(args)-1] != "https://example.com/v?a=1&b=2" {
		t.Fatalf("URL must be the last argument after --: %v", args)
	}
	for _, flag := range []string{"--ignore-config", "--no-playlist", "--no-update", "--newline", "--progress-template", "--merge-output-format"} {
		if !slices.Contains(args, flag) {
			t.Fatalf("missing %s in %v", flag, args)
		}
	}
	if i := slices.Index(args, "--ffmpeg-location"); i < 0 || args[i+1] != `C:\tools\bin\ffmpeg.exe` {
		t.Fatalf("ffmpeg location not passed: %v", args)
	}
	if i := slices.Index(args, "-f"); i < 0 || args[i+1] != "bv*[height<=720]+ba/b[height<=720]" {
		t.Fatalf("format selector wrong: %v", args)
	}
}

func TestBuildDownloadArgsAudioAndNoFFmpeg(t *testing.T) {
	args, err := build_download_args(download_request{URL: "https://example.com/v", Choice: PRESET_AUDIO_MP3, FFmpegPath: "C:/f/ffmpeg.exe", OutDir: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.Index(args, "--audio-format"); i < 0 || args[i+1] != "mp3" || !slices.Contains(args, "-x") {
		t.Fatalf("audio extraction missing: %v", args)
	}
	args, err = build_download_args(download_request{URL: "https://example.com/v", Choice: PRESET_BEST, OutDir: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(args, "--ffmpeg-location") || slices.Contains(args, "--merge-output-format") {
		t.Fatalf("no ffmpeg should mean no merge flags: %v", args)
	}
	if _, err := build_download_args(download_request{URL: "https://example.com/v", Choice: "137+ba", OutDir: "x"}); err == nil {
		t.Fatal("merge choice without ffmpeg must fail before running yt-dlp")
	}
	nativeArgs, err := build_download_args(download_request{URL: "https://example.com/v", Choice: "137+ba", NativeMerge: true, OutDir: "x"})
	if err != nil {
		t.Fatalf("native merge args: %v", err)
	}
	if !slices.Contains(nativeArgs, "--merge-output-format") || slices.Contains(nativeArgs, "-x") || slices.Contains(nativeArgs, "--ffmpeg-location") {
		t.Fatalf("native merge flags = %v", nativeArgs)
	}
	if _, err := build_download_args(download_request{URL: "ftp://x", Choice: PRESET_BEST, OutDir: "x"}); err == nil {
		t.Fatal("non-http link must fail")
	}
}

func TestProbeArgsAreMetadataOnly(t *testing.T) {
	args, err := probe_args("https://example.com/v")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "-J") || !slices.Contains(args, "--no-playlist") || !slices.Contains(args, "--ignore-no-formats-error") || args[len(args)-1] != "https://example.com/v" {
		t.Fatalf("probe args wrong: %v", args)
	}
	if slices.Contains(args, "-x") || slices.Contains(args, "-P") {
		t.Fatalf("probe must not download: %v", args)
	}
}

func TestYoutubeUsesDownloadablePlayerClient(t *testing.T) {
	for name, build := range map[string]func(string) ([]string, error){
		"probe": probe_args,
		"download": func(raw string) ([]string, error) {
			return build_download_args(download_request{URL: raw, Choice: PRESET_BEST, OutDir: "tmp"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			args, err := build("https://youtu.be/example")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(args, "--extractor-args") || !slices.Contains(args, "youtube:player_client=android") {
				t.Fatalf("YouTube player client override missing: %v", args)
			}
		})
	}
	args, err := probe_args("https://example.com/video")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(args, "--extractor-args") {
		t.Fatalf("non-YouTube sites must keep their default extractor: %v", args)
	}
}
