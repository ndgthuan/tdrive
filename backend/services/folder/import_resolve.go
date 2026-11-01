package folder

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"TDrive/backend/projection"
	"TDrive/backend/services/servicecontext"
)

// ErrImportConflict reports that an import path names a file where it needs a
// folder. Nothing is aliased or renamed: the user has to resolve it.
var ErrImportConflict = errors.New("folder: import name is taken by a file")

// ErrImportAmbiguous reports two source names that the drive treats as the
// same name, so one folder cannot honestly stand for both.
var ErrImportAmbiguous = errors.New("folder: import names collide on the drive")

// ImportEntry is one file an import is about to upload, described by where it
// goes. FolderPath is "/"-separated and relative to the import's parent folder;
// its first segment is the picked folder.
type ImportEntry struct {
	FolderPath string `json:"folder_path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Encrypted  bool   `json:"encrypted"`
}

// ImportMatch names an entry that already exists live on the drive, and the
// folder it was found in.
type ImportMatch struct {
	Index    int    `json:"index"`
	FolderID string `json:"folder_id"`
}

// splitImportPath returns the non-empty segments of an import path.
func splitImportPath(path string) []string {
	var out []string
	for _, segment := range strings.Split(path, "/") {
		// CreateContext trims names, so a source name with stray spaces must
		// reuse the same folder rather than fail the canonical lookup.
		segment = strings.TrimSpace(segment)
		if segment != "" && segment != "." {
			out = append(out, segment)
		}
	}
	return out
}

// ResolveImportFolders returns the drive folder id for every path, relative to
// parentID. Each segment reuses a live folder with the same canonical name
// (NFC plus case fold, as the projection compares names) and creates one only
// when none exists. A file with that name is a conflict. Two source names that
// canonicalize alike under one parent are ambiguous and stop the resolve before
// anything is created.
func (s *Service) ResolveImportFolders(ctx context.Context, channelID int64, parentID string, paths []string) (map[string]string, error) {
	if err := servicecontext.Check(ctx, "folder: resolve import folders"); err != nil {
		return nil, err
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := checkImportSiblings(paths); err != nil {
		return nil, err
	}
	parent := normalizeParent(parentID)
	ordered := append([]string(nil), paths...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return len(splitImportPath(ordered[i])) < len(splitImportPath(ordered[j]))
	})

	// Raw prefix -> folder id, so each shared ancestor is resolved once.
	done := map[string]string{"": parent}
	ids := make(map[string]string, len(paths))
	for _, path := range ordered {
		current := parent
		prefix := ""
		for _, segment := range splitImportPath(path) {
			prefix = joinImportPath(prefix, segment)
			if id, ok := done[prefix]; ok {
				current = id
				continue
			}
			id, err := s.resolveImportSegment(ctx, channelID, current, segment)
			if err != nil {
				return nil, err
			}
			done[prefix] = id
			current = id
		}
		ids[path] = current
	}
	return ids, nil
}

// checkImportSiblings rejects source names that collide once canonicalized,
// e.g. "Camera" and "camera" in the same folder.
func checkImportSiblings(paths []string) error {
	seen := map[string]string{}
	for _, path := range paths {
		parentKey := ""
		for _, segment := range splitImportPath(path) {
			key, err := projection.CanonicalNameKey(segment)
			if err != nil {
				return err
			}
			siblingKey := parentKey + "\x00" + key
			if raw, ok := seen[siblingKey]; ok && raw != segment {
				return fmt.Errorf("%w: %q and %q", ErrImportAmbiguous, raw, segment)
			}
			seen[siblingKey] = segment
			parentKey = parentKey + "/" + key
		}
	}
	return nil
}

func joinImportPath(prefix, segment string) string {
	if prefix == "" {
		return segment
	}
	return prefix + "/" + segment
}

// resolveImportSegment finds or creates one folder directly under parent.
func (s *Service) resolveImportSegment(ctx context.Context, channelID int64, parent, name string) (string, error) {
	entry, found, err := projection.LiveDirentByName(s.DB, channelID, parent, name)
	if err != nil {
		return "", err
	}
	if found {
		if entry.ObjectKind == "folder" {
			return entry.ObjectID, nil
		}
		return "", fmt.Errorf("%w: %q", ErrImportConflict, name)
	}
	created, err := s.CreateContext(ctx, channelID, name, parent)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// FindExistingImportEntries reports which entries already exist live under
// parentID: a folder with the same canonical path, a file with the same
// canonical name, and the same size. Trashed or deleted entries are not live
// and never match. Encrypted files only match an encrypted entry whose
// plaintext size equals the stored plaintext size, and never a plain one; an
// entry with an unknown size (negative) never matches. Nothing is created.
func (s *Service) FindExistingImportEntries(ctx context.Context, channelID int64, parentID string, entries []ImportEntry) ([]ImportMatch, error) {
	if err := servicecontext.Check(ctx, "folder: find existing import entries"); err != nil {
		return nil, err
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	parent := normalizeParent(parentID)
	folderByPath := map[string]string{"": parent}
	missing := map[string]bool{}
	filesByFolder := map[string][]projection.FileSlim{}

	var matches []ImportMatch
	for index, entry := range entries {
		if entry.Size < 0 {
			continue
		}
		key, err := projection.CanonicalNameKey(entry.Name)
		if err != nil {
			continue
		}
		folderID, ok, err := s.liveImportFolder(channelID, parent, entry.FolderPath, folderByPath, missing)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		files, cached := filesByFolder[folderID]
		if !cached {
			_, listed, err := projection.ListFolderContentsContext(ctx, s.DB, channelID, folderID)
			if err != nil {
				return nil, err
			}
			files = listed
			filesByFolder[folderID] = files
		}
		for _, file := range files {
			fileKey, err := projection.CanonicalNameKey(file.Name)
			if err != nil || fileKey != key {
				continue
			}
			if importFileMatches(entry, file) {
				matches = append(matches, ImportMatch{Index: index, FolderID: folderID})
			}
			break
		}
	}
	return matches, nil
}

// liveImportFolder walks a source-relative folder path read-only. A segment
// that is missing, or that names a file, means nothing under it exists yet.
func (s *Service) liveImportFolder(channelID int64, parent, path string, known map[string]string, missing map[string]bool) (string, bool, error) {
	segments := splitImportPath(path)
	raw := ""
	current := parent
	for _, segment := range segments {
		raw = joinImportPath(raw, segment)
		if missing[raw] {
			return "", false, nil
		}
		if id, ok := known[raw]; ok {
			current = id
			continue
		}
		entry, found, err := projection.LiveDirentByName(s.DB, channelID, current, segment)
		if err != nil {
			return "", false, err
		}
		if !found || entry.ObjectKind != "folder" {
			missing[raw] = true
			return "", false, nil
		}
		known[raw] = entry.ObjectID
		current = entry.ObjectID
	}
	return current, true, nil
}

// importFileMatches decides whether a live file is the same upload as entry.
func importFileMatches(entry ImportEntry, file projection.FileSlim) bool {
	if entry.Encrypted != file.Encrypted {
		return false
	}
	if entry.Encrypted {
		return file.PlaintextSize > 0 && file.PlaintextSize == entry.Size
	}
	return file.Size == entry.Size
}
