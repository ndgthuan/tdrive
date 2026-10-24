package projection

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// project_op writes the op into replay_log and applies it, the same way the
// sync path does. runOp alone would leave no control messages to plan.
func project_op(t *testing.T, db *sql.DB, msgID int64, op Op) {
	t.Helper()
	if _, err := ProjectFromOp(db, testChan, msgID, op, 0, Format(op)); err != nil {
		t.Fatalf("project msg=%d %s: %v", msgID, op.Type, err)
	}
}

func TestLocalPurgePlansEveryMessageOfAFolderSubtree(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	project_op(t, db, 1, Op{Type: OpMkdir, Obj: "d:a", Parent: RootParent, Name: "A"})
	project_op(t, db, 2, Op{Type: OpFileUpload, Parent: "d:a", Name: "in.txt", FileSize: 5})
	project_op(t, db, 3, Op{Type: OpRename, Obj: "f:2", Name: "renamed.txt"})
	project_op(t, db, 5, Op{Type: OpMkdir, Obj: "d:b", Parent: RootParent, Name: "B"})
	project_op(t, db, 6, Op{Type: OpFileUpload, Parent: RootParent, Name: "keep.txt", FileSize: 7})

	entry, found, err := DirentByID(db, testChan, "d:a")
	if err != nil || !found {
		t.Fatalf("folder d:a: found=%v err=%v", found, err)
	}
	project_op(t, db, 4, Op{Type: OpTrashTree, ProtocolVersion: 1, OpID: "trash-a", Obj: "d:a",
		ExpectedRevision: entry.Revision, DeletedAt: 100, PurgeAfter: 200})

	revision, err := RegisterTrashPurgeIntent(ctx, db, testChan, "purge-a", "d:a")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := CaptureLocalPurge(ctx, db, testChan, "purge-a", "d:a", revision); err != nil {
		t.Fatalf("capture: %v", err)
	}
	// Repeating the capture after an interruption must be a no-op.
	if err := CaptureLocalPurge(ctx, db, testChan, "purge-a", "d:a", revision); err != nil {
		t.Fatalf("repeat capture: %v", err)
	}
	assertHardDeletePlan(t, db, "purge-a", []int64{1, 2, 3, 4}, 4, true)

	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM replay_log WHERE channel_id=? AND msg_id IN (1,2,3,4)`, testChan).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d purged messages still in replay_log", left)
	}
	refs, err := LiveFileMessageIDs(db, testChan)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].FileMsgID != 6 {
		t.Fatalf("live files after purge = %+v, want only msg 6", refs)
	}
	if FileIsLive(db, testChan, 2) || !FileIsLive(db, testChan, 6) {
		t.Fatal("FileIsLive disagrees with the purge")
	}
	if entries, err := ListTrashEntries(db, testChan); err != nil || len(entries) != 0 {
		t.Fatalf("trash after purge = %+v, %v", entries, err)
	}

	// A rebuild from replay_log must not bring the subtree back.
	if err := RebuildProjection(db, testChan); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if _, found, _ := DirentByID(db, testChan, "d:a"); found {
		t.Fatal("rebuild resurrected the purged folder")
	}
	if FileExists(db, testChan, 2) {
		t.Fatal("rebuild resurrected a file from the purged folder")
	}
	if !FileExists(db, testChan, 6) {
		t.Fatal("rebuild lost an unrelated file")
	}
	if _, found, _ := DirentByID(db, testChan, "d:b"); !found {
		t.Fatal("rebuild lost an unrelated folder")
	}
}

func TestLocalPurgeFileRemovesUploadRenameAndTrashMessages(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	project_op(t, db, 10, Op{Type: OpFileUpload, Parent: RootParent, Name: "photo.jpg", FileSize: 1000})
	project_op(t, db, 11, Op{Type: OpRename, Obj: "f:10", Name: "photo-2.jpg"})
	entry, _, err := DirentByID(db, testChan, "f:10")
	if err != nil {
		t.Fatal(err)
	}
	// A live object cannot be captured as a purge.
	if err := CaptureLocalPurge(ctx, db, testChan, "purge-live", "f:10", entry.Revision); !errors.Is(err, ErrBadOp) {
		t.Fatalf("capture of a live object err = %v, want ErrBadOp", err)
	}
	project_op(t, db, 12, Op{Type: OpTrashTree, ProtocolVersion: 1, OpID: "trash-f10", Obj: "f:10",
		ExpectedRevision: entry.Revision, DeletedAt: 100, PurgeAfter: 200})

	revision, err := RegisterTrashPurgeIntent(ctx, db, testChan, "purge-f10", "f:10")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := CaptureLocalPurge(ctx, db, testChan, "purge-f10", "f:10", revision); err != nil {
		t.Fatalf("capture: %v", err)
	}
	assertHardDeletePlan(t, db, "purge-f10", []int64{10, 11, 12}, 3, true)
	if refs, err := LiveFileMessageIDs(db, testChan); err != nil || len(refs) != 0 {
		t.Fatalf("live refs after purge = %+v, %v", refs, err)
	}
}
