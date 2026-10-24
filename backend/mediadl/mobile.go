// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"context"
	"errors"
)

// SOURCE_EMBEDDED marks a tool that runs inside the app on a phone, rather than
// as an executable file the locator can find.
const SOURCE_EMBEDDED = "embedded"

// MobileStatus is what the phone's download engine reports about itself.
type MobileStatus struct {
	// Installed is true once the engine can run yt-dlp.
	Installed bool
	// Version is yt-dlp's version string, when the engine knows it.
	Version string
	// FFmpeg is true when the engine can merge streams and extract audio.
	FFmpeg bool
}

// MobileEngine runs yt-dlp inside the host app. Android provides it through
// youtubedl-android, iOS through an embedded Python with yt-dlp. The desktop
// build never uses it: there yt-dlp is an executable the Runner starts.
//
// Run receives the same argv the desktop build passes to yt-dlp, so every flag
// and format choice is decided in one place. Cancelling ctx stops the work.
type MobileEngine interface {
	// Status reports whether the engine is installed and what it can do.
	Status(ctx context.Context) (MobileStatus, error)
	// Install prepares the engine. progress receives a stage name and a percent.
	Install(ctx context.Context, progress func(stage string, percent int)) error
	// Update upgrades yt-dlp inside the engine and returns a closing message.
	Update(ctx context.Context) (string, error)
	// Run executes yt-dlp with args, passing each output line to on_line.
	Run(ctx context.Context, args []string, on_line func(line string)) error
}

// mobile_runner adapts a MobileEngine to the Runner the job code already uses.
// The bin argument is ignored: the engine is the only executable there is.
type mobile_runner struct {
	engine MobileEngine
}

func (r mobile_runner) Run(ctx context.Context, _ string, args []string, on_line func(line string)) error {
	return r.engine.Run(ctx, args, on_line)
}

// MobileToolStatus maps the engine status onto the locator's Status shape,
// so the dialog reads one format on every platform.
func MobileToolStatus(status MobileStatus) Status {
	yt := ToolStatus{Name: TOOL_YTDLP}
	if status.Installed {
		yt = ToolStatus{Name: TOOL_YTDLP, Found: true, Source: SOURCE_EMBEDDED, Version: status.Version}
	}
	ff := ToolStatus{Name: TOOL_FFMPEG}
	if status.FFmpeg {
		ff = ToolStatus{Name: TOOL_FFMPEG, Found: true, Source: SOURCE_EMBEDDED}
	}
	return Status{YtDlp: yt, FFmpeg: ff, CanInstall: true}
}

// ensure_mobile_engine_ready refuses a download when the engine is not installed.
func ensure_mobile_engine_ready(status MobileStatus) error {
	if !status.Installed {
		return errors.New("yt-dlp is not installed")
	}
	return nil
}
