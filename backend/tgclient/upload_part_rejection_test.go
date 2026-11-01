package tgclient

import (
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// partRejectingUploader answers every part with the given error.
type partRejectingUploader struct {
	err      error
	attempts int
}

func (u *partRejectingUploader) UploadSaveFilePart(context.Context, *tg.UploadSaveFilePartRequest) (bool, error) {
	u.attempts++
	return false, u.err
}

func (u *partRejectingUploader) UploadSaveBigFilePart(context.Context, *tg.UploadSaveBigFilePartRequest) (bool, error) {
	u.attempts++
	return false, u.err
}

func partClientFor(fake *partRejectingUploader) *retryingUploadClient {
	return &retryingUploadClient{
		policy: FloodWaitRetryPolicy{WaitUntilCanceled: true},
		run: func(_ context.Context, action func(uploader.Client) error) error {
			return action(fake)
		},
	}
}

func TestUploadPartLengthRejectionIsDefiniteNotAmbiguous(t *testing.T) {
	t.Parallel()
	fake := &partRejectingUploader{err: tgerr.New(400, "FILE_PART_LENGTH_INVALID")}
	_, err := partClientFor(fake).UploadSaveFilePart(t.Context(), &tg.UploadSaveFilePartRequest{})
	if !errors.Is(err, ErrRejected) || !IsDefiniteRejection(err) {
		t.Fatalf("err = %v, want definite rejection", err)
	}
	if errors.Is(err, ErrSendOutcomeUnknown) {
		t.Fatalf("err = %v, must not be marked as an ambiguous send", err)
	}
	if !tgerr.Is(err, "FILE_PART_LENGTH_INVALID") {
		t.Fatalf("err = %v, want the Telegram error preserved", err)
	}
	if fake.attempts != 1 {
		t.Fatalf("attempts = %d, want one attempt for a definite refusal", fake.attempts)
	}
}

func TestUploadPartTransportErrorIsNotDefiniteRejection(t *testing.T) {
	t.Parallel()
	fake := &partRejectingUploader{err: errors.New("connection reset by peer")}
	_, err := partClientFor(fake).UploadSaveBigFilePart(t.Context(), &tg.UploadSaveBigFilePartRequest{})
	if err == nil || IsDefiniteRejection(err) {
		t.Fatalf("err = %v, want a non-rejection error", err)
	}
}
