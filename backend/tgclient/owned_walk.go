package tgclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// ownedFolderCount is the number of dialog folders an owned-channel walk
// covers: the main folder (0) and the archive (1).
const ownedFolderCount = 2

// OwnedChannelCursor is where the next page of an owned-channel walk starts.
// The zero value starts a walk at the first page of the main folder. A page
// that fails returns the cursor it should resume from, so the caller can
// continue the walk later instead of paging from the beginning.
type OwnedChannelCursor struct {
	// Folder is the dialog folder being walked: 0 is the main list, 1 the archive.
	Folder int
	// Pages counts the page requests this walk has made so far.
	Pages int
	// FloodWaited is the FLOOD_WAIT time this attempt has already slept through.
	FloodWaited time.Duration
	// StartedAt is when the walk first started, for logs.
	StartedAt time.Time
	// OffsetDate, OffsetID and OffsetPeer are the messages.getDialogs paging
	// offsets of the next page. A nil OffsetPeer means the first page.
	OffsetDate int
	OffsetID   int
	OffsetPeer tg.InputPeerClass
}

// OwnedChannelPage is one page of an owned-channel walk. Next is the cursor
// for the following page; Done means the walk has covered every folder.
type OwnedChannelPage struct {
	Channels []OwnedBroadcastChannel
	Next     OwnedChannelCursor
	Done     bool
}

func (c OwnedChannelCursor) offsetPeer() tg.InputPeerClass {
	if c.OffsetPeer == nil {
		return &tg.InputPeerEmpty{}
	}
	return c.OffsetPeer
}

// dialogWalk is one attempt at a dialog walk. It holds the FLOOD_WAIT budget
// shared by the attempt's requests (the joined walk runs folders concurrently,
// so it is locked) and the counters the walk logs report.
type dialogWalk struct {
	caller  string
	started time.Time

	mu    sync.Mutex
	spent time.Duration
	pages int
}

func newDialogWalk(caller string, started time.Time) *dialogWalk {
	if started.IsZero() {
		started = time.Now()
	}
	return &dialogWalk{caller: caller, started: started}
}

// reserve books pause against the walk budget and reports whether it fit.
func (w *dialogWalk) reserve(pause time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.spent+pause > dialogFloodWaitMaxTotal {
		return false
	}
	w.spent += pause
	return true
}

func (w *dialogWalk) waited() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.spent
}

// nextPage numbers a page request of the walk, starting at 1.
func (w *dialogWalk) nextPage() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pages++
	return w.pages
}

func (w *dialogWalk) pageCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pages
}

// pageTrace records the FLOOD_WAIT pauses one request sleeps through, so the
// request's log line can report them. It travels in the request context.
type pageTrace struct {
	waited     time.Duration
	floodWaits int
}

type pageTraceKey struct{}

// floodWaitResumingQuery re-issues the same dialogs page after a FLOOD_WAIT,
// so the walk resumes where Telegram stopped it instead of restarting from the
// first page and flooding again at the same point.
type floodWaitResumingQuery struct {
	next dialogs.Query
	walk *dialogWalk
}

func (q floodWaitResumingQuery) Query(ctx context.Context, request dialogs.Request) (tg.MessagesDialogsClass, error) {
	for {
		result, err := q.next.Query(ctx, request)
		if err == nil {
			return result, nil
		}
		wait, ok := FloodWaitDuration(normalizeError(err))
		if !ok || wait > dialogFloodWaitMaxSingle {
			return nil, err
		}
		pause := wait + dialogFloodWaitPad
		if !q.walk.reserve(pause) {
			return nil, err
		}
		if trace, ok := ctx.Value(pageTraceKey{}).(*pageTrace); ok {
			trace.waited += pause
			trace.floodWaits++
		}
		slog.Warn("tgclient: FLOOD_WAIT while listing dialogs, waiting", "walk", q.walk.caller, "seconds", int(wait/time.Second))
		if sleepErr := dialog_sleep(ctx, pause); sleepErr != nil {
			return nil, sleepErr
		}
	}
}

// runDialogPage issues one messages.getDialogs request for a walk. The request
// rides the walk's FLOOD_WAIT budget and is logged once, with its outcome.
func runDialogPage(ctx context.Context, walk *dialogWalk, folder int, query dialogs.Query, request dialogs.Request) (tg.MessagesDialogsClass, error) {
	page := walk.nextPage()
	trace := &pageTrace{}
	began := time.Now()
	pageCtx := context.WithValue(ctx, pageTraceKey{}, trace)
	result, err := floodWaitResumingQuery{next: query, walk: walk}.Query(pageCtx, request)
	logDialogPage(walk, folder, page, request, trace, err, time.Since(began))
	return result, err
}

// walkPageQuery is an iterator-facing query: each request the iterator makes
// becomes one logged page of the walk.
type walkPageQuery struct {
	walk   *dialogWalk
	folder int
	next   dialogs.Query
}

func (q walkPageQuery) Query(ctx context.Context, request dialogs.Request) (tg.MessagesDialogsClass, error) {
	return runDialogPage(ctx, q.walk, q.folder, q.next, request)
}

// resumeOnFloodWait wraps the folder queries of an iterator-driven walk so that
// they share one FLOOD_WAIT budget and log every page. The queries are given in
// folder order, so the index is the folder id.
func resumeOnFloodWait(walk *dialogWalk, queries []dialogs.Query) []dialogs.Query {
	wrapped := make([]dialogs.Query, len(queries))
	for index, query := range queries {
		wrapped[index] = walkPageQuery{walk: walk, folder: index, next: query}
	}
	return wrapped
}

func logDialogPage(walk *dialogWalk, folder, page int, request dialogs.Request, trace *pageTrace, err error, took time.Duration) {
	attrs := []any{
		"rpc", "messages.getDialogs",
		"walk", walk.caller,
		"folder_id", folder,
		"page", page,
		"offset_date", request.OffsetDate,
		"offset_id", request.OffsetID,
		"limit", request.Limit,
		"elapsed", time.Since(walk.started).Round(time.Millisecond),
		"took", took.Round(time.Millisecond),
	}
	if trace.floodWaits > 0 {
		attrs = append(attrs, "flood_waits", trace.floodWaits, "waited", trace.waited.Round(time.Second))
	}
	if err == nil {
		attrs = append(attrs, "result", "ok")
	} else if wait, ok := FloodWaitDuration(normalizeError(err)); ok {
		attrs = append(attrs, "result", "flood_wait", "flood_wait_seconds", int(wait/time.Second))
	} else {
		attrs = append(attrs, "result", "error", "error_class", errorClass(err))
	}
	slog.Info("tgclient: dialog page", attrs...)
}

// logDialogWalkStart and logDialogWalkEnd bracket a whole walk. The caller label
// says which walk it was: owned, joined or source.
func logDialogWalkStart(walk *dialogWalk, folders int, resumed bool) {
	slog.Info("tgclient: dialog walk started", "walk", walk.caller, "folders", folders, "resumed", resumed)
}

func logDialogWalkEnd(walk *dialogWalk, channels int, err error) {
	attrs := []any{
		"walk", walk.caller,
		"pages", walk.pageCount(),
		"channels", channels,
		"elapsed", time.Since(walk.started).Round(time.Millisecond),
		"waited", walk.waited().Round(time.Second),
	}
	if err != nil {
		slog.Warn("tgclient: dialog walk stopped", append(attrs, "error_class", errorClass(err))...)
		return
	}
	slog.Info("tgclient: dialog walk finished", attrs...)
}

// errorClass names an error for logs without its text, which can carry more
// detail than a log line should. RPC errors report their type, such as
// FLOOD_WAIT, and other errors report their Go type.
func errorClass(err error) string {
	if rpc, ok := tgerr.As(err); ok {
		return rpc.Type
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	return fmt.Sprintf("%T", err)
}

// ownedChannelPage makes one messages.getDialogs request for an owned-channel
// walk and filters its dialogs. On failure the returned cursor is the one to
// resume from, with the budget and page count the attempt has used.
func ownedChannelPage(ctx context.Context, query dialogs.Query, cursor OwnedChannelCursor) (OwnedChannelPage, error) {
	walk := newDialogWalk("owned", cursor.StartedAt)
	walk.spent = cursor.FloodWaited
	walk.pages = cursor.Pages
	request := dialogs.Request{
		OffsetDate: cursor.OffsetDate,
		OffsetID:   cursor.OffsetID,
		OffsetPeer: cursor.offsetPeer(),
		Limit:      ownedDialogsBatchSize,
	}
	result, err := runDialogPage(ctx, walk, cursor.Folder, query, request)

	next := cursor
	next.Pages = walk.pageCount()
	next.FloodWaited = walk.waited()
	if err != nil {
		return OwnedChannelPage{Next: next}, fmt.Errorf("tgclient: list owned broadcast channels: %w", err)
	}

	var (
		dialogList []tg.DialogClass
		messages   []tg.MessageClass
		entities   peer.Entities
		final      bool
	)
	switch dlgs := result.(type) {
	case *tg.MessagesDialogs:
		dialogList, messages, entities, final = dlgs.Dialogs, dlgs.Messages, peer.EntitiesFromResult(dlgs), true
	case *tg.MessagesDialogsSlice:
		dialogList, messages, entities, final = dlgs.Dialogs, dlgs.Messages, peer.EntitiesFromResult(dlgs), len(dlgs.Dialogs) == 0
	default:
		return OwnedChannelPage{Next: next}, fmt.Errorf("tgclient: list owned broadcast channels: unexpected dialogs type %T", result)
	}

	channels := make([]OwnedBroadcastChannel, 0)
	for _, dlg := range dialogList {
		input, err := entities.ExtractPeer(dlg.GetPeer())
		if err != nil {
			continue
		}
		inputChannel, ok := input.(*tg.InputPeerChannel)
		if !ok {
			continue
		}
		channel, ok := entities.Channel(inputChannel.ChannelID)
		if !ok || !channel.Creator || !channel.Broadcast || channel.Megagroup || channel.Left {
			continue
		}
		dialog, _ := dlg.(*tg.Dialog)
		channels = append(channels, OwnedBroadcastChannel{
			ID:          channel.ID,
			AccessHash:  channel.AccessHash,
			Title:       strings.TrimSpace(channel.Title),
			CreatedAt:   int64(channel.Date),
			HasActivity: dialog != nil && dialog.TopMessage > 0,
		})
	}

	if final {
		// The folder is exhausted: the walk moves on to the next folder from its first page.
		folder := cursor.Folder + 1
		next = OwnedChannelCursor{Folder: folder, Pages: next.Pages, FloodWaited: next.FloodWaited, StartedAt: cursor.StartedAt}
		return OwnedChannelPage{Channels: channels, Next: next, Done: folder >= ownedFolderCount}, nil
	}
	if len(dialogList) > 0 {
		// The next page starts after the last dialog of this one, as gotd's iterator does.
		last := dialogList[len(dialogList)-1]
		lastPeer, err := entities.ExtractPeer(last.GetPeer())
		if err != nil {
			return OwnedChannelPage{Next: next}, fmt.Errorf("tgclient: list owned broadcast channels: offset peer: %w", err)
		}
		next.OffsetPeer = lastPeer
		// Same as gotd's dialogs iterator: without the last dialog's top message
		// the offsets keep their previous values and no error is returned. The
		// discovery walk stops a page that does not move on (dialogCursorStalled).
		if message := lastMessageOf(messages, last.GetPeer()); message != nil {
			next.OffsetID = message.GetID()
			next.OffsetDate = message.GetDate()
		}
	}
	return OwnedChannelPage{Channels: channels, Next: next}, nil
}

// lastMessageOf returns the top message of a dialog from a dialogs answer, or
// nil when the answer carries none for that peer.
func lastMessageOf(messages []tg.MessageClass, dialogPeer tg.PeerClass) tg.NotEmptyMessage {
	var want dialogs.DialogKey
	if err := want.FromPeer(dialogPeer); err != nil {
		return nil
	}
	var found tg.NotEmptyMessage
	for _, raw := range messages {
		message, ok := raw.AsNotEmpty()
		if !ok {
			continue
		}
		var key dialogs.DialogKey
		if err := key.FromPeer(message.GetPeerID()); err != nil {
			continue
		}
		if key == want {
			found = message
		}
	}
	return found
}

// ListOwnedBroadcastChannelPage makes one page request of an owned-channel walk.
// The walk is driven by the caller, which keeps the cursor between attempts.
func (g *Gotd) ListOwnedBroadcastChannelPage(ctx context.Context, cursor OwnedChannelCursor) (OwnedChannelPage, error) {
	page := OwnedChannelPage{Next: cursor}
	err := g.run(ctx, func(ctx context.Context, api *tg.Client) error {
		var err error
		page, err = ownedChannelPage(ctx, folderDialogsQuery{api: api, folderID: cursor.Folder}, cursor)
		return err
	})
	return page, err
}
