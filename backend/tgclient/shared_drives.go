package tgclient

import (
	"context"
	"time"

	"github.com/gotd/td/tg"
)

// SharedDriveDescription is the marker written by every TDrive shared-drive
// creator. Ordinary Telegram groups are never registered as drives by title.
const SharedDriveDescription = "TDrive shared drive"

// DiscoverSharedDrives finds joined TDrive megagroups missing from the local
// registry. The cache belongs to this authenticated client, never another account.
// Each pass inspects at most 20 unknown groups; negative results expire in ten
// minutes and subsequent passes continue with unchecked groups.
func (g *Gotd) DiscoverSharedDrives(ctx context.Context, known map[int64]bool) ([]SourcePeer, error) {
	g.sharedDriveMu.Lock()
	defer g.sharedDriveMu.Unlock()
	if time.Since(g.sharedDriveLastScan) < 30*time.Second {
		return nil, nil
	}
	g.sharedDriveLastScan = time.Now()
	peers, err := g.ListMediaSourcePeers(ctx)
	if err != nil {
		return nil, err
	}
	if g.sharedDriveChecked == nil {
		g.sharedDriveChecked = make(map[int64]time.Time)
	}
	now := time.Now()
	for id, checked := range g.sharedDriveChecked {
		if now.Sub(checked) >= 10*time.Minute {
			delete(g.sharedDriveChecked, id)
		}
	}
	var found []SourcePeer
	budget := 20
	for _, peer := range peers {
		if peer.Kind != PeerSupergroup || known[peer.ID] || !g.sharedDriveChecked[peer.ID].IsZero() {
			continue
		}
		if budget == 0 {
			break
		}
		budget--
		marked := false
		err := g.run(ctx, func(ctx context.Context, api *tg.Client) error {
			full, err := api.ChannelsGetFullChannel(ctx, &tg.InputChannel{ChannelID: peer.ID, AccessHash: peer.AccessHash})
			if err != nil {
				return err
			}
			if info, ok := full.FullChat.(*tg.ChannelFull); ok {
				marked = info.About == SharedDriveDescription
			}
			return nil
		})
		if err != nil {
			return found, err
		}
		if marked {
			found = append(found, peer)
		} else {
			g.sharedDriveChecked[peer.ID] = now
		}
	}
	return found, nil
}

func (f *Fake) DiscoverSharedDrives(_ context.Context, known map[int64]bool) ([]SourcePeer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var found []SourcePeer
	for id, channel := range f.channels {
		if !known[id] && channel.About == SharedDriveDescription {
			found = append(found, SourcePeer{Kind: PeerSupergroup, ID: id,
				AccessHash: channel.Peer.AccessHash, Title: channel.Title})
		}
	}
	return found, nil
}
