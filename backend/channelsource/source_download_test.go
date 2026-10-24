package channelsource

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"TDrive/backend/tgclient"
)

func TestSourceDownloadWritesTheChosenFileAtomically(t *testing.T) {
	ctx := t.Context()
	fake := tgclient.NewFake(testAccountID)
	fake.SeedMediaSourcePeers(tgclient.SourcePeer{Kind: tgclient.PeerGroup, ID: 80, AccessHash: 77, Title: "Files"})
	fake.SeedJoinedBroadcastChannels(tgclient.JoinedBroadcastChannel{ID: 80, AccessHash: 77, Title: "Files"})
	body := []byte("source document body")
	seedTypedVideo(fake, tgclient.PeerGroup, 80, 9, 901, body)
	sources, _, _ := sourceFixture(t, fake, fake)
	connected := connectTyped(t, sources, tgclient.PeerGroup, 80, "")

	dir := t.TempDir()
	want := filepath.Join(dir, "clip.mp4")
	got, err := sources.DownloadSourceMedia(ctx, "group", 80, 9, testAccountID, connected.Generation,
		func(name string) (string, error) {
			if name != "clip.mp4" {
				t.Errorf("default name = %q, want clip.mp4", name)
			}
			return want, nil
		}, nil)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if got.SavedPath != want || got.Size != int64(len(body)) {
		t.Fatalf("result = %#v", got)
	}
	data, err := os.ReadFile(want)
	if err != nil || !bytes.Equal(data, body) {
		t.Fatalf("saved bytes = %q, %v; want %q", data, err, body)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries, want only the saved file", len(entries))
	}
}

func TestSourceDownloadCancelAndStaleConnectionWriteNothing(t *testing.T) {
	ctx := t.Context()
	fake := tgclient.NewFake(testAccountID)
	fake.SeedMediaSourcePeers(tgclient.SourcePeer{Kind: tgclient.PeerGroup, ID: 80, AccessHash: 77, Title: "Files"})
	fake.SeedJoinedBroadcastChannels(tgclient.JoinedBroadcastChannel{ID: 80, AccessHash: 77, Title: "Files"})
	seedTypedVideo(fake, tgclient.PeerGroup, 80, 9, 901, []byte("body"))
	fake.SeedHistory(tgclient.HistoryMessage{PeerKind: tgclient.PeerGroup, ChannelID: 80, MsgID: 10, HasMedia: true,
		DocumentID: 903, DocumentAccessHash: 904, MediaSize: 4, DocumentName: "locked.mp4", MimeType: "video/mp4", Restricted: true})
	sources, _, _ := sourceFixture(t, fake, fake)
	connected := connectTyped(t, sources, tgclient.PeerGroup, 80, "")
	dir := t.TempDir()
	choose := func(string) (string, error) { return filepath.Join(dir, "clip.mp4"), nil }

	if _, err := sources.DownloadSourceMedia(ctx, "group", 80, 9, testAccountID, connected.Generation,
		func(string) (string, error) { return "", nil }, nil); !errors.Is(err, ErrSourceDownloadCanceled) {
		t.Fatalf("empty path err = %v, want %v", err, ErrSourceDownloadCanceled)
	}
	if _, err := sources.DownloadSourceMedia(ctx, "group", 80, 9, testAccountID, "older-generation", choose, nil); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("stale generation err = %v, want %v", err, ErrNotConnected)
	}
	if _, err := sources.DownloadSourceMedia(ctx, "group", 80, 10, testAccountID, connected.Generation, choose, nil); !errors.Is(err, ErrSourceDownloadRestricted) {
		t.Fatalf("restricted post err = %v, want %v", err, ErrSourceDownloadRestricted)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("refused downloads left %d files behind", len(entries))
	}
}
