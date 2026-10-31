package personaldrive

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"TDrive/backend/tgclient"
)

const (
	// ownedListCacheTTL bounds how long a complete owned-channel list is reused
	// without asking Telegram again.
	ownedListCacheTTL = 5 * time.Minute
	// ownedWalkResumeTTL bounds how long an interrupted walk keeps its progress.
	// The clock runs from the last save, so a walk that keeps failing keeps its place.
	ownedWalkResumeTTL = 10 * time.Minute

	// ownedWalkKey coalesces concurrent owned-channel walks into one.
	ownedWalkKey = "owned"
)

// discoveryState is what channel discovery keeps between calls. It holds one
// account at a time: an interrupted walk keeps its cursor and the channels it
// has found so far, and a finished walk keeps its complete list. A call for a
// different account, or Forget after logout, drops everything.
type discoveryState struct {
	account  int64
	partial  *partialOwnedWalk
	complete *completeOwnedList
}

// partialOwnedWalk is an owned-channel walk that failed part way. Its channels
// are never returned to callers: an incomplete picker could imply that the
// user's drive is absent. They are kept only so a resumed walk can finish.
type partialOwnedWalk struct {
	cursor   tgclient.OwnedChannelCursor
	channels []tgclient.OwnedBroadcastChannel
	savedAt  time.Time
}

type completeOwnedList struct {
	channels []tgclient.OwnedBroadcastChannel
	savedAt  time.Time
}

// currentTime reads the injected clock, so tests can move time forward.
func (s *Service) currentTime() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// currentAccount returns the logged-in user id that keys discovery state. When
// the id cannot be read it returns 0, a session-scoped key that Forget resets.
func (s *Service) currentAccount(ctx context.Context) int64 {
	if s.accountID == nil {
		return 0
	}
	id, err := s.accountID(ctx)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// listOwnedChannels returns the complete owned-channel list. A fresh cached
// list is returned without any Telegram request unless refresh is set. Otherwise
// one walk runs, resuming an interrupted walk of the same account, and
// concurrent callers share that walk.
func (s *Service) listOwnedChannels(ctx context.Context, refresh bool) ([]tgclient.OwnedBroadcastChannel, error) {
	if s.telegram == nil {
		return nil, errTelegramUnavailable
	}
	account := s.currentAccount(ctx)
	s.discoveryMu.Lock()
	s.adoptAccountLocked(account)
	if refresh {
		s.discovery.complete = nil
	} else if cached, ok := s.cachedOwnedLocked(); ok {
		s.discoveryMu.Unlock()
		slog.Info("personaldrive: owned channel list served from cache", "candidate_count", len(cached))
		return cached, nil
	}
	s.discoveryMu.Unlock()

	result, err, _ := s.ownedWalks.Do(ownedWalkKey, func() (any, error) {
		return s.walkOwnedChannels(ctx, account)
	})
	if err != nil {
		return nil, err
	}
	return copyOwned(result.([]tgclient.OwnedBroadcastChannel)), nil
}

// walkOwnedChannels pages through every owned-channel folder. A failed page
// stores the cursor it stopped at, so the next attempt continues from there.
func (s *Service) walkOwnedChannels(ctx context.Context, account int64) ([]tgclient.OwnedBroadcastChannel, error) {
	cursor, channels, resumed := s.resumePoint(account)
	if cursor.StartedAt.IsZero() {
		cursor.StartedAt = s.currentTime()
	}
	// Each attempt gets its own FLOOD_WAIT budget. The user's Retry is the pause between attempts.
	cursor.FloodWaited = 0
	attemptStart := s.currentTime()
	slog.Info("personaldrive: owned dialog walk started", "resumed", resumed, "folder_id", cursor.Folder,
		"pages_done", cursor.Pages, "channels_so_far", len(channels))

	for {
		page, err := s.telegram.ListOwnedBroadcastChannelPage(ctx, cursor)
		if err != nil {
			s.saveOwnedProgress(account, page.Next, channels)
			slog.Warn("personaldrive: owned dialog walk interrupted", "pages", page.Next.Pages,
				"channels_so_far", len(channels), "waited", page.Next.FloodWaited,
				"attempt_elapsed", s.currentTime().Sub(attemptStart),
				"walk_elapsed", s.currentTime().Sub(cursor.StartedAt), "error", err)
			return nil, fmt.Errorf("personaldrive: list owned channels: %w", err)
		}
		channels = mergeOwned(channels, page.Channels)
		cursor = page.Next
		if page.Done {
			break
		}
	}
	s.saveComplete(account, channels)
	slog.Info("personaldrive: owned dialog walk finished", "pages", cursor.Pages, "channels", len(channels),
		"waited", cursor.FloodWaited, "attempt_elapsed", s.currentTime().Sub(attemptStart),
		"walk_elapsed", s.currentTime().Sub(cursor.StartedAt))
	return channels, nil
}

// adoptAccountLocked clears the state when it belongs to another account, so
// progress or a cached list never crosses an account switch.
func (s *Service) adoptAccountLocked(account int64) {
	if s.discovery.account == account {
		return
	}
	s.discovery = discoveryState{account: account}
}

// cachedOwnedLocked returns a copy of the complete list while it is fresh.
func (s *Service) cachedOwnedLocked() ([]tgclient.OwnedBroadcastChannel, bool) {
	complete := s.discovery.complete
	if complete == nil {
		return nil, false
	}
	if s.currentTime().Sub(complete.savedAt) > ownedListCacheTTL {
		s.discovery.complete = nil
		return nil, false
	}
	return copyOwned(complete.channels), true
}

// resumePoint returns where an interrupted walk of this account should continue,
// or an empty cursor when there is no usable progress.
func (s *Service) resumePoint(account int64) (tgclient.OwnedChannelCursor, []tgclient.OwnedBroadcastChannel, bool) {
	s.discoveryMu.Lock()
	defer s.discoveryMu.Unlock()
	partial := s.discovery.partial
	if partial == nil {
		return tgclient.OwnedChannelCursor{}, nil, false
	}
	if s.discovery.account != account || s.currentTime().Sub(partial.savedAt) > ownedWalkResumeTTL {
		s.discovery.partial = nil
		return tgclient.OwnedChannelCursor{}, nil, false
	}
	return partial.cursor, copyOwned(partial.channels), true
}

func (s *Service) saveOwnedProgress(account int64, cursor tgclient.OwnedChannelCursor, channels []tgclient.OwnedBroadcastChannel) {
	s.discoveryMu.Lock()
	defer s.discoveryMu.Unlock()
	if s.discovery.account != account {
		return
	}
	s.discovery.partial = &partialOwnedWalk{cursor: cursor, channels: copyOwned(channels), savedAt: s.currentTime()}
}

func (s *Service) saveComplete(account int64, channels []tgclient.OwnedBroadcastChannel) {
	s.discoveryMu.Lock()
	defer s.discoveryMu.Unlock()
	if s.discovery.account != account {
		return
	}
	s.discovery.partial = nil
	s.discovery.complete = &completeOwnedList{channels: copyOwned(channels), savedAt: s.currentTime()}
}

// invalidateOwnedList drops the complete list and any partial walk. It runs
// after this account's channel set changes, so a later read walks Telegram again.
func (s *Service) invalidateOwnedList() {
	s.discoveryMu.Lock()
	defer s.discoveryMu.Unlock()
	s.discovery.complete = nil
	s.discovery.partial = nil
}

// Forget drops all discovery state. Logout calls it so that the next account
// starts from an empty walk and no cached list.
func (s *Service) Forget() {
	s.discoveryMu.Lock()
	defer s.discoveryMu.Unlock()
	s.discovery = discoveryState{}
}

// mergeOwned appends channels not already present, keyed by channel id.
func mergeOwned(existing, incoming []tgclient.OwnedBroadcastChannel) []tgclient.OwnedBroadcastChannel {
	seen := make(map[int64]struct{}, len(existing)+len(incoming))
	for _, channel := range existing {
		seen[channel.ID] = struct{}{}
	}
	for _, channel := range incoming {
		if _, duplicate := seen[channel.ID]; duplicate {
			continue
		}
		seen[channel.ID] = struct{}{}
		existing = append(existing, channel)
	}
	return existing
}

// copyOwned returns a new slice, so callers cannot change cached state.
func copyOwned(channels []tgclient.OwnedBroadcastChannel) []tgclient.OwnedBroadcastChannel {
	return append(make([]tgclient.OwnedBroadcastChannel, 0, len(channels)), channels...)
}
