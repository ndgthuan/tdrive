// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package mediadl

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Stage names shared with the frontend. Terminal stages never change again.
const (
	STAGE_DOWNLOADING = "downloading"
	STAGE_MERGING     = "merging"
	STAGE_UPLOADING   = "uploading"
	STAGE_DONE        = "done"
	STAGE_ERROR       = "error"
	STAGE_CANCELLED   = "cancelled"
)

// Limits on concurrency, history and event rate.
const (
	MAX_ACTIVE_JOBS   = 3
	MAX_HISTORY       = 50
	PROBE_TIMEOUT     = 90 * time.Second
	PROGRESS_INTERVAL = 250 * time.Millisecond
	MAX_KEPT_LINES    = 200
)

// Event is the payload of the media_dl:progress event.
type Event struct {
	JobID    string  `json:"job_id"`
	Stage    string  `json:"stage"`
	Percent  float64 `json:"percent"`
	Speed    float64 `json:"speed"`
	ETA      int64   `json:"eta"`
	Title    string  `json:"title"`
	Error    string  `json:"error,omitempty"`
	FileName string  `json:"file_name,omitempty"`
	FolderID string  `json:"folder_id,omitempty"`
	DriveID  int64   `json:"drive_id"`
}

// JobView is a snapshot of one job for ListJobs.
type JobView struct {
	JobID     string  `json:"job_id"`
	URL       string  `json:"url"`
	Choice    string  `json:"choice"`
	Title     string  `json:"title"`
	Stage     string  `json:"stage"`
	Percent   float64 `json:"percent"`
	Speed     float64 `json:"speed"`
	ETA       int64   `json:"eta"`
	Error     string  `json:"error,omitempty"`
	FileName  string  `json:"file_name,omitempty"`
	FolderID  string  `json:"folder_id,omitempty"`
	DriveID   int64   `json:"drive_id"`
	StartedAt int64   `json:"started_at"`
}

// StartRequest is the payload of the Start call. DriveID is the drive the
// download is saved to, and TargetFolderID the folder in that drive. An empty
// TargetFolderID means the drive's root; DriveID 0 means the active drive.
type StartRequest struct {
	URL              string `json:"url"`
	PresetOrFormatID string `json:"preset_or_format_id"`
	TargetFolderID   string `json:"target_folder_id"`
	DriveID          int64  `json:"drive_id"`
}

// Destination is the upload side. The app supplies the implementation so the
// download code never talks to Telegram or the folder service directly.
type Destination interface {
	// CheckTarget reports whether uploads may go to folder_id in the drive, before
	// any download starts. An empty folder_id is the drive's root.
	CheckTarget(ctx context.Context, channel_id int64, folder_id string) error
	// UploadFile uploads one local file into folder_id on the drive. An empty
	// folder_id is the drive's root.
	UploadFile(ctx context.Context, channel_id int64, file_path string, folder_id string) error
}

// allowed_transitions lists the stage changes a job may make. Terminal stages
// have no outgoing edges.
var allowed_transitions = map[string][]string{
	STAGE_DOWNLOADING: {STAGE_MERGING, STAGE_UPLOADING, STAGE_ERROR, STAGE_CANCELLED},
	STAGE_MERGING:     {STAGE_UPLOADING, STAGE_ERROR, STAGE_CANCELLED},
	STAGE_UPLOADING:   {STAGE_DONE, STAGE_ERROR, STAGE_CANCELLED},
}

// can_transition reports whether a job may move from one stage to another.
func can_transition(from string, to string) bool {
	for _, next := range allowed_transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

func is_terminal(stage string) bool {
	return stage == STAGE_DONE || stage == STAGE_ERROR || stage == STAGE_CANCELLED
}

type job struct {
	view       JobView
	channel_id int64
	folder_id  string
	dir        string
	cancel     context.CancelFunc
	last_emit  time.Time
	lines      []string
}

// Manager owns the download jobs. Its methods are safe for concurrent use.
type Manager struct {
	tools  *Tools
	runner Runner
	dest   Destination
	emit   func(Event)
	// mobile, when set, runs yt-dlp inside the host app instead of as a file.
	mobile MobileEngine

	mu   sync.Mutex
	jobs map[string]*job
}

// NewManager builds a manager and clears the temp root left by an earlier run.
// Call it before any job starts, which is why the sweep is safe here.
func NewManager(tools *Tools, runner Runner, dest Destination, emit func(Event)) *Manager {
	if emit == nil {
		emit = func(Event) {}
	}
	if err := os.RemoveAll(tools.paths.Tmp); err != nil {
		slog.Warn("mediadl: cannot clear temp downloads", "error", err)
	}
	return &Manager{tools: tools, runner: runner, dest: dest, emit: emit, jobs: make(map[string]*job)}
}

// NewMobileManager builds a manager whose yt-dlp runs inside the host app. Tools
// are still stored under tools.paths for the job temp directories; no executable
// is located. It clears the temp root like NewManager.
func NewMobileManager(tools *Tools, engine MobileEngine, dest Destination, emit func(Event)) *Manager {
	m := NewManager(tools, mobile_runner{engine: engine}, dest, emit)
	m.mobile = engine
	return m
}

// engine_binary returns the executable Run starts. On a phone there is no file
// to locate: the in-app engine must be installed, and the logical name is passed
// through, which the mobile runner ignores.
func (m *Manager) engine_binary(ctx context.Context) (string, error) {
	if m.mobile == nil {
		bin, err := m.tools.Locate(TOOL_YTDLP)
		if err != nil {
			return "", errors.New("yt-dlp is not installed")
		}
		return bin, nil
	}
	status, err := m.mobile.Status(ctx)
	if err != nil {
		return "", err
	}
	if err := ensure_mobile_engine_ready(status); err != nil {
		return "", err
	}
	return TOOL_YTDLP, nil
}

// start_tools resolves what one job needs: the yt-dlp executable, and how ffmpeg
// is available. On desktop ffmpeg is a path; on a phone the engine carries it.
func (m *Manager) start_tools(ctx context.Context) (ytdlp string, ffmpeg_path string, ffmpeg_embedded bool, err error) {
	if m.mobile != nil {
		status, status_err := m.mobile.Status(ctx)
		if status_err != nil {
			return "", "", false, status_err
		}
		if ready_err := ensure_mobile_engine_ready(status); ready_err != nil {
			return "", "", false, ready_err
		}
		return TOOL_YTDLP, "", status.FFmpeg, nil
	}
	ytdlp, err = m.tools.Locate(TOOL_YTDLP)
	if err != nil {
		return "", "", false, errors.New("yt-dlp is not installed")
	}
	if path, locate_err := m.tools.Locate(TOOL_FFMPEG); locate_err == nil {
		ffmpeg_path = path
	}
	return ytdlp, ffmpeg_path, false, nil
}

// Probe reads the metadata and quality list of one link without downloading.
func (m *Manager) Probe(ctx context.Context, raw_url string) (ProbeResult, error) {
	args, err := probe_args(raw_url)
	if err != nil {
		return ProbeResult{}, err
	}
	bin, err := m.engine_binary(ctx)
	if err != nil {
		return ProbeResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, PROBE_TIMEOUT)
	defer cancel()
	var lines []string
	runErr := m.runner.Run(ctx, bin, args, func(line string) {
		if len(lines) < MAX_KEPT_LINES || strings.HasPrefix(line, "{") {
			lines = append(lines, line)
		}
	})
	if ctx.Err() != nil {
		return ProbeResult{}, errors.New("reading the link timed out")
	}
	result, parse_err := parse_probe_output(lines)
	if parse_err != nil {
		if runErr != nil && len(lines) == 0 {
			return ProbeResult{}, fmt.Errorf("yt-dlp could not run: %w", runErr)
		}
		return ProbeResult{}, parse_err
	}
	return result, nil
}

// Start validates the request, checks the tools and begins a job. The download
// runs in the background under base, so cancelling base stops every job.
func (m *Manager) Start(base context.Context, channel_id int64, request StartRequest) (string, error) {
	if channel_id == 0 {
		return "", errors.New("no drive is selected")
	}
	if err := m.dest.CheckTarget(base, channel_id, request.TargetFolderID); err != nil {
		return "", err
	}
	ytdlp, ffmpeg_path, ffmpeg_embedded, err := m.start_tools(base)
	if err != nil {
		return "", err
	}
	id, err := new_job_id()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(m.tools.paths.Tmp, id)
	args, err := build_download_args(download_request{
		URL:            request.URL,
		Choice:         request.PresetOrFormatID,
		FFmpegPath:     ffmpeg_path,
		FFmpegEmbedded: ffmpeg_embedded,
		OutDir:         dir,
	})
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(base)
	j := &job{
		view: JobView{
			JobID:     id,
			URL:       strings.TrimSpace(request.URL),
			Choice:    request.PresetOrFormatID,
			Stage:     STAGE_DOWNLOADING,
			ETA:       -1,
			DriveID:   channel_id,
			StartedAt: time.Now().UnixMilli(),
		},
		channel_id: channel_id,
		folder_id:  request.TargetFolderID,
		dir:        dir,
		cancel:     cancel,
	}
	m.mu.Lock()
	if m.active_count() >= MAX_ACTIVE_JOBS {
		m.mu.Unlock()
		cancel()
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("finish a download before starting another (limit %d)", MAX_ACTIVE_JOBS)
	}
	m.jobs[id] = j
	m.trim_history()
	initial := event_of(j.view)
	m.mu.Unlock()
	m.emit(initial)

	go m.run(ctx, j, ytdlp, args)
	return id, nil
}

// Cancel stops a running job. The job reports the cancelled stage itself once its
// process has exited.
func (m *Manager) Cancel(job_id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[job_id]
	if !ok {
		return errors.New("download not found")
	}
	if is_terminal(j.view.Stage) {
		return errors.New("download already finished")
	}
	j.cancel()
	return nil
}

// ListJobs returns snapshots, newest first.
func (m *Manager) ListJobs() []JobView {
	m.mu.Lock()
	defer m.mu.Unlock()
	views := make([]JobView, 0, len(m.jobs))
	for _, j := range m.jobs {
		views = append(views, j.view)
	}
	sort.Slice(views, func(i, k int) bool { return views[i].StartedAt > views[k].StartedAt })
	return views
}

func (m *Manager) active_count() int {
	count := 0
	for _, j := range m.jobs {
		if !is_terminal(j.view.Stage) {
			count++
		}
	}
	return count
}

// trim_history drops the oldest finished jobs beyond MAX_HISTORY. Caller holds mu.
func (m *Manager) trim_history() {
	if len(m.jobs) <= MAX_HISTORY {
		return
	}
	finished := make([]*job, 0, len(m.jobs))
	for _, j := range m.jobs {
		if is_terminal(j.view.Stage) {
			finished = append(finished, j)
		}
	}
	sort.Slice(finished, func(i, k int) bool { return finished[i].view.StartedAt < finished[k].view.StartedAt })
	for _, j := range finished {
		if len(m.jobs) <= MAX_HISTORY {
			return
		}
		delete(m.jobs, j.view.JobID)
	}
}

// run executes one job from download through upload and always removes its
// temp directory on the way out.
func (m *Manager) run(ctx context.Context, j *job, ytdlp string, args []string) {
	defer j.cancel()
	defer func() { _ = os.RemoveAll(j.dir) }()

	runErr := m.runner.Run(ctx, ytdlp, args, func(line string) {
		m.handle_line(j, line)
	})
	m.mu.Lock()
	lines := append([]string(nil), j.lines...)
	m.mu.Unlock()
	if ctx.Err() != nil {
		m.finish(j, STAGE_CANCELLED, "", "")
		return
	}
	if runErr != nil {
		m.finish(j, STAGE_ERROR, first_nonempty(error_from_lines(lines), "the download failed"), "")
		return
	}

	output, err := find_output(j.dir)
	if err != nil {
		m.finish(j, STAGE_ERROR, "yt-dlp finished without producing a file", "")
		return
	}
	if !m.transition(j, STAGE_UPLOADING, nil) {
		return
	}
	m.mu.Lock()
	j.view.FolderID = j.folder_id
	m.mu.Unlock()
	err = m.dest.UploadFile(ctx, j.channel_id, output, j.folder_id)
	if ctx.Err() != nil {
		m.finish(j, STAGE_CANCELLED, "", "")
		return
	}
	if err != nil {
		m.finish(j, STAGE_ERROR, err.Error(), "")
		return
	}
	m.finish(j, STAGE_DONE, "", filepath.Base(output))
}

// handle_line routes one yt-dlp output line to progress, the merge stage, or the
// error buffer.
func (m *Manager) handle_line(j *job, line string) {
	if sample, ok := parse_progress_line(line); ok {
		m.report_progress(j, sample)
		return
	}
	if is_merging_line(line) {
		m.transition(j, STAGE_MERGING, func(view *JobView) { view.Percent = 100 })
		return
	}
	m.mu.Lock()
	if len(j.lines) >= MAX_KEPT_LINES {
		j.lines = j.lines[1:]
	}
	j.lines = append(j.lines, line)
	m.mu.Unlock()
}

func (m *Manager) report_progress(j *job, sample ProgressSample) {
	m.mu.Lock()
	if j.view.Stage != STAGE_DOWNLOADING {
		m.mu.Unlock()
		return
	}
	percent := sample.Percent()
	if percent >= 0 {
		j.view.Percent = percent
	}
	j.view.Speed = sample.Speed
	j.view.ETA = sample.ETA
	now := time.Now()
	due := now.Sub(j.last_emit) >= PROGRESS_INTERVAL || (percent >= 100)
	if due {
		j.last_emit = now
	}
	event := event_of(j.view)
	m.mu.Unlock()
	if due {
		m.emit(event)
	}
}

// transition moves a job to a new stage if the state machine allows it. mutate,
// when given, edits the view under the lock first. It returns false when the
// move is refused, which only happens for a job that already finished.
func (m *Manager) transition(j *job, to string, mutate func(view *JobView)) bool {
	m.mu.Lock()
	if !can_transition(j.view.Stage, to) {
		m.mu.Unlock()
		return false
	}
	j.view.Stage = to
	if to == STAGE_UPLOADING {
		j.view.Percent = 0
		j.view.Speed = 0
		j.view.ETA = -1
	}
	if mutate != nil {
		mutate(&j.view)
	}
	event := event_of(j.view)
	m.mu.Unlock()
	m.emit(event)
	return true
}

// finish records the terminal stage. It bypasses can_transition only for the
// error and cancel edges, which every non-terminal stage has.
func (m *Manager) finish(j *job, stage string, message string, file_name string) {
	m.mu.Lock()
	if is_terminal(j.view.Stage) {
		m.mu.Unlock()
		return
	}
	if !can_transition(j.view.Stage, stage) {
		m.mu.Unlock()
		slog.Warn("mediadl: refused terminal transition", "from", j.view.Stage, "to", stage)
		return
	}
	j.view.Stage = stage
	j.view.Error = message
	j.view.FileName = file_name
	j.view.Speed = 0
	j.view.ETA = -1
	if stage == STAGE_DONE {
		j.view.Percent = 100
	}
	event := event_of(j.view)
	m.mu.Unlock()
	// Remove the temp directory before the terminal event, so a reader that
	// reacts to it never finds the job's files still on disk.
	_ = os.RemoveAll(j.dir)
	m.emit(event)
}

func event_of(view JobView) Event {
	return Event{
		JobID:    view.JobID,
		Stage:    view.Stage,
		Percent:  view.Percent,
		Speed:    view.Speed,
		ETA:      view.ETA,
		Title:    view.Title,
		Error:    view.Error,
		FileName: view.FileName,
		FolderID: view.FolderID,
		DriveID:  view.DriveID,
	}
}

// find_output returns the finished media file in dir. Partial and sidecar files
// are ignored; when several remain, the largest is the video.
func find_output(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	best := ""
	var best_size int64 = -1
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || is_partial_name(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > best_size {
			best = filepath.Join(dir, name)
			best_size = info.Size()
		}
	}
	if best == "" {
		return "", errors.New("no output file")
	}
	return best, nil
}

func is_partial_name(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".part", ".ytdl", ".temp"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return strings.Contains(lower, ".part-frag")
}

func first_nonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func new_job_id() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
