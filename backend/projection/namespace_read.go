package projection

import (
	"database/sql"
	"errors"
	"fmt"
)

// DirentByID reads unified namespace metadata for a logical file or folder.
// Tombstoned entries are returned so recovery and cleanup can inspect them.
func DirentByID(db *sql.DB, channelID int64, objectID string) (Dirent, bool, error) {
	if db == nil {
		return Dirent{}, false, fmt.Errorf("projection: dirent by id: db is nil")
	}
	if channelID == 0 || (!IsFileID(objectID) && !IsFolderID(objectID)) {
		return Dirent{}, false, fmt.Errorf("projection: dirent by id: invalid identity")
	}
	return scanDirent(db.QueryRow(`
		SELECT channel_id, object_id, object_kind, parent_id, display_name,
		       name_key, revision, tombstoned
		FROM dirents WHERE channel_id=? AND object_id=?
	`, channelID, objectID))
}

// LiveDirentByName resolves one portable sibling name using the exact same
// NFC/case-fold key used during projection replay.
func LiveDirentByName(db *sql.DB, channelID int64, parentID, name string) (Dirent, bool, error) {
	if db == nil {
		return Dirent{}, false, fmt.Errorf("projection: live dirent by name: db is nil")
	}
	if channelID == 0 || (parentID != RootParent && !IsFolderID(parentID)) {
		return Dirent{}, false, fmt.Errorf("projection: live dirent by name: invalid parent")
	}
	key, err := CanonicalNameKey(name)
	if err != nil {
		return Dirent{}, false, err
	}
	return scanDirent(db.QueryRow(`
		SELECT channel_id, object_id, object_kind, parent_id, display_name,
		       name_key, revision, tombstoned
		FROM dirents
		WHERE channel_id=? AND parent_id=? AND name_key=? AND tombstoned=0
	`, channelID, parentID, key))
}

// LiveFolderNamedUnder finds the live folder called name under parentID, using
// the same canonical comparison as LiveDirentByName. A folder whose namespace
// entry took a collision alias when it was synced is still listed under
// parentID, but its entry no longer carries name, so LiveDirentByName misses
// it. Matching on the folder's own name and requiring a live entry under
// parentID finds that folder without accepting one the drive does not show.
func LiveFolderNamedUnder(db *sql.DB, channelID int64, parentID, name string) (string, bool, error) {
	if db == nil {
		return "", false, fmt.Errorf("projection: live folder named under: db is nil")
	}
	if channelID == 0 || (parentID != RootParent && !IsFolderID(parentID)) {
		return "", false, fmt.Errorf("projection: live folder named under: invalid parent")
	}
	key, err := CanonicalNameKey(name)
	if err != nil {
		return "", false, err
	}
	rows, err := db.Query(`
		SELECT f.id, f.name
		FROM folders f
		JOIN dirents d ON d.channel_id = f.channel_id AND d.object_id = f.id
		WHERE f.channel_id = ? AND f.parent_id = ? AND f.tombstoned = 0
		  AND d.parent_id = ? AND d.object_kind = 'folder' AND d.tombstoned = 0
	`, channelID, parentID, parentID)
	if err != nil {
		return "", false, fmt.Errorf("projection: live folder named under: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, folderName string
		if err := rows.Scan(&id, &folderName); err != nil {
			return "", false, fmt.Errorf("projection: scan live folder: %w", err)
		}
		folderKey, err := CanonicalNameKey(folderName)
		if err != nil {
			continue
		}
		if folderKey == key {
			return id, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, fmt.Errorf("projection: iterate live folders: %w", err)
	}
	return "", false, nil
}

func scanDirent(scanner sqlScanner) (Dirent, bool, error) {
	var entry Dirent
	var tombstoned int
	err := scanner.Scan(
		&entry.ChannelID, &entry.ObjectID, &entry.ObjectKind, &entry.ParentID,
		&entry.DisplayName, &entry.NameKey, &entry.Revision, &tombstoned,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Dirent{}, false, nil
	}
	if err != nil {
		return Dirent{}, false, fmt.Errorf("projection: scan dirent: %w", err)
	}
	entry.Tombstoned = tombstoned != 0
	return entry, true, nil
}
