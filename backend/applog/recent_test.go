package applog

import (
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func TestRecentBufferKeepsNewestLinesInOrder(t *testing.T) {
	buffer := newRecentBuffer(3)
	for _, line := range []string{"one", "two", "three", "four", "five"} {
		buffer.add(line)
	}
	got := strings.Join(buffer.last(0), ",")
	if got != "three,four,five" {
		t.Fatalf("lines = %q, want the newest three in order", got)
	}
	if got := strings.Join(buffer.last(2), ","); got != "four,five" {
		t.Fatalf("last(2) = %q, want the newest two", got)
	}
}

func TestRecentBufferBeforeFullReturnsEverything(t *testing.T) {
	buffer := newRecentBuffer(5)
	buffer.add("a")
	buffer.add("b")
	if got := strings.Join(buffer.last(10), ","); got != "a,b" {
		t.Fatalf("lines = %q, want a,b", got)
	}
}

func TestRecentBufferConcurrentAddsStayBounded(t *testing.T) {
	buffer := newRecentBuffer(50)
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range 200 {
				buffer.add(strings.Repeat("x", worker+index%3))
			}
		}()
	}
	wg.Wait()
	if lines := buffer.last(0); len(lines) != 50 {
		t.Fatalf("kept %d lines, want the capacity of 50", len(lines))
	}
}

func TestRecentHandlerRecordsAndRedactsAttributes(t *testing.T) {
	buffer := newRecentBuffer(10)
	logger := slog.New(newRecentHandler(slog.NewTextHandler(io.Discard, nil), buffer))

	logger.Info("tgclient: dialog page",
		"rpc", "messages.getDialogs",
		"offset_date", 1730000000,
		"access_hash", "9182736450192837465",
		"phone", "+84901234567",
		"api_hash", "0123456789abcdef",
		slog.Group("walk", "folder_id", 1, "page", 2),
	)
	logger.With("walk", "owned").Warn("personaldrive: dialog walk stopped", "error_class", "FLOOD_WAIT")

	lines := buffer.last(0)
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	first := lines[0]
	for _, want := range []string{"INFO", "tgclient: dialog page", "rpc=messages.getDialogs", "offset_date=1730000000", "walk.folder_id=1", "walk.page=2", "access_hash=[redacted]", "phone=[redacted]", "api_hash=[redacted]"} {
		if !strings.Contains(first, want) {
			t.Errorf("line %q is missing %q", first, want)
		}
	}
	for _, secret := range []string{"9182736450192837465", "84901234567", "0123456789abcdef"} {
		if strings.Contains(lines[0]+lines[1], secret) {
			t.Errorf("buffer holds %q, which must be redacted", secret)
		}
	}
	if !strings.Contains(lines[1], "WARN") || !strings.Contains(lines[1], "walk=owned") {
		t.Errorf("second line %q lost its level or the With attribute", lines[1])
	}
}

func TestScrubTextMasksSecretsAndPhoneNumbers(t *testing.T) {
	cases := map[string]string{
		"lock_token=abc123 done":          "lock_token=[redacted] done",
		"Authorization: Bearer eyJhbGci.x": "Authorization: Bearer [redacted]",
		"call +84901234567 failed":        "call [number] failed",
		"dialog 1730000000 page 2":        "dialog 1730000000 page 2",
	}
	for input, want := range cases {
		if got := scrubText(input); got != want {
			t.Errorf("scrubText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTruncateLineDoesNotSplitRunes(t *testing.T) {
	line := strings.Repeat("a", maxRecentLineBytes-1) + "é"
	got := truncateLine(line)
	if !strings.HasSuffix(got, "...") || len(got) > maxRecentLineBytes+3 {
		t.Fatalf("truncated length = %d, want at most %d plus an ellipsis", len(got), maxRecentLineBytes)
	}
	if strings.ContainsRune(got, '�') {
		t.Fatalf("truncation left a broken rune: %q", got[len(got)-6:])
	}
}

func TestRecentLogReportsUnavailableUntilInstalled(t *testing.T) {
	previous := recentReady.Load()
	recentReady.Store(false)
	t.Cleanup(func() { recentReady.Store(previous) })

	if _, err := RecentLog(10); err == nil {
		t.Fatal("RecentLog succeeded before the buffer was installed")
	}
	recentReady.Store(true)
	if _, err := RecentLog(10); err != nil {
		t.Fatalf("RecentLog after install: %v", err)
	}
}
