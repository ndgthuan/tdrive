package tgclient

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/gotd/td/tgtest"
	"github.com/gotd/td/tgtest/cluster"
)

// This file is the in-process Telegram used by dialogs_tgtest_test.go. It runs
// the real gotd client over loopback against gotd's tgtest mock, and answers
// messages.getDialogs from a fixed account. Flood waits come from a token
// bucket on a logical clock, which the walk's dialog_sleep seam advances, so
// no test waits in wall time.

const tgtest_dc = 2

// tgtest_page_size is the limit the app asks for on every owned-walk page.
const tgtest_page_size = ownedDialogsBatchSize

// tgtest_peer identifies a dialog peer. The zero value means "no peer".
type tgtest_peer struct {
	kind string
	id   int64
}

// tgtest_dialog is one dialog of the fixture account, with the fields the
// server needs to answer for it.
type tgtest_dialog struct {
	peer      tgtest_peer
	access    int64
	folders   []int
	pinned    bool
	top       int // top message id; 0 means the dialog has no message
	date      int // date of the top message
	created   int // channel or chat creation date
	title     string
	creator   bool
	broadcast bool
	megagroup bool
	left      bool
}

func (d tgtest_dialog) peer_class() tg.PeerClass {
	switch d.peer.kind {
	case "channel":
		return &tg.PeerChannel{ChannelID: d.peer.id}
	case "chat":
		return &tg.PeerChat{ChatID: d.peer.id}
	default:
		return &tg.PeerUser{UserID: d.peer.id}
	}
}

// tgtest_make_account builds n dialogs. With archive_mod 3, every dialog with
// i%4==3 is archived (folder 1); with archive_mod 0 everything sits in folder
// 0. When dup is set, the first owned channel is also listed in the archive,
// and the walk must report it once.
//
// Per ten dialogs the kinds are: four owned broadcast channels, a non-owner
// broadcast channel, an owned megagroup, an owned channel that has left, a
// user, a basic chat, and an owned broadcast channel with no top message.
// Dates repeat in groups of seven and top message ids repeat across peers, so
// paging depends on the peer as the tie breaker.
func tgtest_make_account(n int, archive_mod int, dup bool) []tgtest_dialog {
	out := make([]tgtest_dialog, 0, n)
	for i := 0; i < n; i++ {
		folder := 0
		if archive_mod > 0 && i%archive_mod == 3 {
			folder = 1
		}
		d := tgtest_dialog{
			folders: []int{folder},
			pinned:  i < 5 && folder == 0,
			top:     i%300 + 1,
			date:    1_700_000_000 - (i/7)*60,
			created: 1_600_000_000 + i,
		}
		switch i % 10 {
		case 0, 1, 2, 3, 9:
			d.peer = tgtest_peer{"channel", 1_000_000 + int64(i)}
			d.access = int64(i)*31 + 7
			d.creator, d.broadcast = true, true
			if i%10 == 9 {
				d.top, d.date = 0, 0
			}
		case 4:
			d.peer = tgtest_peer{"channel", 1_000_000 + int64(i)}
			d.access = int64(i)*31 + 7
			d.broadcast = true
		case 5:
			d.peer = tgtest_peer{"channel", 1_000_000 + int64(i)}
			d.access = int64(i)*31 + 7
			d.creator, d.megagroup = true, true
		case 6:
			d.peer = tgtest_peer{"channel", 1_000_000 + int64(i)}
			d.access = int64(i)*31 + 7
			d.creator, d.broadcast, d.left = true, true, true
		case 7:
			d.peer = tgtest_peer{"user", 2_000_000 + int64(i)}
			d.access = int64(i)*17 + 5
		case 8:
			d.peer = tgtest_peer{"chat", 3_000_000 + int64(i)}
		}
		d.title = fmt.Sprintf("Dialog %d", i)
		out = append(out, d)
	}
	if dup {
		for i := range out {
			if out[i].creator && out[i].broadcast && !out[i].megagroup && !out[i].left && out[i].peer.kind == "channel" {
				out[i].folders = []int{0, 1}
				break
			}
		}
	}
	return out
}

// tgtest_expected_owned is what the owned walk must report for records: the
// creator's broadcast channels that have not left, once each, sorted by id.
func tgtest_expected_owned(records []tgtest_dialog) []OwnedBroadcastChannel {
	seen := map[int64]bool{}
	var out []OwnedBroadcastChannel
	for _, d := range records {
		if d.peer.kind != "channel" || !d.creator || !d.broadcast || d.megagroup || d.left {
			continue
		}
		if seen[d.peer.id] {
			continue
		}
		seen[d.peer.id] = true
		out = append(out, OwnedBroadcastChannel{
			ID:          d.peer.id,
			AccessHash:  d.access,
			Title:       d.title,
			CreatedAt:   int64(d.created),
			HasActivity: d.top > 0,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// tgtest_call is one messages.getDialogs request as the server saw it.
type tgtest_call struct {
	folder      int
	folder_flag bool
	offset_date int
	offset_id   int
	offset_peer tgtest_peer
	limit       int
	flood       string // reply text when the request was rejected with a flood wait
	served      int    // dialogs returned
	terminal    bool   // the reply was the final page of the folder
	unknown     bool   // the offset peer is not in the folder
	mismatch    bool   // the offset date and id differ from the peer's top message
}

// cursor_key identifies a call's cursor.
func (c tgtest_call) cursor_key() string {
	return fmt.Sprintf("%d|%d|%d|%s:%d", c.folder, c.offset_date, c.offset_id, c.offset_peer.kind, c.offset_peer.id)
}

type tgtest_quota struct {
	burst  float64
	refill time.Duration // logical time it takes to earn one token
	tokens float64
	last   time.Duration
}

// tgtest_server answers messages.getDialogs for a fixed account. Position in a
// folder comes from the cursor's peer, as in Telegram. The date and id are
// checked against that peer's top message and reported, not used to place the
// page.
type tgtest_server struct {
	mu sync.Mutex

	records  []tgtest_dialog
	order    map[int][]int               // folder -> record indexes, in server order
	pos      map[int]map[tgtest_peer]int // folder -> peer -> position
	by_peer  map[tgtest_peer]int         // peer -> record index
	omit     map[tgtest_peer]bool        // peers whose top message is left out of the reply
	slice    bool                        // always answer with a slice, so the walk ends on an empty one
	now      time.Duration               // logical clock, advanced only by the sleeper
	quota    *tgtest_quota               // token bucket; nil admits everything
	script   []string                    // forced replies consumed before the bucket: "" serves, otherwise a flood error
	calls    []tgtest_call
	sleeps   []time.Duration
	on_sleep func(pause time.Duration)
}

func newTgtestServer(records []tgtest_dialog) *tgtest_server {
	s := &tgtest_server{
		records: records,
		order:   map[int][]int{},
		pos:     map[int]map[tgtest_peer]int{},
		by_peer: map[tgtest_peer]int{},
		omit:    map[tgtest_peer]bool{},
	}
	for index, d := range records {
		s.by_peer[d.peer] = index
		for _, f := range d.folders {
			s.order[f] = append(s.order[f], index)
		}
	}
	for folder, indexes := range s.order {
		sort.SliceStable(indexes, func(a, b int) bool {
			x, y := records[indexes[a]], records[indexes[b]]
			if x.pinned != y.pinned {
				return x.pinned
			}
			if x.date != y.date {
				return x.date > y.date
			}
			if x.top != y.top {
				return x.top > y.top
			}
			return x.peer.id > y.peer.id
		})
		s.pos[folder] = map[tgtest_peer]int{}
		for at, index := range indexes {
			s.pos[folder][records[index].peer] = at
		}
	}
	return s
}

func (s *tgtest_server) set_quota(burst float64, refill time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quota = &tgtest_quota{burst: burst, refill: refill, tokens: burst}
}

func (s *tgtest_server) set_script(replies ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.script = append([]string(nil), replies...)
}

// admit_locked decides whether the next request is rejected. It returns the
// flood error text when it is.
func (s *tgtest_server) admit_locked() (string, bool) {
	if len(s.script) > 0 {
		next := s.script[0]
		s.script = s.script[1:]
		return next, next != ""
	}
	q := s.quota
	if q == nil {
		return "", false
	}
	elapsed := s.now - q.last
	q.last = s.now
	q.tokens = math.Min(q.burst, q.tokens+float64(elapsed)/float64(q.refill))
	if q.tokens >= 1 {
		q.tokens--
		return "", false
	}
	need := time.Duration((1 - q.tokens) * float64(q.refill))
	seconds := int((need + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("FLOOD_WAIT_%d", seconds), true
}

func tgtest_input_peer(p tg.InputPeerClass) (tgtest_peer, bool) {
	switch v := p.(type) {
	case *tg.InputPeerChannel:
		return tgtest_peer{"channel", v.ChannelID}, true
	case *tg.InputPeerChat:
		return tgtest_peer{"chat", v.ChatID}, true
	case *tg.InputPeerUser:
		return tgtest_peer{"user", v.UserID}, true
	}
	return tgtest_peer{}, false
}

// handle answers one messages.getDialogs request.
func (s *tgtest_server) handle(server *tgtest.Server, req *tgtest.Request) error {
	var in tg.MessagesGetDialogsRequest
	if err := in.Decode(req.Buf); err != nil {
		return fmt.Errorf("decode messages.getDialogs: %w", err)
	}
	folder, flagged := in.GetFolderID()
	call := tgtest_call{
		folder:      folder,
		folder_flag: flagged,
		offset_date: in.OffsetDate,
		offset_id:   in.OffsetID,
		limit:       in.Limit,
	}
	if peer, ok := tgtest_input_peer(in.OffsetPeer); ok {
		call.offset_peer = peer
	}

	s.mu.Lock()
	if text, flood := s.admit_locked(); flood {
		call.flood = text
		s.calls = append(s.calls, call)
		s.mu.Unlock()
		return server.SendErr(req, tgerr.New(420, text))
	}
	result := s.page_locked(&call)
	s.calls = append(s.calls, call)
	s.mu.Unlock()
	return server.SendResult(req, result)
}

func (s *tgtest_server) page_locked(call *tgtest_call) tg.MessagesDialogsClass {
	order := s.order[call.folder]
	start := 0
	if call.offset_peer != (tgtest_peer{}) {
		at, ok := s.pos[call.folder][call.offset_peer]
		if !ok {
			call.unknown = true
			start = len(order)
		} else {
			start = at + 1
			if d := s.records[s.by_peer[call.offset_peer]]; d.top > 0 && (d.top != call.offset_id || d.date != call.offset_date) {
				call.mismatch = true
			}
		}
	}
	rest := order[start:]
	n := min(call.limit, len(rest))
	page := rest[:n]
	call.served = n
	terminal := len(rest) <= call.limit
	call.terminal = terminal

	var (
		dialogs  []tg.DialogClass
		messages []tg.MessageClass
		chats    []tg.ChatClass
		users    []tg.UserClass
		seen     = map[tgtest_peer]bool{}
	)
	for _, index := range page {
		d := s.records[index]
		dialogs = append(dialogs, &tg.Dialog{Peer: d.peer_class(), TopMessage: d.top, Pinned: d.pinned})
		if d.top > 0 && !s.omit[d.peer] {
			messages = append(messages, &tg.Message{ID: d.top, PeerID: d.peer_class(), Date: d.date, Message: d.title})
		}
		if seen[d.peer] {
			continue
		}
		seen[d.peer] = true
		switch d.peer.kind {
		case "channel":
			chats = append(chats, &tg.Channel{
				ID: d.peer.id, AccessHash: d.access, Title: d.title, Date: d.created,
				Photo: &tg.ChatPhotoEmpty{}, Creator: d.creator, Broadcast: d.broadcast,
				Megagroup: d.megagroup, Left: d.left,
			})
		case "chat":
			chats = append(chats, &tg.Chat{
				ID: d.peer.id, Title: d.title, Date: d.created, ParticipantsCount: 1,
				Photo: &tg.ChatPhotoEmpty{}, Version: 1,
			})
		default:
			users = append(users, &tg.User{ID: d.peer.id, AccessHash: d.access, FirstName: d.title})
		}
	}
	if terminal && !s.slice {
		return &tg.MessagesDialogs{Dialogs: dialogs, Messages: messages, Chats: chats, Users: users}
	}
	return &tg.MessagesDialogsSlice{Count: len(order), Dialogs: dialogs, Messages: messages, Chats: chats, Users: users}
}

// sleeper is the dialog_sleep replacement. It advances the logical clock by
// the pause and returns at once, unless the context is already done.
func (s *tgtest_server) sleeper() func(context.Context, time.Duration) error {
	return func(ctx context.Context, pause time.Duration) error {
		s.mu.Lock()
		hook := s.on_sleep
		s.mu.Unlock()
		if hook != nil {
			hook(pause)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		s.now += pause
		s.sleeps = append(s.sleeps, pause)
		s.mu.Unlock()
		return nil
	}
}

// snapshot returns copies of the calls seen so far and of the sleeps taken.
func (s *tgtest_server) snapshot() ([]tgtest_call, []time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]tgtest_call(nil), s.calls...), append([]time.Duration(nil), s.sleeps...)
}

// tgtest_start runs the mock Telegram and a Gotd client against it. The test
// is skipped, not failed, when the loopback listener cannot start. The
// dialog_sleep seam is replaced for the test and restored in cleanup.
func tgtest_start(t *testing.T, srv *tgtest_server) *Gotd {
	t.Helper()
	c := cluster.NewCluster(cluster.Options{})
	d := c.Dispatch(tgtest_dc, "dialogs")
	d.HandleFunc(tg.MessagesGetDialogsRequestTypeID, srv.handle)

	up_ctx, stop := context.WithCancel(context.Background())
	up_done := make(chan struct{})
	var up_err error
	go func() {
		defer close(up_done)
		up_err = c.Up(up_ctx)
	}()
	t.Cleanup(func() {
		stop()
		select {
		case <-up_done:
		case <-time.After(10 * time.Second):
			t.Error("tgtest cluster did not stop")
		}
	})

	select {
	case <-c.Ready():
	case <-up_done:
		t.Skipf("tgtest cluster cannot start in this environment: %v", up_err)
	case <-time.After(10 * time.Second):
		t.Fatal("tgtest cluster not ready after 10s")
	}

	opts := telegram.Options{
		DC:         tgtest_dc,
		DCList:     c.List(),
		Resolver:   c.Resolver(),
		PublicKeys: c.Keys(),
		NoUpdates:  true,
	}
	g := NewGotd(func() (*telegram.Client, error) {
		o := opts
		o.SessionStorage = &session.StorageMemory{}
		return telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, o), nil
	})
	t.Cleanup(g.Close)

	previous := dialog_sleep
	dialog_sleep = srv.sleeper()
	t.Cleanup(func() { dialog_sleep = previous })
	return g
}

// tgtest_check_calls asserts the request-level contract the app relies on:
// every request sets the folder flag and asks for the app's page size, and a
// flood-rejected request is followed by the same request. Each served cursor
// is served exactly once.
func tgtest_check_calls(t *testing.T, calls []tgtest_call) {
	t.Helper()
	served := map[string]int{}
	for index, call := range calls {
		if !call.folder_flag {
			t.Errorf("call %d (folder %d) did not set the folder flag", index, call.folder)
		}
		if call.limit != tgtest_page_size {
			t.Errorf("call %d asked for limit %d, want %d", index, call.limit, tgtest_page_size)
		}
		if call.unknown {
			t.Errorf("call %d carried an offset peer that is not in folder %d", index, call.folder)
		}
		if call.served > call.limit {
			t.Errorf("call %d returned %d dialogs, above its limit %d", index, call.served, call.limit)
		}
		if call.flood != "" {
			if index+1 >= len(calls) {
				t.Errorf("flood-rejected call %d was never retried", index)
				continue
			}
			if next := calls[index+1]; next.cursor_key() != call.cursor_key() {
				t.Errorf("after flood %q the next request moved from %s to %s; want the same cursor",
					call.flood, call.cursor_key(), next.cursor_key())
			}
			continue
		}
		served[call.cursor_key()]++
	}
	for key, count := range served {
		if count != 1 {
			t.Errorf("cursor %s was served %d times, want once", key, count)
		}
	}
}

// tgtest_equal_owned compares two owned lists, sorted by id.
func tgtest_equal_owned(t *testing.T, label string, got, want []OwnedBroadcastChannel) {
	t.Helper()
	sorted := append([]OwnedBroadcastChannel(nil), got...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	if len(sorted) != len(want) {
		t.Fatalf("%s: %d channels, want %d", label, len(sorted), len(want))
	}
	for i := range want {
		if sorted[i] != want[i] {
			t.Fatalf("%s: channel %d = %+v, want %+v", label, i, sorted[i], want[i])
		}
	}
}
