package autobackup

import (
	"context"
	"errors"
	"fmt"
)

// FolderStore is the slice of the drive's folder service that EnsureFolder
// needs. The app adapts the real folder service to it; tests use a fake.
type FolderStore interface {
	// Find returns the live folder named name under parentID, if any.
	Find(ctx context.Context, parentID, name string) (id string, found bool, err error)
	// Create makes a folder named name under parentID and returns its id.
	Create(ctx context.Context, parentID, name string) (id string, err error)
}

// EnsureFolder returns the id of the folder named name under parentID,
// creating it only when it does not exist. Calling it again returns the same
// id, so enabling backup repeatedly never produces a second "iPhone" folder.
//
// Folder creation is remotely projected, so a concurrent caller may create the
// same folder between our Find and Create. When Create fails, Find runs once
// more and resolves the sibling that won; a duplicate is never created with a
// suffix.
func EnsureFolder(ctx context.Context, store FolderStore, parentID, name string) (string, error) {
	if store == nil {
		return "", errors.New("autobackup: folder store unavailable")
	}
	if name == "" {
		return "", ErrInvalidFolderName
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if id, found, err := store.Find(ctx, parentID, name); err != nil {
		return "", err
	} else if found {
		return id, nil
	}
	id, err := store.Create(ctx, parentID, name)
	if err == nil {
		return id, nil
	}
	existing, found, findErr := store.Find(ctx, parentID, name)
	if findErr != nil {
		return "", errors.Join(err, findErr)
	}
	if found {
		return existing, nil
	}
	return "", fmt.Errorf("autobackup: create folder %q: %w", name, err)
}
