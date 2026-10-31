package tgclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestOwnedChannelPageReturnsCursorAfterLastDialog(t *testing.T) {
	query := &steppedDialogsQuery{steps: []dialog_step{
		{page: dialogsPage(
			dialogChannel{id: 1001, title: "First", creator: true, broadcast: true, topMessage: 9, createdDate: 10},
			dialogChannel{id: 1002, title: "Second", creator: true, broadcast: true, topMessage: 8, createdDate: 20},
		)},
	}}

	page, err := ownedChannelPage(context.Background(), query, OwnedChannelCursor{})
	if err != nil {
		t.Fatalf("ownedChannelPage: %v", err)
	}
	if len(page.Channels) != 2 || page.Done {
		t.Fatalf("page = %d channels, done=%v; want 2 channels and not done", len(page.Channels), page.Done)
	}
	// The next page starts after the last dialog: its top message id, date and peer.
	if page.Next.OffsetID != 8 || page.Next.OffsetDate != 20 {
		t.Fatalf("next offset = id %d date %d, want the last dialog's top message (8, 20)", page.Next.OffsetID, page.Next.OffsetDate)
	}
	peer, ok := page.Next.OffsetPeer.(*tg.InputPeerChannel)
	if !ok || peer.ChannelID != 1002 {
		t.Fatalf("next offset peer = %#v, want channel 1002", page.Next.OffsetPeer)
	}
	if page.Next.Pages != 1 || page.Next.Folder != 0 {
		t.Fatalf("next cursor = folder %d pages %d, want folder 0 and 1 page", page.Next.Folder, page.Next.Pages)
	}
}

func TestOwnedChannelPageFolderEndMovesToArchiveFromFirstPage(t *testing.T) {
	main := &steppedDialogsQuery{steps: []dialog_step{{page: &tg.MessagesDialogsSlice{}}}}

	page, err := ownedChannelPage(context.Background(), main, OwnedChannelCursor{})
	if err != nil {
		t.Fatalf("ownedChannelPage: %v", err)
	}
	if page.Done || page.Next.Folder != 1 || page.Next.OffsetID != 0 || page.Next.OffsetPeer != nil {
		t.Fatalf("after the main folder: done=%v cursor=%+v, want the archive from its first page", page.Done, page.Next)
	}
}

func TestOwnedChannelPageFloodWaitCarriesBudgetIntoNextCursor(t *testing.T) {
	pauses := use_recorded_dialog_sleep(t)
	query := &steppedDialogsQuery{steps: []dialog_step{
		{err: tgerr.New(420, "FLOOD_WAIT_23")},
		{page: &tg.MessagesDialogsSlice{}},
	}}

	page, err := ownedChannelPage(context.Background(), query, OwnedChannelCursor{})
	if err != nil {
		t.Fatalf("ownedChannelPage: %v", err)
	}
	if len(*pauses) != 1 {
		t.Fatalf("pauses = %v, want one flood-wait pause", *pauses)
	}
	if page.Next.FloodWaited != 24*time.Second {
		t.Fatalf("flood waited = %s, want 24s carried into the cursor", page.Next.FloodWaited)
	}
}

func TestOwnedChannelPageFailureReturnsResumableCursor(t *testing.T) {
	use_recorded_dialog_sleep(t)
	wantErr := errors.New("connection reset")
	query := &steppedDialogsQuery{steps: []dialog_step{{err: wantErr}}}
	start := OwnedChannelCursor{Folder: 0, Pages: 3, OffsetID: 42, OffsetDate: 7, OffsetPeer: &tg.InputPeerChannel{ChannelID: 5, AccessHash: 6}}

	page, err := ownedChannelPage(context.Background(), query, start)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(page.Channels) != 0 || page.Done {
		t.Fatalf("failed page returned channels=%d done=%v", len(page.Channels), page.Done)
	}
	// Pages counts requests made, so the failed request is counted; the offsets stay as they were.
	if page.Next.OffsetID != 42 || page.Next.OffsetDate != 7 || page.Next.Pages != 4 {
		t.Fatalf("resume cursor = %+v, want the saved offsets and 4 requests made", page.Next)
	}
	if query.calls[0].OffsetID != 42 || query.calls[0].Limit != ownedDialogsBatchSize {
		t.Fatalf("request = %+v, want the saved offset and the batch size", query.calls[0])
	}
}

func TestOwnedChannelWalkResumesFromFailedPageWithSpentBudget(t *testing.T) {
	pauses := use_recorded_dialog_sleep(t)
	first := &steppedDialogsQuery{steps: []dialog_step{
		{page: dialogsPage(dialogChannel{id: 1001, title: "First", creator: true, broadcast: true, topMessage: 9, createdDate: 10})},
	}}
	page, err := ownedChannelPage(context.Background(), first, OwnedChannelCursor{})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}

	// Page 2 floods beyond the single-wait limit, so the attempt stops and keeps its cursor.
	second := &steppedDialogsQuery{steps: []dialog_step{{err: tgerr.New(420, "FLOOD_WAIT_120")}}}
	stopped, err := ownedChannelPage(context.Background(), second, page.Next)
	if err == nil {
		t.Fatal("second page succeeded past an overlong flood wait")
	}
	// The stopped cursor keeps the offset of the page that failed: page 2 starts after dialog 9.
	if stopped.Next.OffsetID != 9 || stopped.Next.Pages != 2 {
		t.Fatalf("stopped cursor = %+v, want page 2's offset with 2 requests made", stopped.Next)
	}

	// The resumed attempt asks for the same page, and the walk is complete.
	third := &steppedDialogsQuery{steps: []dialog_step{{page: &tg.MessagesDialogsSlice{}}}}
	resumed, err := ownedChannelPage(context.Background(), third, stopped.Next)
	if err != nil {
		t.Fatalf("resumed page: %v", err)
	}
	if third.calls[0] != second.calls[0] {
		t.Fatalf("resumed request %+v differs from the stopped one %+v", third.calls[0], second.calls[0])
	}
	if len(*pauses) != 0 || resumed.Next.Folder != 1 {
		t.Fatalf("pauses = %v, folder = %d; want no pause and the archive next", *pauses, resumed.Next.Folder)
	}
}

func TestLastMessageOfMatchesOnlyTheDialogPeer(t *testing.T) {
	messages := []tg.MessageClass{
		&tg.Message{ID: 3, PeerID: &tg.PeerChannel{ChannelID: 1}, Date: 100},
		&tg.Message{ID: 9, PeerID: &tg.PeerChannel{ChannelID: 2}, Date: 200},
	}
	got := lastMessageOf(messages, &tg.PeerChannel{ChannelID: 2})
	if got == nil || got.GetID() != 9 {
		t.Fatalf("lastMessageOf = %v, want message 9 of channel 2", got)
	}
	if lastMessageOf(messages, &tg.PeerChannel{ChannelID: 4}) != nil {
		t.Fatal("lastMessageOf matched a dialog with no message in the answer")
	}
}

func TestOwnedChannelPageMissingTopMessageUsesEarlierDialogOffset(t *testing.T) {
	answer := dialogsPage(
		dialogChannel{id: 1001, title: "First", creator: true, broadcast: true, topMessage: 9, createdDate: 10},
		dialogChannel{id: 1002, title: "Second", creator: true, broadcast: true, topMessage: 8, createdDate: 20},
	).(*tg.MessagesDialogsSlice)
	// The answer carries the top message of the first dialog only.
	answer.Messages = answer.Messages[:1]
	query := &steppedDialogsQuery{steps: []dialog_step{{page: answer}}}
	start := OwnedChannelCursor{OffsetID: 42, OffsetDate: 7, OffsetPeer: &tg.InputPeerChannel{ChannelID: 5, AccessHash: 6}}

	page, err := ownedChannelPage(context.Background(), query, start)
	if err != nil {
		t.Fatalf("ownedChannelPage: %v", err)
	}
	// The offset is taken from the last dialog that has a message: the first one, all of it.
	if page.Next.OffsetID != 9 || page.Next.OffsetDate != 10 {
		t.Fatalf("offsets = id %d date %d, want the first dialog's top message (9, 10)", page.Next.OffsetID, page.Next.OffsetDate)
	}
	peer, ok := page.Next.OffsetPeer.(*tg.InputPeerChannel)
	if !ok || peer.ChannelID != 1001 {
		t.Fatalf("offset peer = %#v, want channel 1001", page.Next.OffsetPeer)
	}
}

func TestOwnedChannelPageOffsetSkipsTrailingEmptyDialog(t *testing.T) {
	query := &steppedDialogsQuery{steps: []dialog_step{{page: dialogsPage(
		dialogChannel{id: 1001, title: "First", creator: true, broadcast: true, topMessage: 9, createdDate: 10},
		dialogChannel{id: 1002, title: "Empty", creator: true, broadcast: true, topMessage: 0},
	)}}}

	page, err := ownedChannelPage(context.Background(), query, OwnedChannelCursor{})
	if err != nil {
		t.Fatalf("ownedChannelPage: %v", err)
	}
	if page.Done || page.Next.Folder != 0 || len(page.Channels) != 2 {
		t.Fatalf("page done=%v folder=%d channels=%d; want the main folder to continue with 2 channels", page.Done, page.Next.Folder, len(page.Channels))
	}
	// The next page must start after the dialog with a message, not at offsets 0/0.
	if page.Next.OffsetID != 9 || page.Next.OffsetDate != 10 {
		t.Fatalf("offsets = id %d date %d, want (9, 10) from the first dialog", page.Next.OffsetID, page.Next.OffsetDate)
	}
	peer, ok := page.Next.OffsetPeer.(*tg.InputPeerChannel)
	if !ok || peer.ChannelID != 1001 {
		t.Fatalf("offset peer = %#v, want channel 1001", page.Next.OffsetPeer)
	}
	// The empty channel is still listed, without activity.
	if empty := page.Channels[1]; empty.ID != 1002 || empty.HasActivity {
		t.Fatalf("empty channel = %+v, want channel 1002 without activity", empty)
	}
}

func TestOwnedChannelPageWithoutAnyMessageEndsFolder(t *testing.T) {
	query := &steppedDialogsQuery{steps: []dialog_step{{page: dialogsPage(
		dialogChannel{id: 1001, title: "First", creator: true, broadcast: true, topMessage: 0},
		dialogChannel{id: 1002, title: "Second", creator: true, broadcast: true, topMessage: 0},
	)}}}

	page, err := ownedChannelPage(context.Background(), query, OwnedChannelCursor{})
	if err != nil {
		t.Fatalf("ownedChannelPage: %v", err)
	}
	// No dialog has a date to page by, so the main folder is treated as exhausted.
	if page.Done || page.Next.Folder != 1 || page.Next.OffsetID != 0 || page.Next.OffsetDate != 0 || page.Next.OffsetPeer != nil {
		t.Fatalf("after an all-empty page: done=%v cursor=%+v, want the archive from its first page", page.Done, page.Next)
	}
	// The channels on the page are still returned.
	if len(page.Channels) != 2 || page.Channels[0].ID != 1001 || page.Channels[1].ID != 1002 {
		t.Fatalf("channels = %+v, want both owned channels of the page", page.Channels)
	}
}

// tgtest_tail_account is folder 0 of an account of owned channels: messaged
// channels first, by date, then empty channels, which have no date and sort
// last. Each empty channel has no top message.
func tgtest_tail_account(messaged, empty int) []tgtest_dialog {
	var out []tgtest_dialog
	for i := 0; i < messaged+empty; i++ {
		d := tgtest_dialog{
			peer:      tgtest_peer{"channel", 1_000_000 + int64(i)},
			access:    int64(i)*31 + 7,
			folders:   []int{0},
			created:   1_600_000_000 + i,
			title:     "Owned",
			creator:   true,
			broadcast: true,
		}
		if i < messaged {
			d.top = i + 1
			d.date = 1_700_000_000 - i*60
		}
		out = append(out, d)
	}
	return out
}

// tgtest_empty_boundary_account puts an empty owned channel last on page one:
// ninety-nine channels have messages, two are empty, and the page size of 100
// ends on one of the empty channels.
func tgtest_empty_boundary_account() []tgtest_dialog {
	return tgtest_tail_account(99, 2)
}

func TestTgtestEmptyChannelAtPageBoundaryCompletesWalk(t *testing.T) {
	records := tgtest_empty_boundary_account()
	srv := newTgtestServer(records)
	g := tgtest_start(t, srv)

	// The walk is driven page by page with a bound, so a stalled walk fails here
	// instead of looping.
	var got []OwnedBroadcastChannel
	cursor := OwnedChannelCursor{}
	done := false
	for step := 0; step < 10 && !done; step++ {
		page, err := g.ListOwnedBroadcastChannelPage(t.Context(), cursor)
		if err != nil {
			t.Fatalf("page %d: %v", step+1, err)
		}
		got = append(got, page.Channels...)
		cursor = page.Next
		done = page.Done
	}
	if !done {
		t.Fatal("walk did not finish within 10 pages: paging stalled at the empty channel")
	}
	tgtest_equal_owned(t, "boundary walk", dedupeOwned(got), tgtest_expected_owned(records))

	calls, _ := srv.snapshot()
	if len(calls) != 3 {
		t.Fatalf("requests = %d, want main page one, main page two and the empty archive", len(calls))
	}
	// Page two starts after the last dialog that has a message (the 99th), not after the empty one.
	if second := calls[1]; second.offset_peer != (tgtest_peer{"channel", 1_000_098}) || second.offset_id != 99 {
		t.Fatalf("second request offset = %+v, want the 99th channel's top message", second)
	}
	tgtest_check_calls(t, calls)
	if mismatches := countMismatch(calls); mismatches != 0 {
		t.Fatalf("%d requests carried an offset that differs from its peer's top message", mismatches)
	}
}

// TestTgtestTrailingEmptyBatchEndsFolderWithLimit documents a known limit of
// the all-empty rule. A folder whose trailing dialogs fill a whole batch with
// no messages ends at that page: the walk completes without error and without
// stalling, the channels of the message-bearing pages and of that page are
// kept, and the empty channels after it are not listed. Telegram's order of
// message-less dialogs is not verified against the real service.
func TestTgtestTrailingEmptyBatchEndsFolderWithLimit(t *testing.T) {
	// 150 messaged channels, then 120 empty ones. Page two ends in empties, so
	// its offset is the 150th channel. Page three is 100 empties and ends the folder.
	records := tgtest_tail_account(150, 120)
	srv := newTgtestServer(records)
	g := tgtest_start(t, srv)

	got, err := g.ListOwnedBroadcastChannels(t.Context())
	if err != nil {
		t.Fatalf("ListOwnedBroadcastChannels: %v (the limit must end the walk, not fail it)", err)
	}
	// Empty channels tie on date and top, so the mock lists them by descending id:
	// the walk keeps the 150 messaged channels and the 100 highest-id empty ones,
	// and drops the 20 lowest-id empty ones after the batch.
	var want []OwnedBroadcastChannel
	for _, channel := range tgtest_expected_owned(records) {
		if channel.ID < 1_000_150 || channel.ID >= 1_000_170 {
			want = append(want, channel)
		}
	}
	if len(want) != 250 {
		t.Fatalf("expected set = %d channels, want 250", len(want))
	}
	tgtest_equal_owned(t, "walk with trailing empty batch", got, want)

	calls, _ := srv.snapshot()
	// Main folder: three pages, the third all empty. Archive: one empty page.
	if len(calls) != 4 {
		t.Fatalf("requests = %d, want 4 (three main pages, one archive page)", len(calls))
	}
	tgtest_check_calls(t, calls)
}
