package channelsource

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"testing"

	"TDrive/backend/tgclient"
)

func thumb_test_jpeg(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSourceThumbnailIsOnlyThePreviewOfAPhotoOrVideo(t *testing.T) {
	ctx := t.Context()
	preview := thumb_test_jpeg(t)
	fake := tgclient.NewFake(testAccountID)
	fake.SeedMediaSourcePeers(tgclient.SourcePeer{Kind: tgclient.PeerGroup, ID: 80, AccessHash: 77, Title: "Files"})
	fake.SeedJoinedBroadcastChannels(tgclient.JoinedBroadcastChannel{ID: 80, AccessHash: 77, Title: "Files"})
	fake.SeedHistory(
		tgclient.HistoryMessage{PeerKind: tgclient.PeerGroup, ChannelID: 80, MsgID: 9, HasMedia: true, DocumentID: 901,
			DocumentAccessHash: 902, MediaSize: 4096, DocumentName: "clip.mp4", MimeType: "video/mp4",
			Thumbs: []tgclient.FileThumb{{Type: "m", Bytes: preview}}},
		tgclient.HistoryMessage{PeerKind: tgclient.PeerGroup, ChannelID: 80, MsgID: 10, HasMedia: true, DocumentID: 903,
			DocumentAccessHash: 904, MediaSize: 2048, DocumentName: "Telegram photo 10.jpg", MimeType: "image/jpeg",
			Thumbs: []tgclient.FileThumb{{Type: "m", Bytes: preview}}},
		tgclient.HistoryMessage{PeerKind: tgclient.PeerGroup, ChannelID: 80, MsgID: 11, Text: "words only"},
		tgclient.HistoryMessage{PeerKind: tgclient.PeerGroup, ChannelID: 80, MsgID: 12, HasMedia: true, DocumentID: 905,
			DocumentAccessHash: 906, MediaSize: 512, DocumentName: "notes.pdf", MimeType: "application/pdf",
			Thumbs: []tgclient.FileThumb{{Type: "m", Bytes: preview}}},
		tgclient.HistoryMessage{PeerKind: tgclient.PeerGroup, ChannelID: 80, MsgID: 13, HasMedia: true, DocumentID: 907,
			DocumentAccessHash: 908, MediaSize: 512, DocumentName: "clip.mp4", MimeType: "video/mp4",
			Thumbs: []tgclient.FileThumb{{Type: "m", Bytes: bytes.Repeat([]byte{1}, max_source_thumbnail_bytes+1)}}},
		tgclient.HistoryMessage{PeerKind: tgclient.PeerGroup, ChannelID: 80, MsgID: 14, HasMedia: true, DocumentID: 909,
			DocumentAccessHash: 910, MediaSize: 512, DocumentName: "clip.mp4", MimeType: "video/mp4",
			Thumbs: []tgclient.FileThumb{{Type: "m", Bytes: []byte("not a jpeg")}}},
	)
	sources, _, _ := sourceFixture(t, fake, fake)
	connected := connectTyped(t, sources, tgclient.PeerGroup, 80, "")

	got, err := sources.SourceThumbnail(ctx, "group", 80, 9, testAccountID, connected.Generation)
	if err != nil || !bytes.Equal(got, preview) {
		t.Fatalf("video preview = %d bytes, %v; want the seeded JPEG", len(got), err)
	}
	if got, err := sources.SourceThumbnail(ctx, "group", 80, 10, testAccountID, connected.Generation); err != nil || !bytes.Equal(got, preview) {
		t.Fatalf("photo preview = %d bytes, %v", len(got), err)
	}
	for _, msgID := range []int64{11, 12, 13, 14} {
		if got, err := sources.SourceThumbnail(ctx, "group", 80, msgID, testAccountID, connected.Generation); err != nil || got != nil {
			t.Fatalf("message %d preview = %d bytes, %v; want none", msgID, len(got), err)
		}
	}
	if _, err := sources.SourceThumbnail(ctx, "group", 80, 99, testAccountID, connected.Generation); err == nil {
		t.Fatal("a message Telegram does not have produced a preview")
	}
}

func TestSourceThumbnailRejectsAStaleConnection(t *testing.T) {
	ctx := t.Context()
	fake := tgclient.NewFake(testAccountID)
	fake.SeedMediaSourcePeers(tgclient.SourcePeer{Kind: tgclient.PeerGroup, ID: 80, AccessHash: 77, Title: "Files"})
	fake.SeedJoinedBroadcastChannels(tgclient.JoinedBroadcastChannel{ID: 80, AccessHash: 77, Title: "Files"})
	fake.SeedHistory(tgclient.HistoryMessage{PeerKind: tgclient.PeerGroup, ChannelID: 80, MsgID: 9, HasMedia: true, DocumentID: 901,
		DocumentAccessHash: 902, MediaSize: 4096, DocumentName: "clip.mp4", MimeType: "video/mp4",
		Thumbs: []tgclient.FileThumb{{Type: "m", Bytes: thumb_test_jpeg(t)}}})
	sources, _, _ := sourceFixture(t, fake, fake)
	connected := connectTyped(t, sources, tgclient.PeerGroup, 80, "")

	if _, err := sources.SourceThumbnail(ctx, "group", 80, 9, testAccountID, "some-older-generation"); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("stale generation err = %v, want %v", err, ErrNotConnected)
	}
	if _, err := sources.SourceThumbnail(ctx, "group", 80, 9, testAccountID+1, connected.Generation); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("other account err = %v, want %v", err, ErrNotConnected)
	}
	if _, err := sources.SourceThumbnail(ctx, "group", 80, 0, testAccountID, connected.Generation); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("zero message err = %v, want %v", err, ErrUnavailable)
	}
}
