package tgclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// ErrRejected means Telegram answered a file part with a definite error. The
// server stored nothing for that request, so it is not an ambiguous send and a
// fresh attempt with the same bytes cannot succeed.
var ErrRejected = errors.New("tgclient: telegram rejected the request")

// uploadRejectionTypes are the RPC errors upload.saveFilePart and
// upload.saveBigFilePart answer when Telegram refuses a part outright.
var uploadRejectionTypes = []string{
	"FILE_PART_LENGTH_INVALID",
	"FILE_PARTS_INVALID",
	"FILE_PART_SIZE_INVALID",
	"FILE_PART_EMPTY",
	"FILE_PART_INVALID",
}

// IsDefiniteRejection reports whether err is a Telegram refusal that proves the
// upload did not land: either already marked ErrRejected, or an RPC answer
// from the upload-part set above.
func IsDefiniteRejection(err error) bool {
	return errors.Is(err, ErrRejected) || tgerr.Is(err, uploadRejectionTypes...)
}

// uploadPartRunner reacquires the active Telegram API for each attempt. This
// is the key difference from retrying a whole document upload: gotd retains the
// exact 512 KiB buffer, file_id, and part index while a dead liveConn restarts.
type uploadPartRunner func(context.Context, func(uploader.Client) error) error

type retryingUploadClient struct {
	policy FloodWaitRetryPolicy
	run    uploadPartRunner
}

func (c *retryingUploadClient) UploadSaveFilePart(ctx context.Context, request *tg.UploadSaveFilePartRequest) (bool, error) {
	return c.savePart(ctx, func(client uploader.Client) (bool, error) {
		return client.UploadSaveFilePart(ctx, request)
	})
}

func (c *retryingUploadClient) UploadSaveBigFilePart(ctx context.Context, request *tg.UploadSaveBigFilePartRequest) (bool, error) {
	return c.savePart(ctx, func(client uploader.Client) (bool, error) {
		return client.UploadSaveBigFilePart(ctx, request)
	})
}

// savePart sends one part attempt under the shared upload budget. The slot is
// held for the request only, never across a retry backoff.
func (c *retryingUploadClient) savePart(ctx context.Context, action func(uploader.Client) (bool, error)) (bool, error) {
	var accepted bool
	err := c.policy.Do(ctx, func() error {
		release, err := acquireUploadPartSlot(ctx)
		if err != nil {
			return err
		}
		defer release()
		return c.run(ctx, func(client uploader.Client) error {
			var err error
			accepted, err = action(client)
			if err != nil && tgerr.Is(err, uploadRejectionTypes...) {
				return fmt.Errorf("%w: upload part: %w", ErrRejected, err)
			}
			return err
		})
	})
	return accepted, err
}
