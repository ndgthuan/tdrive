// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"TDrive/backend/mediadl"
)

// MEDIA_DL_HOST_EVENT carries one request from Go to the web view, which relays
// it to the native engine (youtubedl-android on Android, embedded Python on iOS).
// Go cannot call the native side directly, so the web view is the bridge.
const MEDIA_DL_HOST_EVENT = "media_dl:host"

// MEDIA_HOST_STATUS_TIMEOUT bounds a status read; the native side answers at once.
const MEDIA_HOST_STATUS_TIMEOUT = 30 * time.Second

var errMediaHostUnavailable = errors.New("the video downloader is not available on this device")

// MediaHostRequest is one call the native engine must answer. Op is "status",
// "install", "update", "run", or "cancel". A "cancel" request has no reply.
type MediaHostRequest struct {
	RequestID string   `json:"request_id"`
	Op        string   `json:"op"`
	Args      []string `json:"args,omitempty"`
}

type media_host_reply struct {
	result string
	err    error
}

type media_host_call struct {
	on_line func(string)
	done    chan media_host_reply
}

// media_host_broker pairs each request emitted to the web view with the reply
// the web view sends back. Lines reported while a call runs go to its on_line.
type media_host_broker struct {
	emit func(string, ...any)

	mu    sync.Mutex
	calls map[string]*media_host_call
}

func new_media_host_broker(emit func(string, ...any)) *media_host_broker {
	return &media_host_broker{emit: emit, calls: make(map[string]*media_host_call)}
}

// call sends one request and waits for its reply. Cancelling ctx sends a cancel
// request and returns at once, so a stalled webview never blocks a job.
func (b *media_host_broker) call(ctx context.Context, op string, args []string, on_line func(string)) (string, error) {
	id, err := new_media_host_request_id()
	if err != nil {
		return "", err
	}
	call := &media_host_call{on_line: on_line, done: make(chan media_host_reply, 1)}
	b.mu.Lock()
	b.calls[id] = call
	b.mu.Unlock()
	b.emit(MEDIA_DL_HOST_EVENT, MediaHostRequest{RequestID: id, Op: op, Args: args})
	select {
	case reply := <-call.done:
		return reply.result, reply.err
	case <-ctx.Done():
		b.forget(id)
		b.emit(MEDIA_DL_HOST_EVENT, MediaHostRequest{RequestID: id, Op: "cancel"})
		return "", ctx.Err()
	}
}

func (b *media_host_broker) forget(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.calls, id)
}

// resolve delivers the native answer for one request. failure, when set, is the
// message the user will read; it wins over result.
func (b *media_host_broker) resolve(id string, result string, failure string) error {
	b.mu.Lock()
	call, ok := b.calls[id]
	if ok {
		delete(b.calls, id)
	}
	b.mu.Unlock()
	if !ok {
		return errors.New("that download request is no longer waiting")
	}
	var err error
	if failure != "" {
		err = errors.New(failure)
	}
	call.done <- media_host_reply{result: result, err: err}
	return nil
}

// report_line passes one line of native output to the request waiting for it.
func (b *media_host_broker) report_line(id string, line string) error {
	b.mu.Lock()
	call, ok := b.calls[id]
	b.mu.Unlock()
	if !ok {
		return errors.New("that download request is no longer waiting")
	}
	if call.on_line != nil {
		call.on_line(line)
	}
	return nil
}

func new_media_host_request_id() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// media_host_engine implements mediadl.MobileEngine on top of the broker. Each
// method is one native request; the native side answers in the shapes below.
//
//	status  -> JSON {"installed":bool,"version":string,"ffmpeg":bool}
//	install -> progress lines "stage|percent", then an empty result
//	update  -> the closing message as the result
//	run     -> yt-dlp output lines, then an empty result
type media_host_engine struct {
	broker *media_host_broker
}

var _ mediadl.MobileEngine = media_host_engine{}

type media_host_status_payload struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	FFmpeg    bool   `json:"ffmpeg"`
}

func (e media_host_engine) Status(ctx context.Context) (mediadl.MobileStatus, error) {
	raw, err := e.broker.call(ctx, "status", nil, nil)
	if err != nil {
		return mediadl.MobileStatus{}, err
	}
	var payload media_host_status_payload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return mediadl.MobileStatus{}, errors.New("the video downloader sent an unreadable status")
	}
	return mediadl.MobileStatus{Installed: payload.Installed, Version: payload.Version, FFmpeg: payload.FFmpeg}, nil
}

func (e media_host_engine) Install(ctx context.Context, progress func(stage string, percent int)) error {
	_, err := e.broker.call(ctx, "install", nil, func(line string) {
		stage, percent, ok := parse_install_line(line)
		if ok {
			progress(stage, percent)
		}
	})
	return err
}

func (e media_host_engine) Update(ctx context.Context) (string, error) {
	return e.broker.call(ctx, "update", nil, nil)
}

func (e media_host_engine) Run(ctx context.Context, args []string, on_line func(line string)) error {
	_, err := e.broker.call(ctx, "run", args, on_line)
	return err
}

// parse_install_line reads one "stage|percent" progress line from the native side.
func parse_install_line(line string) (string, int, bool) {
	raw_stage, raw_percent, found := strings.Cut(strings.TrimSpace(line), "|")
	stage := strings.TrimSpace(raw_stage)
	if !found || stage == "" {
		return "", 0, false
	}
	percent, err := strconv.Atoi(strings.TrimSpace(raw_percent))
	if err != nil {
		return "", 0, false
	}
	return stage, percent, true
}

// ResolveMediaDownloadHost is called by the web view with the native answer to
// one request. failure, when set, is shown to the user.
func (s *MediaDownloadService) ResolveMediaDownloadHost(request_id string, result string, failure string) error {
	if s.broker == nil {
		return errMediaHostUnavailable
	}
	return s.broker.resolve(request_id, result, failure)
}

// ReportMediaDownloadLine is called by the web view with one line of native
// output for a request that is still running.
func (s *MediaDownloadService) ReportMediaDownloadLine(request_id string, line string) error {
	if s.broker == nil {
		return errMediaHostUnavailable
	}
	return s.broker.report_line(request_id, line)
}
