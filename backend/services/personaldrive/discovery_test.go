package personaldrive

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"TDrive/backend/tgclient"
)

// testClock is a clock the tests move by hand, so TTLs can be crossed without sleeping.
type testClock struct{ now time.Time }

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time          { return c.now }
func (c *testClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func ownedChannelsN(count int) []tgclient.OwnedBroadcastChannel {
	channels := make([]tgclient.OwnedBroadcastChannel, 0, count)
	for index := range count {
		channels = append(channels, tgclient.OwnedBroadcastChannel{
			ID:         int64(1001 + index),
			AccessHash: int64(2001 + index),
			Title:      "Channel",
			CreatedAt:  int64(10 + index),
		})
	}
	return channels
}

func newDiscoveryService(telegram *fakeTelegram, clock *testClock, account func(context.Context) (int64, error)) *Service {
	service := NewService(Config{Telegram: telegram, AccountID: account})
	service.now = clock.Now
	return service
}

func TestDiscoverResumesInterruptedWalkAtSavedOffset(t *testing.T) {
	telegram := &fakeTelegram{
		channels: ownedChannelsN(4),
		pageSize: 2,
		pageErrs: map[int]error{2: errors.New("list: tgclient: flood wait: 23s")},
	}
	service := newDiscoveryService(telegram, newTestClock(), nil)

	if _, err := service.Discover(context.Background()); err == nil {
		t.Fatal("first discovery succeeded past a failing page")
	}
	candidates, err := service.Discover(context.Background())
	if err != nil {
		t.Fatalf("resumed discovery: %v", err)
	}
	if len(candidates) != 4 {
		t.Fatalf("candidates = %d, want all 4 after resuming", len(candidates))
	}

	cursors := telegram.cursorsSeen()
	if len(cursors) != 3 {
		t.Fatalf("page requests = %d, want 3 (page 1, failed page 2, page 2 again)", len(cursors))
	}
	if cursors[1].OffsetID != 2 || cursors[2].OffsetID != 2 {
		t.Fatalf("page offsets = %d then %d, want the failed page 2 offset 2 both times", cursors[1].OffsetID, cursors[2].OffsetID)
	}
	if cursors[2].Pages != 1 {
		t.Fatalf("resumed walk counts %d finished pages, want 1 (the progress is kept)", cursors[2].Pages)
	}
}

func TestDiscoverServesCompleteListFromCacheWithinTTL(t *testing.T) {
	telegram := &fakeTelegram{channels: ownedChannelsN(2)}
	clock := newTestClock()
	service := newDiscoveryService(telegram, clock, nil)

	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("first discovery: %v", err)
	}
	clock.Advance(4 * time.Minute)
	candidates, err := service.Discover(context.Background())
	if err != nil {
		t.Fatalf("cached discovery: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("cached candidates = %d, want 2", len(candidates))
	}
	if calls := len(telegram.cursorsSeen()); calls != 1 {
		t.Fatalf("page requests = %d, want 1: the cached list must not query Telegram", calls)
	}
}

func TestDiscoverWalksAgainAfterCacheExpiresOrOnRefresh(t *testing.T) {
	telegram := &fakeTelegram{channels: ownedChannelsN(2)}
	clock := newTestClock()
	service := newDiscoveryService(telegram, clock, nil)

	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("first discovery: %v", err)
	}
	clock.Advance(6 * time.Minute)
	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("discovery after expiry: %v", err)
	}
	if calls := len(telegram.cursorsSeen()); calls != 2 {
		t.Fatalf("page requests after expiry = %d, want 2", calls)
	}

	if _, err := service.DiscoverFresh(context.Background()); err != nil {
		t.Fatalf("fresh discovery: %v", err)
	}
	if calls := len(telegram.cursorsSeen()); calls != 3 {
		t.Fatalf("page requests after refresh = %d, want 3: refresh must bypass the cache", calls)
	}
}

func TestDiscoverDropsInterruptedWalkAfterResumeTTL(t *testing.T) {
	telegram := &fakeTelegram{
		channels: ownedChannelsN(3),
		pageSize: 1,
		pageErrs: map[int]error{2: errors.New("network down")},
	}
	clock := newTestClock()
	service := newDiscoveryService(telegram, clock, nil)

	if _, err := service.Discover(context.Background()); err == nil {
		t.Fatal("first discovery succeeded past a failing page")
	}
	clock.Advance(11 * time.Minute)
	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("discovery after resume TTL: %v", err)
	}
	// Requests: page 1, failed page 2, then the new walk's first page.
	cursors := telegram.cursorsSeen()
	if len(cursors) < 3 {
		t.Fatalf("page requests = %d, want the new walk to make a request", len(cursors))
	}
	if cursors[2].OffsetID != 0 {
		t.Fatalf("walk after the TTL started at offset %d, want 0", cursors[2].OffsetID)
	}
}

func TestConcurrentDiscoverCallsShareOneWalk(t *testing.T) {
	gate := make(chan struct{})
	telegram := &fakeTelegram{channels: ownedChannelsN(3), gate: gate}
	service := newDiscoveryService(telegram, newTestClock(), nil)

	var wg sync.WaitGroup
	results := make([][]Candidate, 2)
	errs := make([]error, 2)
	for index := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[index], errs[index] = service.Discover(context.Background())
		}()
	}
	// Give the second caller time to reach the walk before the first one answers.
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	for index := range 2 {
		if errs[index] != nil || len(results[index]) != 3 {
			t.Fatalf("caller %d = %d candidates, %v; want 3 and no error", index, len(results[index]), errs[index])
		}
	}
	if calls := len(telegram.cursorsSeen()); calls != 1 {
		t.Fatalf("page requests = %d, want 1: concurrent calls must share one walk", calls)
	}
}

func TestAccountChangeAndForgetClearDiscoveryState(t *testing.T) {
	account := int64(7)
	accountID := func(context.Context) (int64, error) { return account, nil }
	telegram := &fakeTelegram{channels: ownedChannelsN(2)}
	service := newDiscoveryService(telegram, newTestClock(), accountID)

	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("discovery for account 7: %v", err)
	}
	account = 8
	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("discovery for account 8: %v", err)
	}
	if calls := len(telegram.cursorsSeen()); calls != 2 {
		t.Fatalf("page requests after account switch = %d, want 2: another account must not reuse the list", calls)
	}

	service.Forget()
	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("discovery after logout: %v", err)
	}
	if calls := len(telegram.cursorsSeen()); calls != 3 {
		t.Fatalf("page requests after logout = %d, want 3", calls)
	}
	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("second discovery after logout: %v", err)
	}
	if calls := len(telegram.cursorsSeen()); calls != 3 {
		t.Fatalf("page requests = %d, want the new account's list cached at 3", calls)
	}
}

func TestAccountChangeDropsInterruptedWalkOfOtherAccount(t *testing.T) {
	account := int64(7)
	accountID := func(context.Context) (int64, error) { return account, nil }
	telegram := &fakeTelegram{
		channels: ownedChannelsN(3),
		pageSize: 1,
		pageErrs: map[int]error{2: errors.New("network down")},
	}
	service := newDiscoveryService(telegram, newTestClock(), accountID)

	if _, err := service.Discover(context.Background()); err == nil {
		t.Fatal("first discovery succeeded past a failing page")
	}
	account = 8
	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("discovery for account 8: %v", err)
	}
	// Requests: account 7's page 1, its failed page 2, then account 8's first page.
	cursors := telegram.cursorsSeen()
	if len(cursors) < 3 {
		t.Fatalf("page requests = %d, want account 8 to make a request", len(cursors))
	}
	if cursors[2].OffsetID != 0 {
		t.Fatalf("account 8 resumed account 7's walk at offset %d, want 0", cursors[2].OffsetID)
	}
}

func TestSelectUsesCachedCandidateWithoutDialogRequests(t *testing.T) {
	isolate_data_dir(t)
	telegram := &fakeTelegram{channels: ownedChannelsN(2)}
	var saved, active int64
	service := NewService(Config{
		DB:         newServiceDB(t),
		Telegram:   telegram,
		Sync:       &fakeAuthoritativeSync{},
		LoadConfig: func() (int64, error) { return 0, nil },
		SaveConfig: func(id int64) error { saved = id; return nil },
		SetActive:  func(id int64) { active = id },
	})
	service.now = newTestClock().Now

	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("discovery: %v", err)
	}
	before := len(telegram.cursorsSeen())
	if err := service.Select(context.Background(), 1002); err != nil {
		t.Fatalf("Select cached candidate: %v", err)
	}
	if after := len(telegram.cursorsSeen()); after != before {
		t.Fatalf("Select made %d dialog requests, want zero when the cached list is fresh", after-before)
	}
	if saved != 1002 || active != 1002 {
		t.Fatalf("Select saved=%d active=%d, want 1002", saved, active)
	}
}

func TestCreateInvalidatesCachedListAndWalksAgain(t *testing.T) {
	isolate_data_dir(t)
	telegram := &fakeTelegram{
		channels:     ownedChannelsN(1),
		createResult: tgclient.OwnedBroadcastChannel{ID: 9001, AccessHash: 9002, Title: "TDrive", CreatedAt: 99},
	}
	service := NewService(Config{
		DB:         newServiceDB(t),
		Telegram:   telegram,
		Sync:       &fakeAuthoritativeSync{},
		LoadConfig: func() (int64, error) { return 0, nil },
		SaveConfig: func(int64) error { return nil },
		SetActive:  func(int64) {},
	})
	service.now = newTestClock().Now

	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if err := service.Create(context.Background()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	before := len(telegram.cursorsSeen())
	if _, err := service.Discover(context.Background()); err != nil {
		t.Fatalf("discovery after create: %v", err)
	}
	if after := len(telegram.cursorsSeen()); after == before {
		t.Fatal("discovery after a created channel was served from the stale cache")
	}
}

func TestCachedListIsACopy(t *testing.T) {
	telegram := &fakeTelegram{channels: ownedChannelsN(2)}
	service := newDiscoveryService(telegram, newTestClock(), nil)

	first, err := service.listOwnedChannels(context.Background(), false)
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	first[0].Title = "mutated"
	second, err := service.listOwnedChannels(context.Background(), false)
	if err != nil {
		t.Fatalf("second list: %v", err)
	}
	if want := ownedChannelsN(2); !reflect.DeepEqual(second, want) {
		t.Fatalf("cached list = %#v, want %#v", second, want)
	}
}
