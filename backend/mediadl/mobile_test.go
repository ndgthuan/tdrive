// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fake_mobile_engine stands in for the phone's yt-dlp host. It records the argv
// it was given and can write an output file or block until cancelled.
type fake_mobile_engine struct {
	status      MobileStatus
	status_err  error
	lines       []string
	run_err     error
	write       string
	block       bool
	started     chan struct{}
	update_text string
	update_err  error

	mu   sync.Mutex
	args []string
}

func (f *fake_mobile_engine) Status(context.Context) (MobileStatus, error) {
	return f.status, f.status_err
}

func (f *fake_mobile_engine) Install(_ context.Context, progress func(string, int)) error {
	progress("downloading", 0)
	progress("done", 100)
	return nil
}

func (f *fake_mobile_engine) Update(context.Context) (string, error) {
	return f.update_text, f.update_err
}

func (f *fake_mobile_engine) Run(ctx context.Context, args []string, on_line func(string)) error {
	f.mu.Lock()
	f.args = append([]string(nil), args...)
	f.mu.Unlock()
	if f.started != nil {
		close(f.started)
	}
	if f.write != "" {
		if i := slices.Index(args, "-P"); i >= 0 {
			if err := os.MkdirAll(args[i+1], 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(args[i+1], f.write), []byte("media"), 0o600); err != nil {
				return err
			}
		}
	}
	for _, line := range f.lines {
		on_line(line)
	}
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.run_err
}

func (f *fake_mobile_engine) recorded_args() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.args...)
}

func new_mobile_manager(t *testing.T, engine MobileEngine, dest Destination, log *event_log) *Manager {
	t.Helper()
	return NewMobileManager(new_test_tools(t), engine, dest, log.emit)
}

func TestMobileStartRefusesWhenEngineIsNotInstalled(t *testing.T) {
	engine := &fake_mobile_engine{status: MobileStatus{Installed: false}}
	dest := &fake_destination{}
	manager := new_mobile_manager(t, engine, dest, new_event_log())

	_, err := manager.Start(context.Background(), 7, StartRequest{URL: "https://example.com/v", PresetOrFormatID: PRESET_BEST})
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected a not-installed refusal, got %v", err)
	}
	if engine.recorded_args() != nil {
		t.Fatal("engine must not run when it is not installed")
	}
}

func TestMobileStartSurfacesEngineStatusError(t *testing.T) {
	engine := &fake_mobile_engine{status_err: errors.New("host is gone")}
	manager := new_mobile_manager(t, engine, &fake_destination{}, new_event_log())

	_, err := manager.Start(context.Background(), 7, StartRequest{URL: "https://example.com/v", PresetOrFormatID: PRESET_BEST})
	if err == nil || err.Error() != "host is gone" {
		t.Fatalf("expected the status error, got %v", err)
	}
}

func TestMobileProbeReadsMetadataThroughEngine(t *testing.T) {
	engine := &fake_mobile_engine{status: MobileStatus{Installed: true}, lines: []string{sample_probe_json}}
	manager := new_mobile_manager(t, engine, &fake_destination{}, new_event_log())

	result, err := manager.Probe(context.Background(), "https://example.com/v")
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Launch clip" {
		t.Fatalf("title = %q", result.Title)
	}
	args := engine.recorded_args()
	if !slices.Contains(args, "-J") {
		t.Fatalf("probe argv missing -J: %v", args)
	}
}

func TestMobileDownloadUsesEmbeddedFFmpegWithoutLocationFlag(t *testing.T) {
	engine := &fake_mobile_engine{status: MobileStatus{Installed: true, FFmpeg: true}, write: "clip.mp4"}
	dest := &fake_destination{}
	log := new_event_log()
	manager := new_mobile_manager(t, engine, dest, log)

	if _, err := manager.Start(context.Background(), 7, StartRequest{URL: "https://example.com/v", PresetOrFormatID: PRESET_BEST, TargetFolderID: "f1"}); err != nil {
		t.Fatal(err)
	}
	done := wait_terminal(t, log)
	if done.Stage != STAGE_DONE {
		t.Fatalf("stage = %s, error = %s", done.Stage, done.Error)
	}
	args := engine.recorded_args()
	if !slices.Contains(args, "bv*+ba/b") {
		t.Fatalf("expected the merging selector, got %v", args)
	}
	if slices.Contains(args, "--ffmpeg-location") {
		t.Fatalf("embedded ffmpeg must not pass a location: %v", args)
	}
	if len(dest.uploads) != 1 || dest.uploads[0] != "clip.mp4@f1" {
		t.Fatalf("uploads = %v", dest.uploads)
	}
}

func TestMobileDownloadWithoutFFmpegFallsBackToSingleFileSelector(t *testing.T) {
	engine := &fake_mobile_engine{status: MobileStatus{Installed: true, FFmpeg: false}, write: "clip.mp4"}
	log := new_event_log()
	manager := new_mobile_manager(t, engine, &fake_destination{}, log)

	if _, err := manager.Start(context.Background(), 7, StartRequest{URL: "https://example.com/v", PresetOrFormatID: PRESET_BEST}); err != nil {
		t.Fatal(err)
	}
	if done := wait_terminal(t, log); done.Stage != STAGE_DONE {
		t.Fatalf("stage = %s, error = %s", done.Stage, done.Error)
	}
	if args := engine.recorded_args(); !slices.Contains(args, "b") {
		t.Fatalf("expected the single-file selector, got %v", args)
	}
}

func TestMobileDownloadWithNativeMuxerUsesAVCAndAAC(t *testing.T) {
	engine := &fake_mobile_engine{status: MobileStatus{Installed: true, NativeMerge: true}, write: "clip.mp4"}
	log := new_event_log()
	manager := new_mobile_manager(t, engine, &fake_destination{}, log)
	if _, err := manager.Start(context.Background(), 7, StartRequest{URL: "https://example.com/v", PresetOrFormatID: PRESET_BEST}); err != nil {
		t.Fatal(err)
	}
	if done := wait_terminal(t, log); done.Stage != STAGE_DONE {
		t.Fatalf("stage = %s, error = %s", done.Stage, done.Error)
	}
	args := engine.recorded_args()
	if !slices.Contains(args, "bv*[ext=mp4][vcodec^=avc1]+ba[ext=m4a][acodec^=mp4a]/b[ext=mp4][vcodec^=avc1][acodec^=mp4a]") {
		t.Fatalf("native merger must only request iOS-compatible streams: %v", args)
	}
	if !slices.Contains(args, "--merge-output-format") || slices.Contains(args, "-x") || slices.Contains(args, "--ffmpeg-location") {
		t.Fatalf("native merger must not request ffmpeg conversions: %v", args)
	}
}

func TestMobileCancelStopsEngineRun(t *testing.T) {
	engine := &fake_mobile_engine{status: MobileStatus{Installed: true}, block: true, started: make(chan struct{})}
	log := new_event_log()
	manager := new_mobile_manager(t, engine, &fake_destination{}, log)

	id, err := manager.Start(context.Background(), 7, StartRequest{URL: "https://example.com/v", PresetOrFormatID: PRESET_BEST})
	if err != nil {
		t.Fatal(err)
	}
	<-engine.started
	if err := manager.Cancel(id); err != nil {
		t.Fatal(err)
	}
	if done := wait_terminal(t, log); done.Stage != STAGE_CANCELLED {
		t.Fatalf("stage = %s", done.Stage)
	}
}

func TestMobileStatusMapsToToolStatus(t *testing.T) {
	status := MobileToolStatus(MobileStatus{Installed: true, Version: "2026.10.12", FFmpeg: false, NativeMerge: true})
	if !status.YtDlp.Found || status.YtDlp.Source != SOURCE_EMBEDDED || status.YtDlp.Version != "2026.10.12" {
		t.Fatalf("yt-dlp status wrong: %+v", status.YtDlp)
	}
	if status.FFmpeg.Found {
		t.Fatalf("ffmpeg should be absent: %+v", status.FFmpeg)
	}
	if !status.NativeMerge {
		t.Fatal("native merger capability must remain separate from ffmpeg")
	}
	if !status.CanInstall {
		t.Fatal("mobile engines can always be installed")
	}
	missing := MobileToolStatus(MobileStatus{})
	if missing.YtDlp.Found {
		t.Fatal("an uninstalled engine must not report found")
	}
}
