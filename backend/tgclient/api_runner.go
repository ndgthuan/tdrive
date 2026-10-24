package tgclient

import (
	"context"
	"errors"

	"github.com/gotd/td/tg"
)

// APIRunner runs raw Telegram API calls on the shared connection. Domains that
// need RPCs outside the narrow Client surface (for example shared-drive role
// management) reach the live *tg.Client through it instead of widening Client.
// The concrete *Gotd implements it; fakes that do not need it need not.
type APIRunner interface {
	RunAPI(ctx context.Context, fn func(ctx context.Context, api *tg.Client) error) error
}

// RunAPI satisfies APIRunner on top of the shared, lazily connected client.
func (g *Gotd) RunAPI(ctx context.Context, fn func(ctx context.Context, api *tg.Client) error) error {
	return g.run(ctx, fn)
}

// StaticAPIRunner runs calls on one raw *tg.Client the caller already owns,
// such as a gotd client in an integration test. It does no connecting or
// reconnecting of its own.
type StaticAPIRunner struct {
	API *tg.Client
}

// RunAPI satisfies APIRunner by calling fn with the wrapped client.
func (r StaticAPIRunner) RunAPI(ctx context.Context, fn func(ctx context.Context, api *tg.Client) error) error {
	if r.API == nil {
		return errNilAPIClient
	}
	return fn(ctx, r.API)
}

var errNilAPIClient = errors.New("tgclient: static API runner has no client")
