package tgclient

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestPickThumbPrefersTheSmallestPreviewThatReachesTheMinimum(t *testing.T) {
	sizes := []tg.PhotoSizeClass{
		&tg.PhotoStrippedSize{Type: "i", Bytes: []byte{1, 2}},
		&tg.PhotoSize{Type: "s", W: 90, H: 60, Size: 500},
		&tg.PhotoSize{Type: "m", W: 320, H: 213, Size: 9000},
		&tg.PhotoSize{Type: "x", W: 800, H: 533, Size: 40000},
		&tg.PhotoSizeProgressive{Type: "y", W: 1280, H: 853, Sizes: []int{1, 90000}},
	}
	choice, ok := pick_thumb(sizes, 4, func(kind string) tg.InputFileLocationClass {
		return &tg.InputPhotoFileLocation{ThumbSize: kind}
	})
	if !ok || choice.dcID != 4 || choice.inline != nil {
		t.Fatalf("pick_thumb = %+v, %v", choice, ok)
	}
	if got := choice.location.(*tg.InputPhotoFileLocation).ThumbSize; got != "m" {
		t.Fatalf("picked size %q, want m", got)
	}
}

func TestPickThumbFallsBackToTheLargestBelowTheMinimum(t *testing.T) {
	sizes := []tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "s", W: 90, H: 60},
		&tg.PhotoSize{Type: "m", W: 160, H: 100},
	}
	choice, ok := pick_thumb(sizes, 2, func(kind string) tg.InputFileLocationClass {
		return &tg.InputDocumentFileLocation{ThumbSize: kind}
	})
	if !ok || choice.location.(*tg.InputDocumentFileLocation).ThumbSize != "m" {
		t.Fatalf("pick_thumb = %+v, %v; want size m", choice, ok)
	}
}

func TestPickThumbUsesInlineJPEGAndIgnoresStrippedOnly(t *testing.T) {
	jpegBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0}
	if choice, ok := pick_thumb([]tg.PhotoSizeClass{&tg.PhotoCachedSize{Type: "i", W: 320, H: 200, Bytes: jpegBytes}}, 1,
		func(string) tg.InputFileLocationClass { return nil }); !ok || string(choice.inline) != string(jpegBytes) {
		t.Fatalf("inline preview = %+v, %v", choice, ok)
	}
	if _, ok := pick_thumb([]tg.PhotoSizeClass{&tg.PhotoStrippedSize{Type: "i", Bytes: []byte{1, 2, 3}}}, 1,
		func(string) tg.InputFileLocationClass { return nil }); ok {
		t.Fatal("a stripped preview alone was picked")
	}
}

func TestMessageThumbOfAPlainTextMessageIsNone(t *testing.T) {
	if _, ok := message_thumb(&tg.Message{ID: 3, Message: "text"}); ok {
		t.Fatal("text message offered a preview")
	}
}
