package tgclient

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

// Integration tests for dialog discovery under FLOOD_WAIT. Each test runs the
// real gotd client against gotd's tgtest mock Telegram (see
// dialogs_tgtest_helpers_test.go). They do not run in parallel: they swap the
// package-level dialog_sleep seam.

// tgtest_flood_count counts the flood-rejected calls in a snapshot.
func tgtest_flood_count(calls []tgtest_call) int {
	n := 0
	for _, call := range calls {
		if call.flood != "" {
			n++
		}
	}
	return n
}

func TestTgtestSingleRPCFloodWaitNormalizesToFloodWaitDuration(t *testing.T) {
	cases := []struct {
		reply string
		want  time.Duration
	}{
		{"FLOOD_WAIT_23", 23 * time.Second},
		{"FLOOD_PREMIUM_WAIT_40", 40 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.reply, func(t *testing.T) {
			srv := newTgtestServer(tgtest_make_account(20, 0, false))
			srv.set_script(tc.reply)
			g := tgtest_start(t, srv)

			err := g.run(t.Context(), func(ctx context.Context, api *tg.Client) error {
				_, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{Limit: 10, OffsetPeer: &tg.InputPeerEmpty{}})
				return err
			})
			got, ok := FloodWaitDuration(err)
			if !ok || got != tc.want {
				t.Fatalf("FloodWaitDuration(%v) = %s, %v; want %s, true", err, got, ok, tc.want)
			}
		})
	}
}

func TestTgtestFullWalkSurvivesMidWalkFloodWithoutRestarting(t *testing.T) {
	records := tgtest_make_account(3300, 4, true)
	srv := newTgtestServer(records)
	// Eight pages are free, then one token earns back every five logical seconds.
	srv.set_quota(8, 5*time.Second)
	g := tgtest_start(t, srv)

	got, err := g.ListOwnedBroadcastChannels(t.Context())
	if err != nil {
		t.Fatalf("ListOwnedBroadcastChannels: %v", err)
	}
	tgtest_equal_owned(t, "owned walk", got, tgtest_expected_owned(records))

	calls, sleeps := srv.snapshot()
	floods := tgtest_flood_count(calls)
	if floods == 0 {
		t.Fatal("the quota never flooded the walk; the test does not exercise resume")
	}
	if len(sleeps) != floods {
		t.Fatalf("sleeps = %d, flood replies = %d; want one pause per flood", len(sleeps), floods)
	}
	var total time.Duration
	for _, pause := range sleeps {
		total += pause
	}
	if total > dialogFloodWaitMaxTotal {
		t.Fatalf("total pause %s exceeds the %s walk budget", total, dialogFloodWaitMaxTotal)
	}
	tgtest_check_calls(t, calls)
	if first := calls[0]; first.offset_date != 0 || first.offset_id != 0 || first.offset_peer != (tgtest_peer{}) {
		t.Fatalf("first request cursor = %+v, want the empty cursor", first)
	}
	if mismatches := countMismatch(calls); mismatches != 0 {
		t.Fatalf("%d requests carried a cursor that differs from the peer's top message", mismatches)
	}
	t.Logf("walk: %d requests, %d flood waits, %s logical wait", len(calls), floods, total)
}

func TestTgtestBudgetExhaustionReturnsErrorAndNoPartialList(t *testing.T) {
	records := tgtest_make_account(3300, 4, true)
	srv := newTgtestServer(records)
	// Each page after the first costs 60 logical seconds of wait plus one second of padding.
	srv.set_quota(1, 60*time.Second)
	g := tgtest_start(t, srv)

	got, err := g.ListOwnedBroadcastChannels(t.Context())
	if err == nil || got != nil {
		t.Fatalf("ListOwnedBroadcastChannels = %d channels, %v; want no list and an error", len(got), err)
	}
	wait, ok := FloodWaitDuration(err)
	if !ok || wait != 60*time.Second {
		t.Fatalf("FloodWaitDuration(%v) = %s, %v; want 60s, true", err, wait, ok)
	}
	_, sleeps := srv.snapshot()
	// Four pauses of 61s fit the 5 minute budget. The fifth would need 305s.
	if len(sleeps) != 4 {
		t.Fatalf("pauses = %v, want exactly 4 before the budget is spent", sleeps)
	}
}

func TestTgtestOverlongFloodWaitReturnsImmediately(t *testing.T) {
	for _, seconds := range []int{91, 120, 300} {
		t.Run("FLOOD_WAIT_"+strconv.Itoa(seconds), func(t *testing.T) {
			records := tgtest_make_account(3300, 4, true)
			srv := newTgtestServer(records)
			reply := "FLOOD_WAIT_" + strconv.Itoa(seconds)
			srv.set_script(reply)
			g := tgtest_start(t, srv)

			got, err := g.ListOwnedBroadcastChannels(t.Context())
			if err == nil || got != nil {
				t.Fatalf("ListOwnedBroadcastChannels = %d channels, %v; want an error and no list", len(got), err)
			}
			wait, ok := FloodWaitDuration(err)
			if !ok || wait != time.Duration(seconds)*time.Second {
				t.Fatalf("FloodWaitDuration(%v) = %s, %v; want %ds, true", err, wait, ok, seconds)
			}
			calls, sleeps := srv.snapshot()
			if len(sleeps) != 0 || len(calls) != 1 {
				t.Fatalf("sleeps = %v, requests = %d; want no pause and one request", sleeps, len(calls))
			}
		})
	}
}

func TestTgtestCancellationDuringLogicalWaitStopsTheWalk(t *testing.T) {
	srv := newTgtestServer(tgtest_make_account(3300, 4, true))
	srv.set_script("FLOOD_WAIT_60")
	g := tgtest_start(t, srv)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	srv.mu.Lock()
	srv.on_sleep = func(time.Duration) { cancel() }
	srv.mu.Unlock()

	_, err := g.ListOwnedBroadcastChannels(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	calls, sleeps := srv.snapshot()
	if len(calls) != 1 || len(sleeps) != 0 {
		t.Fatalf("requests = %d, sleeps = %v; want the flooded request only", len(calls), sleeps)
	}
}

func TestTgtestRealTimerCancellationDuringFloodWait(t *testing.T) {
	srv := newTgtestServer(tgtest_make_account(3300, 4, true))
	// The first reply serves the warm-up call; the walk's first page is flooded.
	srv.set_script("", "FLOOD_WAIT_60")
	g := tgtest_start(t, srv)
	// Connect first: the lazy handshake can outlast the timer below.
	err := g.run(t.Context(), func(ctx context.Context, api *tg.Client) error {
		_, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{Limit: 1, OffsetPeer: &tg.InputPeerEmpty{}})
		return err
	})
	if err != nil {
		t.Fatalf("warm-up call: %v", err)
	}
	// The one test that waits in wall time: the real sleeper must observe cancellation.
	dialog_sleep = sleepContext

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()

	started := time.Now()
	_, err = g.ListOwnedBroadcastChannels(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("cancelled walk took %s, want prompt return well inside the 61s pause", elapsed)
	}
	calls, _ := srv.snapshot()
	if len(calls) != 2 {
		t.Fatalf("requests = %d, want the warm-up and the one flooded walk page", len(calls))
	}
}

// tgtest_drain_cursor walks with the cursor API. A failed page is retried from
// the cursor it returned, as the discovery service does. It returns the
// channels in page order and the number of calls it made.
func tgtest_drain_cursor(t *testing.T, g *Gotd, cursor OwnedChannelCursor) ([]OwnedBroadcastChannel, int) {
	t.Helper()
	var all []OwnedBroadcastChannel
	calls := 0
	for {
		page, err := g.ListOwnedBroadcastChannelPage(t.Context(), cursor)
		calls++
		if err != nil {
			wait, ok := FloodWaitDuration(err)
			if !ok {
				t.Fatalf("page %d failed with a non-flood error: %v", calls, err)
			}
			// The failed page must hand back the cursor it was asked from.
			if page.Next.Folder != cursor.Folder || page.Next.OffsetDate != cursor.OffsetDate ||
				page.Next.OffsetID != cursor.OffsetID || !reflect.DeepEqual(page.Next.OffsetPeer, cursor.OffsetPeer) {
				t.Fatalf("failed page (wait %s) returned cursor %+v, want the same position as %+v", wait, page.Next, cursor)
			}
			cursor = page.Next
			continue
		}
		all = append(all, page.Channels...)
		cursor = page.Next
		if page.Done {
			return all, calls
		}
	}
}

func TestTgtestCursorAPIResumesFromSavedCursorAfterFlood(t *testing.T) {
	records := tgtest_make_account(3300, 4, true)
	srv := newTgtestServer(records)
	// Page one is served, page two is flooded beyond the 90 second cap, then
	// the caller retries from the saved cursor.
	srv.set_script("", "FLOOD_WAIT_120")
	g := tgtest_start(t, srv)

	first, err := g.ListOwnedBroadcastChannelPage(t.Context(), OwnedChannelCursor{})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	second, err := g.ListOwnedBroadcastChannelPage(t.Context(), first.Next)
	wait, ok := FloodWaitDuration(err)
	if !ok || wait != 120*time.Second {
		t.Fatalf("second page error = %v (%s, %v); want a 120s flood", err, wait, ok)
	}
	if second.Next.Folder != first.Next.Folder || second.Next.OffsetDate != first.Next.OffsetDate ||
		second.Next.OffsetID != first.Next.OffsetID || !reflect.DeepEqual(second.Next.OffsetPeer, first.Next.OffsetPeer) {
		t.Fatalf("flooded page returned cursor %+v, want the saved cursor %+v", second.Next, first.Next)
	}

	// The caller retries from the saved cursor and walks to the end.
	rest, _ := tgtest_drain_cursor(t, g, second.Next)
	got := append(append([]OwnedBroadcastChannel(nil), first.Channels...), rest...)
	tgtest_equal_owned(t, "cursor union", dedupeOwned(got), tgtest_expected_owned(records))

	calls, _ := srv.snapshot()
	tgtest_check_calls(t, calls)
}

func TestTgtestCursorWalkMatchesOneShotWalkForTerminalVariants(t *testing.T) {
	records := tgtest_make_account(3300, 4, true)
	for _, slice := range []bool{false, true} {
		name := "terminal messages.dialogs"
		if slice {
			name = "terminal empty messages.dialogsSlice"
		}
		t.Run(name, func(t *testing.T) {
			srv := newTgtestServer(records)
			srv.slice = slice
			srv.set_quota(8, 5*time.Second)
			g := tgtest_start(t, srv)

			drained, _ := tgtest_drain_cursor(t, g, OwnedChannelCursor{})
			tgtest_equal_owned(t, "cursor walk", dedupeOwned(drained), tgtest_expected_owned(records))
			// Checked before the one-shot walk, which asks for the same cursors again.
			calls, _ := srv.snapshot()
			tgtest_check_calls(t, calls)

			oneshot, err := g.ListOwnedBroadcastChannels(t.Context())
			if err != nil {
				t.Fatalf("one-shot walk: %v", err)
			}
			tgtest_equal_owned(t, "one-shot walk", oneshot, tgtest_expected_owned(records))
		})
	}
}

func TestTgtestPageBoundaryAndEmptyFoldersEndCleanly(t *testing.T) {
	// Exactly one page of folder 0 and an empty archive.
	records := tgtest_make_account(tgtest_page_size, 0, false)
	want := tgtest_expected_owned(records)
	cases := []struct {
		slice     bool
		wantCalls int
	}{
		// Non-slice: folder 0 ends on its only page, the archive on an empty messages.dialogs.
		{slice: false, wantCalls: 2},
		// Slice: folder 0 needs an empty slice to end, the archive ends on one.
		{slice: true, wantCalls: 3},
	}
	for _, tc := range cases {
		t.Run(map[bool]string{false: "non-slice", true: "slice"}[tc.slice], func(t *testing.T) {
			srv := newTgtestServer(records)
			srv.slice = tc.slice
			g := tgtest_start(t, srv)

			got, err := g.ListOwnedBroadcastChannels(t.Context())
			if err != nil {
				t.Fatalf("ListOwnedBroadcastChannels: %v", err)
			}
			tgtest_equal_owned(t, "boundary walk", got, want)
			calls, _ := srv.snapshot()
			if len(calls) != tc.wantCalls {
				t.Fatalf("requests = %d, want %d", len(calls), tc.wantCalls)
			}
			tgtest_check_calls(t, calls)
		})
	}
}

func TestTgtestTopMessageMissingFromReplyStillCompletesTheWalk(t *testing.T) {
	records := tgtest_make_account(3300, 4, true)
	srv := newTgtestServer(records)
	// Leave the top message out of the reply for every owned channel that has one.
	for _, d := range records {
		if d.peer.kind == "channel" && d.creator && d.top > 0 {
			srv.omit[d.peer] = true
		}
	}
	g := tgtest_start(t, srv)

	got, err := g.ListOwnedBroadcastChannels(t.Context())
	if err != nil {
		t.Fatalf("ListOwnedBroadcastChannels: %v", err)
	}
	// Activity comes from the dialog's top message id, not the message list.
	tgtest_equal_owned(t, "walk without messages", got, tgtest_expected_owned(records))

	calls, _ := srv.snapshot()
	// gotd's iterator keeps the previous offsets when the last top message is
	// missing. Paging here follows the cursor peer, so the walk still ends.
	t.Logf("requests with stale offsets (gotd parity): %d of %d", countMismatch(calls), len(calls))
}

// countMismatch counts requests whose offset date or id differs from the peer's top message.
func countMismatch(calls []tgtest_call) int {
	n := 0
	for _, call := range calls {
		if call.mismatch {
			n++
		}
	}
	return n
}

// dedupeOwned keeps the first occurrence of each channel id, as the one-shot walk does.
func dedupeOwned(channels []OwnedBroadcastChannel) []OwnedBroadcastChannel {
	seen := map[int64]bool{}
	out := make([]OwnedBroadcastChannel, 0, len(channels))
	for _, channel := range channels {
		if seen[channel.ID] {
			continue
		}
		seen[channel.ID] = true
		out = append(out, channel)
	}
	return out
}
