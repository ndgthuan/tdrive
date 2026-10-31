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
