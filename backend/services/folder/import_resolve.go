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

// maxKeptCopies bounds the "name_N" search for a free name for a kept copy.
const maxKeptCopies = 999

// ImportEntry is one file an import is about to upload, described by where it
// goes. FolderPath is "/"-separated and relative to the import's parent folder;
// its first segment is the picked folder. Folder marks a folder the import will
// create at FolderPath/Name instead: it never matches, but its name is taken, so
// no copy of a file may be suggested under it.
type ImportEntry struct {
	FolderPath string `json:"folder_path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Encrypted  bool   `json:"encrypted"`
	Folder     bool   `json:"folder"`
}

// ImportMatch names an entry that already exists live on the drive, and the
// folder it was found in. NameTaken marks a live file with the entry's name but
// different content: an upload would be renamed beside it, so the user decides
// whether to replace that file or keep a copy. ExistingMsgID and ExistingSize
// describe the live file, and SuggestedName is the free name a kept copy takes
// ("" when none is free).
type ImportMatch struct {
	Index         int    `json:"index"`
	FolderID      string `json:"folder_id"`
	NameTaken     bool   `json:"name_taken"`
	ExistingMsgID int64  `json:"existing_msg_id"`
	ExistingSize  int64  `json:"existing_size"`
	SuggestedName string `json:"suggested_name"`
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
	// CreateContext refuses any live folder with this exact name, even one the
	// name lookup missed, so reuse that folder before asking to create it.
	id, ok, err := projection.LiveFolderNamedUnder(s.DB, channelID, parent, name)
	if err != nil {
		return "", err
	}
	if ok {
		return id, nil
	}
	created, err := s.CreateContext(ctx, channelID, name, parent)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// FindExistingImportEntries reports which entries already exist live under
// parentID. A match is a folder with the same canonical path, or a file with the
// same canonical name and the same size. Trashed or deleted entries are not live
// and never match. Encrypted files only match an encrypted entry whose plaintext
// size equals the stored plaintext size, and never a plain one; an entry with an
// unknown size (negative) never matches. A live file with the entry's canonical
// name that fails that test is a name clash, reported with NameTaken set.
//
// SuggestedName is the free copy name for every file match. It skips the names
// the drive holds in that folder and the names the other entries of this batch
// land under, so two copies never claim the same name. Nothing is created.
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

	type placement struct {
		index    int
		entry    ImportEntry
		folderID string
		key      string
	}
	var placed []placement
	for index, entry := range entries {
		key, err := projection.CanonicalNameKey(entry.Name)
		if err != nil {
			continue
		}
		folderID, ok, err := s.liveImportFolder(channelID, parent, entry.FolderPath, folderByPath, missing)
		if err != nil {
			return nil, err
		}
		if ok {
			placed = append(placed, placement{index: index, entry: entry, folderID: folderID, key: key})
		}
	}

	// Per folder: the canonical names the drive holds there, and the live file
	// behind each name. The first live file wins, as it always has.
	taken := map[string]map[string]bool{}
	live := map[string]map[string]projection.FileSlim{}
	for _, item := range placed {
		if _, loaded := taken[item.folderID]; loaded {
			continue
		}
		folders, files, err := projection.ListFolderContentsContext(ctx, s.DB, channelID, item.folderID)
		if err != nil {
			return nil, err
		}
		taken[item.folderID] = map[string]bool{}
		live[item.folderID] = map[string]projection.FileSlim{}
		for _, folder := range folders {
			if key, err := projection.CanonicalNameKey(folder.Name); err == nil {
				taken[item.folderID][key] = true
			}
		}
		for _, file := range files {
			key, err := projection.CanonicalNameKey(file.Name)
			if err != nil {
				continue
			}
			taken[item.folderID][key] = true
			if _, seen := live[item.folderID][key]; !seen {
				live[item.folderID][key] = file
			}
		}
	}
	// The batch's own names are taken too, so a copy never lands on one of them.
	for _, item := range placed {
		taken[item.folderID][item.key] = true
	}

	var matches []ImportMatch
	for _, item := range placed {
		if item.entry.Folder || item.entry.Size < 0 {
			continue
		}
		file, found := live[item.folderID][item.key]
		if !found {
			continue
		}
		match := ImportMatch{
			Index:         item.index,
			FolderID:      item.folderID,
			NameTaken:     !importFileMatches(item.entry, file),
			ExistingMsgID: file.MsgID,
			ExistingSize:  existingImportSize(file),
		}
		if suggested, key, err := keptCopyName(item.entry.Name, taken[item.folderID]); err == nil {
			taken[item.folderID][key] = true
			match.SuggestedName = suggested
		}
		matches = append(matches, match)
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
		if found && entry.ObjectKind == "folder" {
			known[raw] = entry.ObjectID
			current = entry.ObjectID
			continue
		}
		if found {
			missing[raw] = true
			return "", false, nil
		}
		id, ok, err := projection.LiveFolderNamedUnder(s.DB, channelID, current, segment)
		if err != nil {
			return "", false, err
		}
		if !ok {
			missing[raw] = true
			return "", false, nil
		}
		known[raw] = id
		current = id
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

// existingImportSize is the size the drive shows for a file: the plaintext size
// for an encrypted file, the stored size otherwise.
func existingImportSize(file projection.FileSlim) int64 {
	if file.Encrypted {
		return file.PlaintextSize
	}
	return file.Size
}

// keptCopyName names a copy kept beside a file of the same name: "a.txt" becomes
// "a_1.txt", then "a_2.txt", and so on, skipping every canonical name in taken.
// It returns the name and its canonical key.
func keptCopyName(name string, taken map[string]bool) (string, string, error) {
	stem, ext := name, ""
	if dot := strings.LastIndex(name, "."); dot > 0 {
		stem, ext = name[:dot], name[dot:]
	}
	for n := 1; n <= maxKeptCopies; n++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, n, ext)
		key, err := projection.CanonicalNameKey(candidate)
		if err != nil {
			return "", "", err
		}
		if !taken[key] {
			return candidate, key, nil
		}
	}
	return "", "", fmt.Errorf("folder: no free copy name for %q", name)
}
