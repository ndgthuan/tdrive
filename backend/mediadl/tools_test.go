// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// new_test_tools returns a locator whose PATH lookup never finds anything, so the
// developer's own yt-dlp or ffmpeg cannot change the outcome.
func new_test_tools(t *testing.T) *Tools {
	t.Helper()
	tools := NewTools(t.TempDir())
	tools.look_path = func(string) (string, error) { return "", errors.New("not on PATH") }
	return tools
}

func write_fake_tool(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fake"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestLocateOrderIsOverrideThenPathThenManaged(t *testing.T) {
	tools := new_test_tools(t)
	managed := filepath.Join(tools.paths.Bin, exe_name(TOOL_YTDLP))
	write_fake_tool(t, managed)
	path_copy := filepath.Join(t.TempDir(), exe_name(TOOL_YTDLP))
	write_fake_tool(t, path_copy)
	override := filepath.Join(t.TempDir(), exe_name(TOOL_YTDLP))
	write_fake_tool(t, override)

	_, source, err := tools.locate(TOOL_YTDLP)
	if err != nil || source != SOURCE_MANAGED {
		t.Fatalf("managed copy should be found last-resort: %s %v", source, err)
	}

	tools.look_path = func(string) (string, error) { return path_copy, nil }
	if _, source, _ := tools.locate(TOOL_YTDLP); source != SOURCE_PATH {
		t.Fatalf("PATH should beat the managed copy, got %s", source)
	}

	if err := tools.SetOverride(TOOL_YTDLP, override); err != nil {
		t.Fatal(err)
	}
	got, source, err := tools.locate(TOOL_YTDLP)
	if err != nil || source != SOURCE_OVERRIDE || got != override {
		t.Fatalf("override should win: %q %s %v", got, source, err)
	}

	if err := tools.SetOverride(TOOL_YTDLP, ""); err != nil {
		t.Fatal(err)
	}
	if _, source, _ := tools.locate(TOOL_YTDLP); source != SOURCE_PATH {
		t.Fatalf("clearing the override should restore PATH, got %s", source)
	}
}

func TestSetOverrideRejectsWrongNamesAndMissingFiles(t *testing.T) {
	tools := new_test_tools(t)
	dir := t.TempDir()
	notepad := filepath.Join(dir, "notepad.exe")
	write_fake_tool(t, notepad)
	if err := tools.SetOverride(TOOL_YTDLP, notepad); err == nil {
		t.Fatal("an executable with the wrong name must not be accepted")
	}
	if err := tools.SetOverride(TOOL_FFMPEG, filepath.Join(dir, "ffmpeg.exe")); err == nil {
		t.Fatal("a missing file must not be accepted")
	}
	if err := tools.SetOverride("curl", notepad); err == nil {
		t.Fatal("unknown tool names must be refused")
	}
}

func TestOverrideIsStoredOutsideConfigJSON(t *testing.T) {
	tools := new_test_tools(t)
	ffmpeg := filepath.Join(t.TempDir(), exe_name(TOOL_FFMPEG))
	write_fake_tool(t, ffmpeg)
	if err := tools.SetOverride(TOOL_FFMPEG, ffmpeg); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(tools.paths.Settings) != SETTINGS_FILE {
		t.Fatalf("settings file = %s", tools.paths.Settings)
	}
	reloaded := &Tools{paths: tools.paths, look_path: tools.look_path}
	if got := reloaded.load_settings().FFmpegPath; got != ffmpeg {
		t.Fatalf("override not persisted: %q", got)
	}
}

func TestStatusReportsMissingToolsAndInstallSupport(t *testing.T) {
	tools := new_test_tools(t)
	status := tools.Status()
	if status.YtDlp.Found || status.FFmpeg.Found {
		t.Fatalf("nothing should be found in an empty data dir: %+v", status)
	}
	if status.CanInstall != (runtime.GOOS == "windows") {
		t.Fatalf("can_install = %v on %s", status.CanInstall, runtime.GOOS)
	}
}
