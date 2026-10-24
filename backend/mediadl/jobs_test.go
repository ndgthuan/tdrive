package mediadl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// fake_runner plays back scripted yt-dlp output. It can write the output file
// into the -P directory, block until cancelled, or fail like yt-dlp would.
type fake_runner struct {
	lines   []string
	err     error
	write   string
	block   bool
	started chan struct{}

	mu   sync.Mutex
	bin  string
	args []string
}

func (f *fake_runner) Run(ctx context.Context, bin string, args []string, on_line func(string)) error {
	f.mu.Lock()
	f.bin = bin
	f.args = append([]string(nil), args...)
	f.mu.Unlock()
	if f.started != nil {
		close(f.started)
	}
	dir := ""
	if i := slices.Index(args, "-P"); i >= 0 {
		dir = args[i+1]
	}
	if f.write != "" && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, f.write), []byte("media"), 0o600); err != nil {
			return err
		}
	}
	for _, line := range f.lines {
		on_line(line)
	}
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

// fake_destination records uploads and can fail them.
type fake_destination struct {
	mu            sync.Mutex
	uploads       []string
	channels      []int64
	folder        string
	upload_err    error
	block         bool
	folders_asked []string
}

func (d *fake_destination) ResolveFolder(ctx context.Context, channel_id int64, folder_id string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.folders_asked = append(d.folders_asked, folder_id)
	if folder_id != "" {
		return folder_id, nil
	}
	return d.folder, nil
}

func (d *fake_destination) UploadFile(ctx context.Context, channel_id int64, file_path string, folder_id string) error {
	if d.block {
		<-ctx.Done()
		return ctx.Err()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channels = append(d.channels, channel_id)
	d.uploads = append(d.uploads, filepath.Base(file_path)+"@"+folder_id)
	return d.upload_err
}

// event_log collects emitted events and signals when a job reaches a terminal stage.
type event_log struct {
	mu       sync.Mutex
	events   []Event
	terminal chan Event
}

func new_event_log() *event_log {
	return &event_log{terminal: make(chan Event, 16)}
}

func (l *event_log) emit(e Event) {
	l.mu.Lock()
	l.events = append(l.events, e)
	l.mu.Unlock()
	if is_terminal(e.Stage) {
		l.terminal <- e
	}
}

func (l *event_log) stages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	stages := make([]string, 0, len(l.events))
	for _, e := range l.events {
		if len(stages) == 0 || stages[len(stages)-1] != e.Stage {
			stages = append(stages, e.Stage)
		}
	}
	return stages
}

func wait_terminal(t *testing.T, log *event_log) Event {
	t.Helper()
	select {
	case e := <-log.terminal:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("job did not reach a terminal stage")
	}
	return Event{}
}

func new_job_manager(t *testing.T, runner Runner, dest Destination, log *event_log) (*Manager, *Tools) {
	t.Helper()
	tools := new_test_tools(t)
	write_fake_tool(t, filepath.Join(tools.paths.Bin, exe_name(TOOL_YTDLP)))
	manager := NewManager(tools, runner, dest, log.emit)
	return manager, tools
}

func valid_request() StartRequest {
	return StartRequest{URL: "https://example.com/watch?v=abc", PresetOrFormatID: PRESET_720P}
}

func TestStateMachineAllowsOnlyForwardMoves(t *testing.T) {
	allowed := [][2]string{
		{STAGE_DOWNLOADING, STAGE_MERGING},
		{STAGE_DOWNLOADING, STAGE_UPLOADING},
		{STAGE_MERGING, STAGE_UPLOADING},
		{STAGE_UPLOADING, STAGE_DONE},
		{STAGE_UPLOADING, STAGE_CANCELLED},
		{STAGE_DOWNLOADING, STAGE_ERROR},
	}
	for _, edge := range allowed {
		if !can_transition(edge[0], edge[1]) {
			t.Fatalf("%s -> %s should be allowed", edge[0], edge[1])
		}
	}
	refused := [][2]string{
		{STAGE_DONE, STAGE_DOWNLOADING},
		{STAGE_ERROR, STAGE_UPLOADING},
		{STAGE_CANCELLED, STAGE_DONE},
		{STAGE_MERGING, STAGE_DOWNLOADING},
		{STAGE_DOWNLOADING, STAGE_DONE},
	}
	for _, edge := range refused {
		if can_transition(edge[0], edge[1]) {
			t.Fatalf("%s -> %s must be refused", edge[0], edge[1])
		}
	}
}

func TestSuccessfulJobUploadsAndRemovesTempFiles(t *testing.T) {
	log := new_event_log()
	dest := &fake_destination{folder: "downloads-folder"}
	runner := &fake_runner{
		lines: []string{
			"[download] Destination: clip.mp4",
			"TDLP|512|1024|NA|2048|3",
			"TDLP|1024|1024|NA|2048|0",
			`[Merger] Merging formats into "clip.mp4"`,
		},
		write: "clip [abc].mp4",
	}
	manager, tools := new_job_manager(t, runner, dest, log)
	id, err := manager.Start(context.Background(), 42, valid_request())
	if err != nil {
		t.Fatal(err)
	}
	done := wait_terminal(t, log)
	if done.Stage != STAGE_DONE || done.JobID != id || done.FileName != "clip [abc].mp4" {
		t.Fatalf("terminal event = %+v", done)
	}
	stages := log.stages()
	want := []string{STAGE_DOWNLOADING, STAGE_MERGING, STAGE_UPLOADING, STAGE_DONE}
	if !slices.Equal(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
	if len(dest.uploads) != 1 || dest.uploads[0] != "clip [abc].mp4@downloads-folder" || dest.channels[0] != 42 {
		t.Fatalf("upload calls = %v %v", dest.uploads, dest.channels)
	}
	if _, err := os.Stat(filepath.Join(tools.paths.Tmp, id)); !os.IsNotExist(err) {
		t.Fatalf("temp directory must be removed after success: %v", err)
	}
	if runner.bin == "" || slices.Contains(runner.args, "--exec") {
		t.Fatalf("runner invoked with %q %v", runner.bin, runner.args)
	}
}

func TestDownloadErrorReportsYtDlpMessage(t *testing.T) {
	log := new_event_log()
	runner := &fake_runner{lines: []string{"ERROR: [youtube] abc: Video unavailable"}, err: errors.New("exit status 1")}
	manager, _ := new_job_manager(t, runner, &fake_destination{folder: "f"}, log)
	if _, err := manager.Start(context.Background(), 1, valid_request()); err != nil {
		t.Fatal(err)
	}
	done := wait_terminal(t, log)
	if done.Stage != STAGE_ERROR || done.Error != "[youtube] abc: Video unavailable" {
		t.Fatalf("error event = %+v", done)
	}
}

func TestUploadFailureMarksJobError(t *testing.T) {
	log := new_event_log()
	dest := &fake_destination{folder: "f", upload_err: errors.New("quota reached")}
	manager, tools := new_job_manager(t, &fake_runner{write: "clip.mp4"}, dest, log)
	id, err := manager.Start(context.Background(), 1, valid_request())
	if err != nil {
		t.Fatal(err)
	}
	done := wait_terminal(t, log)
	if done.Stage != STAGE_ERROR || done.Error != "quota reached" {
		t.Fatalf("error event = %+v", done)
	}
	if _, err := os.Stat(filepath.Join(tools.paths.Tmp, id)); !os.IsNotExist(err) {
		t.Fatal("temp directory must be removed after a failed upload")
	}
}

func TestMissingOutputFileIsAnError(t *testing.T) {
	log := new_event_log()
	manager, _ := new_job_manager(t, &fake_runner{}, &fake_destination{folder: "f"}, log)
	if _, err := manager.Start(context.Background(), 1, valid_request()); err != nil {
		t.Fatal(err)
	}
	if done := wait_terminal(t, log); done.Stage != STAGE_ERROR {
		t.Fatalf("expected error, got %+v", done)
	}
}

func TestCancelStopsRunningJobAndCleansUp(t *testing.T) {
	log := new_event_log()
	started := make(chan struct{})
	runner := &fake_runner{block: true, started: started}
	manager, tools := new_job_manager(t, runner, &fake_destination{folder: "f"}, log)
	id, err := manager.Start(context.Background(), 1, valid_request())
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if err := manager.Cancel(id); err != nil {
		t.Fatalf("cancel refused: %v", err)
	}
	if done := wait_terminal(t, log); done.Stage != STAGE_CANCELLED {
		t.Fatalf("expected cancelled, got %+v", done)
	}
	if err := manager.Cancel(id); err == nil {
		t.Fatal("cancelling a finished job must report it")
	}
	if err := manager.Cancel("missing"); err == nil {
		t.Fatal("unknown job must be reported")
	}
	if _, err := os.Stat(filepath.Join(tools.paths.Tmp, id)); !os.IsNotExist(err) {
		t.Fatal("temp directory must be removed after cancel")
	}
}

func TestCancelDuringUploadIsCancelledNotError(t *testing.T) {
	log := new_event_log()
	dest := &fake_destination{folder: "f", block: true}
	manager, _ := new_job_manager(t, &fake_runner{write: "clip.mp4"}, dest, log)
	id, err := manager.Start(context.Background(), 1, valid_request())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		views := manager.ListJobs()
		if len(views) == 1 && views[0].Stage == STAGE_UPLOADING {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job never reached uploading")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := manager.Cancel(id); err != nil {
		t.Fatal(err)
	}
	if done := wait_terminal(t, log); done.Stage != STAGE_CANCELLED {
		t.Fatalf("expected cancelled, got %+v", done)
	}
}

func TestStartRefusesWithoutToolOrValidLink(t *testing.T) {
	log := new_event_log()
	tools := new_test_tools(t)
	manager := NewManager(tools, &fake_runner{}, &fake_destination{folder: "f"}, log.emit)
	if _, err := manager.Start(context.Background(), 1, valid_request()); err == nil {
		t.Fatal("start without yt-dlp must fail")
	}
	write_fake_tool(t, filepath.Join(tools.paths.Bin, exe_name(TOOL_YTDLP)))
	if _, err := manager.Start(context.Background(), 1, StartRequest{URL: "file:///x", PresetOrFormatID: PRESET_BEST}); err == nil {
		t.Fatal("non-http link must fail")
	}
	if _, err := manager.Start(context.Background(), 0, valid_request()); err == nil {
		t.Fatal("start without a drive must fail")
	}
	if len(manager.ListJobs()) != 0 {
		t.Fatal("refused starts must not leave jobs behind")
	}
}

func TestProbeUsesRunnerAndParsesJSON(t *testing.T) {
	log := new_event_log()
	runner := &fake_runner{lines: []string{sample_probe_json}}
	manager, _ := new_job_manager(t, runner, &fake_destination{folder: "f"}, log)
	result, err := manager.Probe(context.Background(), "https://example.com/v")
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Launch clip" || !slices.Contains(runner.args, "-J") {
		t.Fatalf("probe = %+v args=%v", result, runner.args)
	}
}

func TestNewManagerClearsStaleTempDirectories(t *testing.T) {
	tools := new_test_tools(t)
	stale := filepath.Join(tools.paths.Tmp, "old")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	NewManager(tools, &fake_runner{}, &fake_destination{}, nil)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale temp downloads must be swept at construction")
	}
}
