package projection

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// CaptureLocalPurge records the cleanup plan for permanently destroying one
// trashed object without publishing a harddel marker or a tombstone to the
// channel. The plan holds every Telegram message that belongs to the object:
// body and part messages, the upload or manifest message that defines the
// file, and every control message that names it or a folder beneath it (mkdir,
// rename, move, trash, restore, metadata, commits). Folder purges include the
// subtree's folders and the files inside them.
//
// The local projection is updated in the same transaction. The object's trash
// entry and part pointers are dropped, and the replay_log rows for every
// planned message are removed, so a rebuild from replay_log cannot resurrect
// the object. The Telegram deletion itself runs afterwards from the plan, and
// it is safe to retry: a plan that already exists is a no-op here.
//
// revision must equal the trashed object's current revision; the caller claims
// the purge intent first, which also checks the trash entry.
func CaptureLocalPurge(ctx context.Context, db *sql.DB, channelID int64, opID, objectID string, revision int64) (err error) {
	if err := validateHardDeletePlanRequest(ctx, db, channelID, opID); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("projection: begin local purge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := validateHardDeleteChannel(tx, channelID); err != nil {
		return err
	}
	if _, exists, err := hardDeleteJobStateTx(tx, channelID, opID, objectID); err != nil {
		return err
	} else if exists {
		// An interrupted purge already captured and locally applied its plan.
		return nil
	}
	scope, err := loadHardDeleteScope(tx, channelID, Op{Obj: objectID, ExpectedRevision: revision})
	if err != nil {
		return err
	}
	if !scope.trashed {
		return fmt.Errorf("%w: local purge requires a trashed object", ErrBadOp)
	}
	if err := validateHardDeleteContent(tx, scope); err != nil {
		return err
	}
	// Body messages come from the same query the marker path uses. Passing a
	// zero marker means no message is excluded from the plan.
	if err := createHardDeletePlanTx(tx, scope, 0, opID); err != nil {
		return err
	}
	if err := plan_object_control_messages(tx, scope, opID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE hard_delete_jobs
		SET total_messages=(
			SELECT COUNT(*) FROM hard_delete_plan_items
			WHERE channel_id=? AND op_id=?
		)
		WHERE channel_id=? AND op_id=?
	`, channelID, opID, channelID, opID); err != nil {
		return fmt.Errorf("projection: count local purge plan: %w", err)
	}
	for _, table := range []string{"replay_log", "replay_log_tamper", "replay_log_rejects"} {
		if _, err := tx.Exec(`
			DELETE FROM `+table+`
			WHERE channel_id=? AND msg_id IN (
				SELECT msg_id FROM hard_delete_plan_items
				WHERE channel_id=? AND op_id=?
			)
		`, channelID, channelID, opID); err != nil {
			return fmt.Errorf("projection: forget purged messages in %s: %w", table, err)
		}
	}
	if err := removeHardDeletedPartPointersTx(tx, scope); err != nil {
		return err
	}
	if err := clearHardDeleteRetentionTx(tx, scope); err != nil {
		return err
	}
	if err := removeHardDeleteTrashEntriesTx(tx, scope); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("projection: commit local purge: %w", err)
	}
	return nil
}

// plan_object_control_messages adds every replay_log message that defines or
// changes the purged objects. An op belongs to the purge when its message is
// the file's own message, or when its object is the purged root, a folder in
// its subtree, or a file in its subtree.
func plan_object_control_messages(tx *sql.Tx, scope hardDeleteScope, opID string) error {
	file_msg_ids, err := purge_file_msg_ids(tx, scope)
	if err != nil {
		return err
	}
	objects := map[string]bool{scope.objectID: true}
	for _, msg_id := range file_msg_ids {
		objects[fmt.Sprintf("%s%d", FileIDPrefix, msg_id)] = true
	}
	if scope.objectKind == ObjectKindFolder {
		folder_ids, err := purge_folder_ids(tx, scope)
		if err != nil {
			return err
		}
		for _, id := range folder_ids {
			objects[id] = true
		}
	}
	files := make(map[int64]bool, len(file_msg_ids))
	for _, msg_id := range file_msg_ids {
		files[msg_id] = true
	}

	rows, err := tx.Query(`SELECT msg_id, op_payload_json FROM replay_log WHERE channel_id=?`, scope.channelID)
	if err != nil {
		return fmt.Errorf("projection: scan replay_log for purge: %w", err)
	}
	var owned []int64
	for rows.Next() {
		var msg_id int64
		var payload string
		if err := rows.Scan(&msg_id, &payload); err != nil {
			_ = rows.Close()
			return fmt.Errorf("projection: scan replay_log for purge: %w", err)
		}
		var probe struct{ Obj string }
		if err := json.Unmarshal([]byte(payload), &probe); err != nil {
			_ = rows.Close()
			return fmt.Errorf("projection: decode replay op msg=%d: %w", msg_id, err)
		}
		if files[msg_id] || (probe.Obj != "" && objects[probe.Obj]) {
			owned = append(owned, msg_id)
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("projection: close replay_log purge scan: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("projection: iterate replay_log for purge: %w", err)
	}
	for msg_id := range files {
		owned = append(owned, msg_id)
	}
	for _, msg_id := range owned {
		if msg_id <= 0 {
			continue
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO hard_delete_plan_items (channel_id, op_id, msg_id)
			VALUES (?, ?, ?)
		`, scope.channelID, opID, msg_id); err != nil {
			return fmt.Errorf("projection: plan control message %d: %w", msg_id, err)
		}
	}
	return nil
}

// purge_file_msg_ids lists the file identities (their defining message ids)
// covered by the purge, including files already tombstoned inside a folder.
func purge_file_msg_ids(tx *sql.Tx, scope hardDeleteScope) ([]int64, error) {
	query, args := scope.withAffectedFiles(`SELECT file_msg_id FROM affected_files`)
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("projection: list purged files: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("projection: scan purged file: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// purge_folder_ids lists the purged folder and every folder beneath it, as
// object ids. Tombstoned folders are included on purpose.
func purge_folder_ids(tx *sql.Tx, scope hardDeleteScope) ([]string, error) {
	rows, err := tx.Query(`
		WITH RECURSIVE subtree(id) AS (
			SELECT id FROM folders WHERE channel_id=? AND id=?
			UNION
			SELECT child.id FROM folders child
			JOIN subtree parent ON child.parent_id=parent.id
			WHERE child.channel_id=?
		)
		SELECT id FROM subtree
	`, scope.channelID, scope.objectID, scope.channelID)
	if err != nil {
		return nil, fmt.Errorf("projection: list purged folders: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("projection: scan purged folder: %w", err)
		}
		if strings.TrimSpace(id) != "" {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}
