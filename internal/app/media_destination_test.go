package app

import (
	"context"
	"testing"

	"TDrive/backend/core"
)

// destination_test_host is a serviceHost with no engine, so the destination
// calls must refuse instead of touching a drive.
type destination_test_host struct {
	active int64
}

func (h destination_test_host) coreEngine() *core.Engine                  { return nil }
func (h destination_test_host) WaitForBackendReady(context.Context) error { return nil }
func (h destination_test_host) activeChannelID() int64                    { return h.active }
func (h destination_test_host) appContext() context.Context               { return context.Background() }
func (h destination_test_host) emit(string, ...any)                       {}

func TestDestinationCallsRefuseWithoutBackend(t *testing.T) {
	svc := &MediaDownloadService{host: destination_test_host{active: 9}}
	ctx := context.Background()
	if _, err := svc.ListDestinationFolders(ctx, 9, ""); err == nil {
		t.Fatal("listing destinations without a backend must fail")
	}
	if _, err := svc.CreateDestinationFolder(ctx, 9, "", "Clips"); err == nil {
		t.Fatal("creating a destination folder without a backend must fail")
	}
	if err := (media_destination{host: svc.host}).CheckTarget(ctx, 9, ""); err == nil {
		t.Fatal("a destination check without a backend must fail")
	}
}
