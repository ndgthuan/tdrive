package tgclient

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSharedDriveScanIsBoundedAndContinuesPastExpiredNegatives(t *testing.T) {
	g := &Gotd{}
	peers := make([]SourcePeer, 45)
	for i := range peers {
		peers[i] = SourcePeer{Kind: PeerSupergroup, ID: int64(i + 1), AccessHash: 100}
	}
	calls := 0
	lookup := func(_ context.Context, peer SourcePeer) (bool, error) {
		calls++
		return peer.ID == 45, nil
	}
	for pass := range 3 {
		// Simulate slow polling: previously checked negatives have expired.
		for id := range g.sharedDriveChecked {
			g.sharedDriveChecked[id] = time.Now().Add(-time.Hour)
		}
		calls = 0
		found, err := g.scanSharedDrives(t.Context(), peers, nil, lookup)
		if err != nil {
			t.Fatal(err)
		}
		if calls > 20 {
			t.Fatalf("pass %d made %d calls", pass, calls)
		}
		if pass == 2 && (len(found) != 1 || found[0].ID != 45) {
			t.Fatalf("later drive was starved: %+v", found)
		}
	}
}

func TestSharedDriveScanRetainsPartialResultsOnFailure(t *testing.T) {
	g := &Gotd{}
	wantErr := errors.New("network unavailable")
	peers := []SourcePeer{{Kind: PeerSupergroup, ID: 1}, {Kind: PeerSupergroup, ID: 2}}
	found, err := g.scanSharedDrives(t.Context(), peers, nil, func(_ context.Context, peer SourcePeer) (bool, error) {
		if peer.ID == 2 {
			return false, wantErr
		}
		return true, nil
	})
	if !errors.Is(err, wantErr) || len(found) != 1 || found[0].ID != 1 {
		t.Fatalf("found = %+v, error = %v", found, err)
	}
}
