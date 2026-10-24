package file

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"TDrive/backend/projection"
)

// A purge must leave no trace in the channel: it deletes every message that
// belongs to the object and sends no hard-delete or tombstone record.
func TestPurgeLeavesNoRecordInTheChannel(t *testing.T) {
	isolate_data_dir(t)
	svc, db, fakeTG, _ := newTestService(t)
	seedCommittedFile(t, db, 2001, 9001, "gone.txt")
	project(t, db, personalChannelID, 2003, 7, projection.Op{Type: projection.OpRename, Obj: "f:2001", Name: "renamed.txt"})
	if err := svc.Delete(context.Background(), personalChannelID, 2001); err != nil {
		t.Fatalf("delete: %v", err)
	}
	ownedBefore := replay_msg_ids(t, db)
	controlsBefore := len(fakeTG.SentControls())

	if err := svc.PurgeObject(context.Background(), personalChannelID, "f:2001"); err != nil {
		t.Fatalf("purge: %v", err)
	}

	if got := len(fakeTG.SentControls()); got != controlsBefore {
		t.Fatalf("purge sent %d new control messages", got-controlsBefore)
	}
	for _, control := range fakeTG.SentControls() {
		if strings.Contains(control.Text, "hard_delete") || strings.Contains(control.Text, "tombstone") {
			t.Fatalf("purge sent a deletion record: %q", control.Text)
		}
	}
	for _, id := range append(ownedBefore, 9001) {
		if !sawDeletedMessage(fakeTG, id) {
			t.Fatalf("purge left message %d in Telegram", id)
		}
	}
	if rest := replay_msg_ids(t, db); len(rest) != 0 {
		t.Fatalf("replay_log still lists %v after purge", rest)
	}

	// Rebuilding from what is left in the channel must not resurrect the file.
	if err := projection.RebuildProjection(db, personalChannelID); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if projection.FileExists(db, personalChannelID, 2001) {
		t.Fatal("rebuild resurrected the purged file")
	}
	if ids := trashedObjectIDs(t, svc); len(ids) != 0 {
		t.Fatalf("trash lists %v after purge and rebuild", ids)
	}
}

// replay_msg_ids lists every message the projection still remembers for the
// personal channel, which is what a rebuild would replay.
func replay_msg_ids(t *testing.T, db *sql.DB) []int64 {
	t.Helper()
	rows, err := db.Query(`SELECT msg_id FROM replay_log WHERE channel_id=? ORDER BY msg_id`, personalChannelID)
	if err != nil {
		t.Fatalf("list replay_log: %v", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan replay_log: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate replay_log: %v", err)
	}
	return ids
}
