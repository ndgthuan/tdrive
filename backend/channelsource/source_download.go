package channelsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"TDrive/backend/tgclient"
)

// ErrSourceDownloadRestricted refuses a post TDrive may not save: protected or
// restricted content, paid media, and self-destructing media.
var ErrSourceDownloadRestricted = errors.New("this post cannot be downloaded from its source")

// ErrSourceDownloadCanceled reports that no save location was chosen.
var ErrSourceDownloadCanceled = errors.New("download canceled")

// SourceDownload is the result of saving one source post to disk.
type SourceDownload struct {
	SavedPath string `json:"saved_path"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
}

// DownloadSourceMedia writes one post's file to the path choose returns. The
// bytes go to a temporary file beside the destination and are renamed into
// place only when Telegram delivered them all, so a failed or canceled
// download never leaves a partial file under the chosen name.
func (s *Service) DownloadSourceMedia(ctx context.Context, kind string, id, msgID, expectedAccountID int64, expectedGeneration string, choose func(name string) (string, error), progress func(done, total int64)) (SourceDownload, error) {
	if !validSource(kind, id) || msgID <= 0 || expectedAccountID <= 0 || expectedGeneration == "" || choose == nil {
		return SourceDownload{}, ErrUnavailable
	}
	stored, _, source, err := s.currentSource(ctx, kind, id)
	if err != nil {
		return SourceDownload{}, err
	}
	if stored.AccountID != expectedAccountID || stored.Generation != expectedGeneration {
		return SourceDownload{}, ErrNotConnected
	}
	if source.Protected {
		return SourceDownload{}, ErrSourceDownloadRestricted
	}
	peer := tgclient.InputPeer{Kind: source.Kind, ChannelID: id, AccessHash: source.AccessHash}
	var message tgclient.HistoryMessage
	err = s.telegram(ctx, func() error {
		var callErr error
		message, callErr = s.tg.GetChannelMessage(ctx, peer, msgID)
		return callErr
	})
	if err != nil {
		return SourceDownload{}, fmt.Errorf("media source: fetch media: %w", err)
	}
	item, ok := mediaItem(source, message)
	if !ok || item.Protected {
		return SourceDownload{}, ErrSourceDownloadRestricted
	}
	// Unsupported formats are still downloadable: any file Telegram sends can be
	// saved, unlike streaming, which needs a format the viewers understand.
	switch item.BlockReason {
	case "", "unsupported_format":
	default:
		return SourceDownload{}, ErrSourceDownloadRestricted
	}
	path, err := choose(item.Name)
	if err != nil {
		return SourceDownload{}, fmt.Errorf("media source: choose save location: %w", err)
	}
	if path == "" {
		return SourceDownload{}, ErrSourceDownloadCanceled
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tdrive-source-*.part")
	if err != nil {
		return SourceDownload{}, err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	err = s.telegram(ctx, func() error {
		// A flood-wait retry starts the stream again, so discard any bytes the
		// previous attempt already wrote.
		if err := tmp.Truncate(0); err != nil {
			return err
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		return s.tg.DownloadFile(ctx, peer, msgID, tmp, progress)
	})
	if err != nil {
		return SourceDownload{}, fmt.Errorf("media source: download: %w", err)
	}
	if err := s.checkPhotoScope(ctx, stored.AccountID, kind, id, expectedGeneration); err != nil {
		return SourceDownload{}, err
	}
	info, err := tmp.Stat()
	if err != nil {
		return SourceDownload{}, err
	}
	if err := tmp.Close(); err != nil {
		return SourceDownload{}, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return SourceDownload{}, err
	}
	committed = true
	return SourceDownload{SavedPath: path, Name: item.Name, Size: info.Size()}, nil
}
