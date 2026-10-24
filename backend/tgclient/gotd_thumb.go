package tgclient

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

// message_thumb_min_side is the smallest preview worth showing in a grid tile.
// Telegram's sizes below it are skipped in favour of the smallest one that
// reaches it, or the largest there is when none does.
const message_thumb_min_side = 320

// thumbChoice is the preview one message offers: inline bytes Telegram already
// sent with the message, or a size to download from its data center.
type thumbChoice struct {
	inline   []byte
	location tg.InputFileLocationClass
	dcID     int
}

func (g *Gotd) DownloadMessageThumbnail(ctx context.Context, peer InputPeer, msgID int64, w io.Writer) error {
	return g.runClient(ctx, func(ctx context.Context, client *telegram.Client) error {
		messages, err := channelMessages(ctx, client.API(), peer, []int64{msgID})
		if err != nil {
			return err
		}
		msg, ok := messages[msgID]
		if !ok {
			return ErrMessageNotFound
		}
		choice, ok := message_thumb(msg)
		if !ok {
			return nil
		}
		if len(choice.inline) > 0 {
			_, err := w.Write(choice.inline)
			return err
		}
		release, err := AcquireBackgroundGetFileSlots(ctx, 1)
		if err != nil {
			return err
		}
		defer release()
		return g.download_via_dc(ctx, client, choice.dcID, func(api *tg.Client) error {
			if _, err := downloader.NewDownloader().Download(api, choice.location).Stream(ctx, w); err != nil {
				return fmt.Errorf("tgclient: download message thumbnail: %w", err)
			}
			return nil
		})
	})
}

// message_thumb picks the preview of a photo or a video message. The file
// itself is never considered. Stripped previews are skipped: they are not a
// complete JPEG on their own.
func message_thumb(msg tg.MessageClass) (thumbChoice, bool) {
	full, ok := msg.(*tg.Message)
	if !ok {
		return thumbChoice{}, false
	}
	switch media := full.Media.(type) {
	case *tg.MessageMediaDocument:
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return thumbChoice{}, false
		}
		return pick_thumb(doc.Thumbs, doc.DCID, func(kind string) tg.InputFileLocationClass {
			return &tg.InputDocumentFileLocation{ID: doc.ID, AccessHash: doc.AccessHash, FileReference: doc.FileReference, ThumbSize: kind}
		})
	case *tg.MessageMediaPhoto:
		photo, ok := media.Photo.(*tg.Photo)
		if !ok || photo.ID <= 0 {
			return thumbChoice{}, false
		}
		return pick_thumb(photo.Sizes, photo.DCID, func(kind string) tg.InputFileLocationClass {
			return &tg.InputPhotoFileLocation{ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference, ThumbSize: kind}
		})
	default:
		return thumbChoice{}, false
	}
}

func pick_thumb(sizes []tg.PhotoSizeClass, dcID int, location func(kind string) tg.InputFileLocationClass) (thumbChoice, bool) {
	var (
		kind  string
		side  int
		found bool
	)
	for _, size := range sizes {
		var w, h int
		var candidate string
		switch value := size.(type) {
		case *tg.PhotoCachedSize:
			if n := max(value.W, value.H); n >= message_thumb_min_side && is_jpeg(value.Bytes) {
				return thumbChoice{inline: bytes.Clone(value.Bytes)}, true
			}
			continue
		case *tg.PhotoSize:
			w, h, candidate = value.W, value.H, value.Type
		case *tg.PhotoSizeProgressive:
			w, h, candidate = value.W, value.H, value.Type
		default:
			continue
		}
		n := max(w, h)
		if candidate == "" || n <= 0 {
			continue
		}
		if !found || thumb_size_better(n, side) {
			kind, side, found = candidate, n, true
		}
	}
	if !found {
		return thumbChoice{}, false
	}
	return thumbChoice{location: location(kind), dcID: dcID}, true
}

// thumb_size_better reports whether a preview of n pixels beats one of current
// pixels: the smallest that reaches the minimum wins, and below it the largest.
func thumb_size_better(n, current int) bool {
	reachable, currentReachable := n >= message_thumb_min_side, current >= message_thumb_min_side
	switch {
	case reachable != currentReachable:
		return reachable
	case reachable:
		return n < current
	default:
		return n > current
	}
}

func is_jpeg(data []byte) bool {
	return len(data) > 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
}
