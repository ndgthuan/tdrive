package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"TDrive/backend/channelsource"
	"TDrive/backend/media"
	fileservice "TDrive/backend/services/file"
)

func (s *DriveService) sourceService() (*channelsource.Service, error) {
	engine := s.engine()
	if engine == nil || engine.ChannelSourceService() == nil {
		return nil, errBackendUnavailable
	}
	return engine.ChannelSourceService(), nil
}

// ListChannelSourceCandidates includes readable primary and archived dialogs.
func (s *DriveService) ListChannelSourceCandidates() ([]channelsource.SourceInfo, error) {
	sources, err := s.sourceService()
	if err != nil {
		return nil, err
	}
	return sources.ListSourceCandidates(s.host.appContext())
}

func (s *DriveService) ResolvePublicChannelSource(input string) (channelsource.SourceInfo, error) {
	sources, err := s.sourceService()
	if err != nil {
		return channelsource.SourceInfo{}, err
	}
	return sources.ResolvePublicSource(s.host.appContext(), input)
}

func (s *DriveService) ListConnectedChannelSources() ([]channelsource.SourceInfo, error) {
	sources, err := s.sourceService()
	if err != nil {
		return nil, err
	}
	return sources.ListAllConnected(s.host.appContext())
}

func (s *DriveService) ConnectChannelSource(peerKind string, peerID int64, username string, expectedAccountID int64) (channelsource.SourceInfo, error) {
	sources, err := s.sourceService()
	if err != nil {
		return channelsource.SourceInfo{}, err
	}
	if s.connectSourceGate == nil {
		return channelsource.SourceInfo{}, errBackendUnavailable
	}
	return sources.ConnectSourceWithGate(s.host.appContext(), peerKind, peerID, username, expectedAccountID, s.connectSourceGate)
}

func (s *DriveService) DisconnectChannelSource(peerKind string, peerID, expectedAccountID int64, expectedGeneration string) error {
	sources, err := s.sourceService()
	if err != nil {
		return err
	}
	tokens, err := sources.DisconnectSourceWithGate(s.host.appContext(), peerKind, peerID, expectedAccountID, expectedGeneration, s.connectSourceGate)
	if err != nil {
		return err
	}
	if s.closeExternalMedia != nil {
		s.closeExternalMedia(tokens)
	}
	return nil
}

// ChannelSourcePhoto returns a Telegram source's small profile photo as base64,
// or "" when the peer has none.
func (s *DriveService) ChannelSourcePhoto(peerKind string, peerID, expectedAccountID int64, expectedGeneration string) (string, error) {
	sources, err := s.sourceService()
	if err != nil {
		return "", err
	}
	photo, err := sources.SourcePhoto(s.host.appContext(), peerKind, peerID, expectedAccountID, expectedGeneration)
	if err != nil || len(photo) == 0 {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(photo), nil
}

// ChannelSourceMediaThumbnail returns a photo or video post's small preview as
// base64 JPEG, or "" when Telegram offers none for it.
func (s *DriveService) ChannelSourceMediaThumbnail(peerKind string, peerID, msgID, expectedAccountID int64, expectedGeneration string) (string, error) {
	sources, err := s.sourceService()
	if err != nil {
		return "", err
	}
	thumb, err := sources.SourceThumbnail(s.host.appContext(), peerKind, peerID, msgID, expectedAccountID, expectedGeneration)
	if err != nil || len(thumb) == 0 {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(thumb), nil
}

func (s *DriveService) ListChannelMedia(peerKind string, peerID, offsetID int64, limit int, search, kind string, expectedAccountID int64, expectedGeneration string) (channelsource.MediaPage, error) {
	if expectedAccountID <= 0 || expectedGeneration == "" {
		return channelsource.MediaPage{}, channelsource.ErrInvalidPage
	}
	sources, err := s.sourceService()
	if err != nil {
		return channelsource.MediaPage{}, err
	}
	return sources.PageSourceScoped(s.host.appContext(), peerKind, peerID, offsetID, limit, search, kind, expectedAccountID, expectedGeneration)
}

// ListChannelSourceTopics pages the topic picker within one connection generation.
func (s *DriveService) ListChannelSourceTopics(peerKind string, peerID, offsetDate, offsetID, offsetTopic int64, limit int, query string, expectedAccountID int64, expectedGeneration string) (channelsource.ForumTopicPage, error) {
	sources, err := s.sourceService()
	if err != nil {
		return channelsource.ForumTopicPage{}, err
	}
	return sources.ListSourceTopics(s.host.appContext(), peerKind, peerID, offsetDate, offsetID, offsetTopic, limit, query, expectedAccountID, expectedGeneration)
}

func (s *DriveService) ListChannelMediaByTopic(peerKind string, peerID, topicID, offsetID int64, limit int, search, kind string, expectedAccountID int64, expectedGeneration string) (channelsource.MediaPage, error) {
	if expectedAccountID <= 0 || expectedGeneration == "" {
		return channelsource.MediaPage{}, channelsource.ErrInvalidPage
	}
	sources, err := s.sourceService()
	if err != nil {
		return channelsource.MediaPage{}, err
	}
	page, err := sources.PageSourceByTopicScoped(s.host.appContext(), peerKind, peerID, topicID, offsetID, limit, search, kind, expectedAccountID, expectedGeneration)
	switch {
	case errors.Is(err, channelsource.ErrTopicUnavailable):
		page.ErrorCode = "topic_unavailable"
		return page, nil
	case errors.Is(err, channelsource.ErrNotForum):
		page.ErrorCode = "forum_unavailable"
		return page, nil
	default:
		return page, err
	}
}

// OpenChannelMedia publishes an account- and source-scoped capability. The
// lifecycle gate serializes its final publication with terminal logout.
func (s *MediaService) OpenChannelMedia(peerKind string, peerID, msgID, expectedAccountID int64, expectedGeneration string) (media.OpenResult, error) {
	engine := s.engine()
	if engine == nil || engine.ChannelSourceService() == nil {
		return media.OpenResult{}, errBackendUnavailable
	}
	ctx := s.host.appContext()
	release, err := s.mount.acquireMountLifecycle(ctx)
	if err != nil {
		return media.OpenResult{}, err
	}
	release()
	return engine.ChannelSourceService().OpenSourceWithGate(ctx, peerKind, peerID, msgID, expectedAccountID, expectedGeneration, func(add func() error) error {
		release, err := s.mount.acquireMountLifecycle(ctx)
		if err != nil {
			return fmt.Errorf("channel source: logout in progress: %w", err)
		}
		defer release()
		return add()
	})
}

func (s *MediaService) closeExternalNativeMedia(tokens []string) {
	for _, token := range tokens {
		_ = s.CloseNativeMedia(token)
	}
}

// DownloadChannelSourceMedia saves one post of a connected source to a path the
// user picks. It takes the same single download slot as My Drive downloads, so
// CancelDownload stops it too, and it reports progress under the request id the
// frontend queued it with.
func (a *App) DownloadChannelSourceMedia(peerKind string, peerID, msgID, expectedAccountID int64, expectedGeneration string, requestID string) DownloadResult {
	if !validFrontendDownloadRequestID(requestID) || !validFrontendDownloadID(peerID) || !validFrontendDownloadID(msgID) || !validFrontendDownloadID(expectedAccountID) || strings.TrimSpace(expectedGeneration) == "" {
		return DownloadResult{Result: operationFailure(errors.New("invalid download request identifiers"))}
	}
	engine := a.engine
	if engine == nil || engine.ChannelSourceService() == nil {
		return DownloadResult{Result: operationFailure(errBackendUnavailable)}
	}
	downloadCtx, finish, err := a.transfers.beginDownload(a.appContext())
	if err != nil {
		return DownloadResult{Result: operationFailure(err)}
	}
	defer finish()
	ctx := fileservice.WithDownloadProgressID(downloadCtx, requestID)
	saved, err := engine.ChannelSourceService().DownloadSourceMedia(ctx, peerKind, peerID, msgID, expectedAccountID, expectedGeneration,
		a.chooseDownloadPath, a.sourceDownloadProgress(requestID))
	if err != nil {
		if errors.Is(err, channelsource.ErrSourceDownloadCanceled) || errors.Is(err, context.Canceled) {
			return DownloadResult{Result: operationFailureMessage(err, "Download canceled")}
		}
		return DownloadResult{Result: operationFailure(err)}
	}
	a.emit("download_progress", 100.0, requestID)
	return DownloadResult{Result: operationSuccess(), SavedPath: saved.SavedPath}
}

// sourceDownloadProgress reports a source download's percentage at most every
// 100 ms, matching the My Drive progress events the frontend already consumes.
func (a *App) sourceDownloadProgress(requestID string) func(done, total int64) {
	var last time.Time
	var mu sync.Mutex
	return func(done, total int64) {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(last) < 100*time.Millisecond {
			return
		}
		percent := 0.0
		if total > 0 {
			percent = min(100, float64(done)/float64(total)*100)
		}
		a.emit("download_progress", percent, requestID)
		last = time.Now()
	}
}
