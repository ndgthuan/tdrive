// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Tool names as the UI and the setup events refer to them.
const (
	TOOL_YTDLP  = "yt-dlp"
	TOOL_FFMPEG = "ffmpeg"
)

// SETTINGS_FILE holds the user's own tool paths. It lives beside TDrive's data,
// never inside config.json, so the app's main settings are not rewritten.
const SETTINGS_FILE = "media_download_tools.json"

// BIN_DIR is the app-managed tool directory inside TDrive's data directory.
const BIN_DIR = "bin"

// TMP_DIR holds per-job download directories inside TDrive's data directory.
const TMP_DIR = "tmp_dl"

var errToolNotFound = errors.New("tool not found")

// Source says where a located tool came from.
const (
	SOURCE_OVERRIDE = "override"
	SOURCE_PATH     = "path"
	SOURCE_BUNDLED  = "bundled"
	SOURCE_MANAGED  = "managed"
)

// Paths is the on-disk layout. Tests point it at a temp directory.
type Paths struct {
	Bin      string
	Bundled  string
	Settings string
	Tmp      string
}

// paths_under returns the layout rooted at a TDrive data directory.
func paths_under(data_dir string) Paths {
	return Paths{
		Bin:      filepath.Join(data_dir, BIN_DIR),
		Settings: filepath.Join(data_dir, SETTINGS_FILE),
		Tmp:      filepath.Join(data_dir, TMP_DIR),
	}
}

// exe_name adds the platform executable suffix.
func exe_name(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}
	return tool
}

// ToolStatus is what the setup state shows for one tool.
type ToolStatus struct {
	Name    string `json:"name"`
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Source  string `json:"source"`
	Version string `json:"version,omitempty"`
}

// Status is the combined tool state the dialog reads before it lays out.
type Status struct {
	YtDlp  ToolStatus `json:"yt_dlp"`
	FFmpeg ToolStatus `json:"ffmpeg"`
	// CanInstall is false on platforms where the managed download is not
	// published. The dialog then only offers the override path.
	CanInstall bool `json:"can_install"`
}

// tool_settings is the persisted override block.
type tool_settings struct {
	YtDlpPath  string `json:"yt_dlp_path,omitempty"`
	FFmpegPath string `json:"ffmpeg_path,omitempty"`
}

// Tools locates yt-dlp and ffmpeg. The order is: the user's override path, the
// verified copy bundled beside the app, an executable on PATH, then the
// app-managed copy in Paths.Bin.
type Tools struct {
	paths     Paths
	look_path func(string) (string, error)
	mu        sync.Mutex
}

// NewTools builds the locator for a data directory.
func NewTools(data_dir string) *Tools {
	paths := paths_under(data_dir)
	// Release packages place verified helpers beside the application in
	// "tools". Resolve this lazily at startup so the same locator also works
	// for portable zip installs and in-app updates.
	if executable, err := os.Executable(); err == nil {
		paths.Bundled = filepath.Join(filepath.Dir(executable), "tools")
	}
	return &Tools{paths: paths, look_path: exec.LookPath}
}

// Paths returns the on-disk layout the tools use.
func (t *Tools) Paths() Paths {
	return t.paths
}

// locate returns the first usable executable for tool and where it came from.
func (t *Tools) locate(tool string) (string, string, error) {
	override := t.load_settings().path_for(tool)
	if override != "" {
		if is_regular_file(override) {
			return override, SOURCE_OVERRIDE, nil
		}
		slog.Warn("mediadl: override tool path is missing", "tool", tool)
	}
	bundled := filepath.Join(t.paths.Bundled, exe_name(tool))
	if t.paths.Bundled != "" && is_regular_file(bundled) {
		return bundled, SOURCE_BUNDLED, nil
	}
	if found, err := t.look_path(exe_name(tool)); err == nil && found != "" {
		return found, SOURCE_PATH, nil
	}
	managed := filepath.Join(t.paths.Bin, exe_name(tool))
	if is_regular_file(managed) {
		return managed, SOURCE_MANAGED, nil
	}
	return "", "", errToolNotFound
}

// Locate returns the executable path for tool, or an error when none is found.
func (t *Tools) Locate(tool string) (string, error) {
	path, _, err := t.locate(tool)
	return path, err
}

// Status reports both tools without running them.
func (t *Tools) Status() Status {
	status := Status{CanInstall: runtime.GOOS == "windows"}
	status.YtDlp = t.tool_status(TOOL_YTDLP)
	status.FFmpeg = t.tool_status(TOOL_FFMPEG)
	return status
}

func (t *Tools) tool_status(tool string) ToolStatus {
	path, source, err := t.locate(tool)
	if err != nil {
		return ToolStatus{Name: tool}
	}
	return ToolStatus{Name: tool, Found: true, Path: path, Source: source}
}

// SetOverride stores a user-chosen path for tool. An empty path clears it. The
// file must exist and be named like the tool, so the setting cannot point the
// downloader at an arbitrary program.
func (t *Tools) SetOverride(tool string, path string) error {
	if tool != TOOL_YTDLP && tool != TOOL_FFMPEG {
		return fmt.Errorf("unknown tool %q", tool)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	settings := t.load_settings()
	if path == "" {
		settings.set_path(tool, "")
		return t.save_settings(settings)
	}
	if !is_regular_file(path) {
		return errors.New("the selected file does not exist")
	}
	if !name_matches_tool(path, tool) {
		return fmt.Errorf("the selected file must be %s", exe_name(tool))
	}
	settings.set_path(tool, path)
	return t.save_settings(settings)
}

func name_matches_tool(path string, tool string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == exe_name(tool) || base == tool || strings.HasPrefix(base, tool+"-")
}

func (t *Tools) load_settings() tool_settings {
	var settings tool_settings
	data, err := os.ReadFile(t.paths.Settings)
	if err != nil {
		return settings
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		slog.Warn("mediadl: ignoring unreadable tool settings", "error", err)
		return tool_settings{}
	}
	return settings
}

func (t *Tools) save_settings(settings tool_settings) error {
	if err := os.MkdirAll(filepath.Dir(t.paths.Settings), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return write_file_atomic(t.paths.Settings, data, 0o600)
}

func (s tool_settings) path_for(tool string) string {
	if tool == TOOL_YTDLP {
		return s.YtDlpPath
	}
	if tool == TOOL_FFMPEG {
		return s.FFmpegPath
	}
	return ""
}

func (s *tool_settings) set_path(tool string, path string) {
	if tool == TOOL_YTDLP {
		s.YtDlpPath = path
	}
	if tool == TOOL_FFMPEG {
		s.FFmpegPath = path
	}
}

func is_regular_file(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// write_file_atomic writes through a sibling temp file and renames it into place.
func write_file_atomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
