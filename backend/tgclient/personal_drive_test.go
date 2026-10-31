package tgclient

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

type scriptedDialogsQuery struct {
	responses []tg.MessagesDialogsClass
	errAt     int
	err       error
	calls     []dialogs.Request
}

func (q *scriptedDialogsQuery) Query(_ context.Context, request dialogs.Request) (tg.MessagesDialogsClass, error) {
	q.calls = append(q.calls, request)
	call := len(q.calls)
	if q.err != nil && call == q.errAt {
		return nil, q.err
	}
	if call <= len(q.responses) {
		return q.responses[call-1], nil
	}
	return &tg.MessagesDialogsSlice{}, nil
}

type dialogChannel struct {
	id          int64
	title       string
	creator     bool
	broadcast   bool
	megagroup   bool
	left        bool
	topMessage  int
	createdDate int
	username    string
	protected   bool
	photoID     int64
}

func dialogsPage(rows ...dialogChannel) tg.MessagesDialogsClass {
	result := &tg.MessagesDialogsSlice{Count: len(rows)}
	for _, row := range rows {
		peer := &tg.PeerChannel{ChannelID: row.id}
		var photo tg.ChatPhotoClass = &tg.ChatPhotoEmpty{}
		if row.photoID != 0 {
			photo = &tg.ChatPhoto{PhotoID: row.photoID, DCID: 2}
		}
		result.Dialogs = append(result.Dialogs, &tg.Dialog{
			Peer:       peer,
			TopMessage: row.topMessage,
		})
		result.Chats = append(result.Chats, &tg.Channel{
			ID:         row.id,
			AccessHash: row.id + 1000,
			Title:      row.title,
			Creator:    row.creator,
			Broadcast:  row.broadcast,
			Megagroup:  row.megagroup,
			Left:       row.left,
			Date:       row.createdDate,
			Username:   row.username,
			Noforwards: row.protected,
			Photo:      photo,
		})
		if row.topMessage > 0 {
			result.Messages = append(result.Messages, &tg.Message{
				ID:     row.topMessage,
				PeerID: peer,
				Date:   row.createdDate,
			})
		}
	}
	return result
}

func TestCollectJoinedBroadcastChannelsIncludesArchivedNonOwnersAndProtection(t *testing.T) {
	main := &scriptedDialogsQuery{responses: []tg.MessagesDialogsClass{
		dialogsPage(
			dialogChannel{id: 101, title: "Joined public", broadcast: true, username: "public"},
			dialogChannel{id: 102, title: "Group", megagroup: true},
		),
		&tg.MessagesDialogsSlice{},
	}}
	archive := &scriptedDialogsQuery{responses: []tg.MessagesDialogsClass{
		dialogsPage(
			dialogChannel{id: 101, title: "Duplicate", broadcast: true},
			dialogChannel{id: 103, title: "Protected archive", broadcast: true, protected: true, photoID: 5503},
			dialogChannel{id: 104, title: "Left", broadcast: true, left: true},
		),
		&tg.MessagesDialogsSlice{},
	}}
	got, err := collectJoinedBroadcastChannels(t.Context(), main, archive)
	if err != nil {
		t.Fatal(err)
	}
	want := []JoinedBroadcastChannel{
		{ID: 101, AccessHash: 1101, Title: "Joined public", Username: "public"},
		{ID: 103, AccessHash: 1103, Title: "Protected archive", Protected: true, PhotoID: 5503},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("joined channels = %#v, want %#v", got, want)
	}
}

func TestCollectOwnedBroadcastChannelsPaginatesFiltersAndDeduplicates(t *testing.T) {
	main := &scriptedDialogsQuery{responses: []tg.MessagesDialogsClass{
		dialogsPage(
			dialogChannel{id: 1001, title: "TDrive", creator: true, broadcast: true, topMessage: 90, createdDate: 100},
			dialogChannel{id: 2001, title: "Joined", creator: false, broadcast: true, topMessage: 8, createdDate: 10},
			dialogChannel{id: 2002, title: "Group", creator: true, megagroup: true, topMessage: 8, createdDate: 10},
		),
		dialogsPage(
			dialogChannel{id: 1003, title: "Archive", creator: true, broadcast: true, topMessage: 7, createdDate: 25},
			dialogChannel{id: 2003, title: "Left", creator: true, broadcast: true, left: true, topMessage: 8, createdDate: 10},
		),
		&tg.MessagesDialogsSlice{},
	}}
	archive := &scriptedDialogsQuery{responses: []tg.MessagesDialogsClass{
		dialogsPage(
			dialogChannel{id: 1001, title: "TDrive", creator: true, broadcast: true, topMessage: 90, createdDate: 100},
			dialogChannel{id: 1002, title: "TDrive", creator: true, broadcast: true, createdDate: 50},
		),
		&tg.MessagesDialogsSlice{},
	}}

	got, err := collectOwnedBroadcastChannels(context.Background(), main, archive)
	if err != nil {
		t.Fatalf("collectOwnedBroadcastChannels: %v", err)
	}
	want := []OwnedBroadcastChannel{
		{ID: 1001, AccessHash: 2001, Title: "TDrive", CreatedAt: 100, HasActivity: true},
		{ID: 1003, AccessHash: 2003, Title: "Archive", CreatedAt: 25, HasActivity: true},
		{ID: 1002, AccessHash: 2002, Title: "TDrive", CreatedAt: 50, HasActivity: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("channels = %#v, want %#v", got, want)
	}
	if len(main.calls) != 3 || len(archive.calls) != 2 {
		t.Fatalf("query calls = main:%d archive:%d, want 3 and 2", len(main.calls), len(archive.calls))
	}
	for _, request := range append(append([]dialogs.Request{}, main.calls...), archive.calls...) {
		if request.Limit != ownedDialogsBatchSize {
			t.Fatalf("request limit = %d, want %d", request.Limit, ownedDialogsBatchSize)
		}
	}
}

func TestCollectOwnedBroadcastChannelsReturnsNoPartialResultsOnFailure(t *testing.T) {
	wantErr := errors.New("dialog lookup failed")
	query := &scriptedDialogsQuery{
		responses: []tg.MessagesDialogsClass{dialogsPage(
			dialogChannel{id: 1001, title: "TDrive", creator: true, broadcast: true, topMessage: 1},
		)},
		errAt: 2,
		err:   wantErr,
	}

	got, err := collectOwnedBroadcastChannels(context.Background(), query)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("partial channels returned after failure: %#v", got)
	}
}

func TestFakeOwnedBroadcastChannelsReturnCopies(t *testing.T) {
	fake := NewFake(7)
	fake.SeedOwnedBroadcastChannels(OwnedBroadcastChannel{ID: 1001, Title: "TDrive"})

	first, err := fake.ListOwnedBroadcastChannels(context.Background())
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	first[0].Title = "mutated"
	second, err := fake.ListOwnedBroadcastChannels(context.Background())
	if err != nil {
		t.Fatalf("second list: %v", err)
	}
	if second[0].Title != "TDrive" {
		t.Fatalf("fake returned aliased state: %#v", second)
	}
}

func TestBroadcastChannelFromAnswersOnlyForAJoinedBroadcast(t *testing.T) {
	t.Parallel()
	peer := InputPeer{ChannelID: 101, AccessHash: 1101}
	answer := func(chats ...tg.ChatClass) tg.MessagesChatsClass { return &tg.MessagesChats{Chats: chats} }
	joined, err := broadcastChannelFrom(answer(&tg.Channel{ID: 101, Broadcast: true, Title: " News ", Noforwards: true,
		Photo: &tg.ChatPhoto{PhotoID: 7}}), peer)
	if err != nil || joined != (JoinedBroadcastChannel{ID: 101, AccessHash: 1101, Title: "News", Protected: true, PhotoID: 7}) {
		t.Fatalf("joined channel = %#v, %v", joined, err)
	}
	for name, chats := range map[string]tg.MessagesChatsClass{
		"left":      answer(&tg.Channel{ID: 101, Broadcast: true, Left: true}),
		"group":     answer(&tg.Channel{ID: 101, Megagroup: true}),
		"forbidden": answer(&tg.ChannelForbidden{ID: 101, Broadcast: true}),
		"missing":   answer(),
	} {
		if _, err := broadcastChannelFrom(chats, peer); !errors.Is(err, ErrChannelUnavailable) {
			t.Errorf("%s: err = %v, want ErrChannelUnavailable", name, err)
		}
	}
}

// dialog_step is one scripted answer of steppedDialogsQuery: a page, or an error.
type dialog_step struct {
	page tg.MessagesDialogsClass
	err  error
}

// steppedDialogsQuery answers the Nth request with the Nth step and records
// every request, so a test can tell a re-issued page from a new one.
type steppedDialogsQuery struct {
	steps []dialog_step
	calls []dialogs.Request
}

func (q *steppedDialogsQuery) Query(_ context.Context, request dialogs.Request) (tg.MessagesDialogsClass, error) {
	q.calls = append(q.calls, request)
	index := len(q.calls) - 1
	if index >= len(q.steps) {
		return &tg.MessagesDialogsSlice{}, nil
	}
	return q.steps[index].page, q.steps[index].err
}

// use_recorded_dialog_sleep replaces the dialog walk sleeper with one that
// records each pause and returns at once. The returned slice grows with the
// pauses the code under test requests.
func use_recorded_dialog_sleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var pauses []time.Duration
	previous := dialog_sleep
	dialog_sleep = func(_ context.Context, pause time.Duration) error {
		pauses = append(pauses, pause)
		return nil
	}
	t.Cleanup(func() { dialog_sleep = previous })
	return &pauses
}

func TestCollectOwnedBroadcastChannelsResumesAfterFloodWaitWithoutRestarting(t *testing.T) {
	pauses := use_recorded_dialog_sleep(t)
	query := &steppedDialogsQuery{steps: []dialog_step{
		{page: dialogsPage(dialogChannel{id: 1001, title: "First", creator: true, broadcast: true, topMessage: 9, createdDate: 10})},
		{err: tgerr.New(420, "FLOOD_WAIT_23")},
		{page: dialogsPage(dialogChannel{id: 1002, title: "Second", creator: true, broadcast: true, topMessage: 8, createdDate: 20})},
		{page: &tg.MessagesDialogsSlice{}},
	}}

	got, err := collectOwnedBroadcastChannels(t.Context(), query)
	if err != nil {
		t.Fatalf("collectOwnedBroadcastChannels: %v", err)
	}
	if len(got) != 2 || got[0].ID != 1001 || got[1].ID != 1002 {
		t.Fatalf("channels = %#v, want both channels after the flood wait", got)
	}
	if len(query.calls) != 4 {
		t.Fatalf("requests = %d, want 4 (page 1, page 2 rejected, page 2 again, end)", len(query.calls))
	}
	if query.calls[0].OffsetID != 0 {
		t.Fatalf("first request offset = %d, want 0", query.calls[0].OffsetID)
	}
	if !reflect.DeepEqual(query.calls[1], query.calls[2]) {
		t.Fatalf("re-issued request differs: %#v vs %#v", query.calls[1], query.calls[2])
	}
	if want := []time.Duration{24 * time.Second}; !reflect.DeepEqual(*pauses, want) {
		t.Fatalf("pauses = %v, want %v", *pauses, want)
	}
}

func TestCollectOwnedBroadcastChannelsStopsWhenFloodWaitBudgetIsSpent(t *testing.T) {
	pauses := use_recorded_dialog_sleep(t)
	floodSteps := make([]dialog_step, 0, 6)
	for range 6 {
		floodSteps = append(floodSteps, dialog_step{err: tgerr.New(420, "FLOOD_WAIT_90")})
	}
	query := &steppedDialogsQuery{steps: floodSteps}

	got, err := collectOwnedBroadcastChannels(t.Context(), query)
	if err == nil || got != nil {
		t.Fatalf("collectOwnedBroadcastChannels = %#v, %v; want no list and an error", got, err)
	}
	if _, ok := FloodWaitDuration(normalizeError(err)); !ok {
		t.Fatalf("error %v no longer reports a flood wait to callers", err)
	}
	// Each 90s wait costs 91s with padding, so three fit in the 5 minute budget.
	if len(*pauses) != 3 {
		t.Fatalf("pauses = %v, want exactly 3 before the budget is spent", *pauses)
	}
}

func TestCollectOwnedBroadcastChannelsDoesNotSleepThroughOverlongFloodWait(t *testing.T) {
	pauses := use_recorded_dialog_sleep(t)
	query := &steppedDialogsQuery{steps: []dialog_step{
		{err: tgerr.New(420, "FLOOD_WAIT_120")},
	}}

	_, err := collectOwnedBroadcastChannels(t.Context(), query)
	if err == nil {
		t.Fatal("collectOwnedBroadcastChannels succeeded past an overlong flood wait")
	}
	if _, ok := FloodWaitDuration(normalizeError(err)); !ok {
		t.Fatalf("error %v no longer reports a flood wait to callers", err)
	}
	if len(*pauses) != 0 || len(query.calls) != 1 {
		t.Fatalf("pauses = %v, requests = %d; want no pause and one request", *pauses, len(query.calls))
	}
}

func TestCollectOwnedBroadcastChannelsReturnsContextErrorWhenCancelledDuringFloodWait(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	previous := dialog_sleep
	dialog_sleep = func(sleep_ctx context.Context, pause time.Duration) error {
		// Cancel while the walk is waiting; the real sleeper must observe it.
		cancel()
		return sleepContext(sleep_ctx, pause)
	}
	t.Cleanup(func() { dialog_sleep = previous })
	query := &steppedDialogsQuery{steps: []dialog_step{
		{err: tgerr.New(420, "FLOOD_WAIT_60")},
		{page: &tg.MessagesDialogsSlice{}},
	}}

	started := time.Now()
	_, err := collectOwnedBroadcastChannels(ctx, query)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("cancelled walk took %s, want prompt return", elapsed)
	}
	if len(query.calls) != 1 {
		t.Fatalf("requests = %d, want no re-issue after cancellation", len(query.calls))
	}
}
