// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"TDrive/backend/mediadl"
)

// host_responder records every request the broker emits and answers each one
// through the handler. The handler runs inside emit, after the call has been
// registered, which is how the real web view replies.
type host_responder struct {
	broker  *media_host_broker
	handler func(req MediaHostRequest)

	mu       sync.Mutex
	requests []MediaHostRequest
}

func (r *host_responder) emit(name string, data ...any) {
	if name != MEDIA_DL_HOST_EVENT || len(data) != 1 {
		return
	}
	req, ok := data[0].(MediaHostRequest)
	if !ok {
		return
	}
	r.mu.Lock()
	r.requests = append(r.requests, req)
	r.mu.Unlock()
	if r.handler != nil {
		r.handler(req)
	}
}

func (r *host_responder) ops() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ops := make([]string, 0, len(r.requests))
	for _, req := range r.requests {
		ops = append(ops, req.Op)
	}
	return ops
}

func new_responder(handler func(req MediaHostRequest, b *media_host_broker)) (*host_responder, *media_host_broker) {
	r := &host_responder{}
	r.broker = new_media_host_broker(r.emit)
	if handler != nil {
		r.handler = func(req MediaHostRequest) { handler(req, r.broker) }
	}
	return r, r.broker
}

func TestMediaHostStatusDecodesNativeAnswer(t *testing.T) {
	_, broker := new_responder(func(req MediaHostRequest, b *media_host_broker) {
		if req.Op == "status" {
			_ = b.resolve(req.RequestID, `{"installed":true,"version":"2026.10.12","ffmpeg":true}`, "")
		}
	})
	engine := media_host_engine{broker: broker}

	status, err := engine.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Installed || status.Version != "2026.10.12" || !status.FFmpeg {
		t.Fatalf("status = %+v", status)
	}
}

func TestMediaHostStatusReportsNativeFailure(t *testing.T) {
	_, broker := new_responder(func(req MediaHostRequest, b *media_host_broker) {
		_ = b.resolve(req.RequestID, "", "engine crashed")
	})
	_, err := media_host_engine{broker: broker}.Status(context.Background())
	if err == nil || err.Error() != "engine crashed" {
		t.Fatalf("expected the native failure, got %v", err)
	}
}

func TestMediaHostStatusRejectsUnreadableAnswer(t *testing.T) {
	_, broker := new_responder(func(req MediaHostRequest, b *media_host_broker) {
		_ = b.resolve(req.RequestID, "not json", "")
	})
	if _, err := (media_host_engine{broker: broker}).Status(context.Background()); err == nil {
		t.Fatal("an unreadable status must be an error")
	}
}

func TestMediaHostRunStreamsLinesBeforeFinishing(t *testing.T) {
	_, broker := new_responder(func(req MediaHostRequest, b *media_host_broker) {
		if req.Op != "run" {
			return
		}
		if !slices.Equal(req.Args, []string{"--", "https://example.com/v"}) {
			t.Errorf("args = %v", req.Args)
		}
		_ = b.report_line(req.RequestID, "first")
		_ = b.report_line(req.RequestID, "second")
		_ = b.resolve(req.RequestID, "", "")
	})
	var lines []string
	err := media_host_engine{broker: broker}.Run(context.Background(), []string{"--", "https://example.com/v"}, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(lines, []string{"first", "second"}) {
		t.Fatalf("lines = %v", lines)
	}
}

func TestMediaHostInstallReportsProgressLines(t *testing.T) {
	_, broker := new_responder(func(req MediaHostRequest, b *media_host_broker) {
		_ = b.report_line(req.RequestID, "downloading|40")
		_ = b.report_line(req.RequestID, "garbage")
		_ = b.resolve(req.RequestID, "", "")
	})
	var stages []string
	err := media_host_engine{broker: broker}.Install(context.Background(), func(stage string, percent int) {
		stages = append(stages, stage)
		if percent != 40 {
			t.Errorf("percent = %d", percent)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(stages, []string{"downloading"}) {
		t.Fatalf("stages = %v", stages)
	}
}

func TestMediaHostCancelSendsCancelAndReturnsAtOnce(t *testing.T) {
	responder, broker := new_responder(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := media_host_engine{broker: broker}.Run(ctx, []string{"--", "u"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancellation, got %v", err)
	}
	if !slices.Equal(responder.ops(), []string{"run", "cancel"}) {
		t.Fatalf("ops = %v", responder.ops())
	}
	cancel_id := responder.requests[1].RequestID
	if cancel_id != responder.requests[0].RequestID {
		t.Fatal("cancel must name the request it stops")
	}
	if err := broker.resolve(cancel_id, "", ""); err == nil {
		t.Fatal("a reply after cancel must be refused")
	}
	if err := broker.report_line(cancel_id, "late"); err == nil {
		t.Fatal("a line after cancel must be refused")
	}
}

func TestMediaHostUnknownRequestIsRefused(t *testing.T) {
	broker := new_media_host_broker(func(string, ...any) {})
	if err := broker.resolve("nope", "", ""); err == nil {
		t.Fatal("resolving an unknown request must fail")
	}
}

func TestParseInstallLine(t *testing.T) {
	stage, percent, ok := parse_install_line(" downloading | 42 ")
	if !ok || stage != "downloading" || percent != 42 {
		t.Fatalf("got %q %d %v", stage, percent, ok)
	}
	for _, bad := range []string{"", "downloading", "|10", "stage|many"} {
		if _, _, ok := parse_install_line(bad); ok {
			t.Fatalf("%q must not parse", bad)
		}
	}
}

func TestMediaDownloadResolveRefusesWithoutMobileEngine(t *testing.T) {
	s := &MediaDownloadService{}
	if err := s.ResolveMediaDownloadHost("id", "", ""); !errors.Is(err, errMediaHostUnavailable) {
		t.Fatalf("resolve err = %v", err)
	}
	if err := s.ReportMediaDownloadLine("id", "x"); !errors.Is(err, errMediaHostUnavailable) {
		t.Fatalf("report err = %v", err)
	}
}

var _ mediadl.MobileEngine = media_host_engine{}
