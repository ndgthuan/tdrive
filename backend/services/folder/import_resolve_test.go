package folder

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"TDrive/backend/projection"
)

// seedMsgIDs keeps seeded message ids clear of the ids the fixture's own
// emitter hands out, so a seeded op never collides with a folder op.
var seedMsgIDs int64 = 500000

// seedImportFile commits one file into the test drive as a committed file op.
func seedImportFile(t *testing.T, db *sql.DB, parent, name string, size int64, encrypted bool, plaintext int64) {
	t.Helper()
	seedMsgIDs++
	msgID := seedMsgIDs
	op := projection.Op{
		Type: projection.OpFileCommit, ProtocolVersion: 1,
		OpID:   fmt.Sprintf("seed-%d", msgID),
		Parent: parent, Name: name, ContentMsgID: msgID + 1000, ContentHash: "h",
		FileSize: size, FileUploadTime: 100,
		Encrypted: encrypted, PlaintextSize: plaintext,
	}
	if encrypted {
		op.EncryptionVersion = 1
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin seed %q: %v", name, err)
	}
	if err := projection.ApplyOp(tx, testChannelID, msgID, op, 0); err != nil {
		_ = tx.Rollback()
		t.Fatalf("apply seed %q: %v", name, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed %q: %v", name, err)
	}
}

func TestResolveReusesLiveFolderByCanonicalName(t *testing.T) {
	svc, _, _ := newTestService(t)
	camera, err := svc.Create(testChannelID, "Camera", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	cases := []string{"Camera", "camera", "CAMERA", "Camera ", " Camera"}
	for _, path := range cases {
		ids, err := svc.ResolveImportFolders(context.Background(), testChannelID, "", []string{path})
		if err != nil {
			t.Fatalf("resolve %q: %v", path, err)
		}
		if ids[path] != camera.ID {
			t.Fatalf("resolve %q = %q, want reuse of %q", path, ids[path], camera.ID)
		}
	}
}

func TestResolveReusesFolderAcrossNormalizationForms(t *testing.T) {
	svc, _, _ := newTestService(t)
	nfc := "Café"  // precomposed e-acute
	nfd := "Café" // e followed by combining acute
	existing, err := svc.Create(testChannelID, nfc, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ids, err := svc.ResolveImportFolders(context.Background(), testChannelID, "", []string{nfd})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ids[nfd] != existing.ID {
		t.Fatalf("NFD name did not reuse the NFC folder: %q vs %q", ids[nfd], existing.ID)
	}
}

func TestResolveCreatesOnceForRepeatedImports(t *testing.T) {
	svc, db, _ := newTestService(t)
	first, err := svc.ResolveImportFolders(context.Background(), testChannelID, "", []string{"Trip/day1", "Trip/day2"})
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	second, err := svc.ResolveImportFolders(context.Background(), testChannelID, "", []string{"Trip/day1", "Trip/day2"})
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if first["Trip/day1"] != second["Trip/day1"] || first["Trip/day2"] != second["Trip/day2"] {
		t.Fatalf("second import created new folders: %v vs %v", first, second)
	}
	if !projection.FolderExists(db, testChannelID, first["Trip/day1"]) {
		t.Fatalf("resolved folder was not projected")
	}
}

func TestResolveRefusesFileWithFolderName(t *testing.T) {
	svc, db, _ := newTestService(t)
	seedImportFile(t, db, projection.RootParent, "Photos", 10, false, 0)

	_, err := svc.ResolveImportFolders(context.Background(), testChannelID, "", []string{"Photos/sub"})
	if !errors.Is(err, ErrImportConflict) {
		t.Fatalf("err = %v, want ErrImportConflict", err)
	}
	if _, err := svc.ResolveImportFolders(context.Background(), testChannelID, "", []string{"Photos"}); !errors.Is(err, ErrImportConflict) {
		t.Fatalf("file named like the folder: err = %v, want ErrImportConflict", err)
	}
}

func TestResolveRejectsAmbiguousSiblings(t *testing.T) {
	svc, db, _ := newTestService(t)
	_, err := svc.ResolveImportFolders(context.Background(), testChannelID, "", []string{"Camera/a", "camera/b"})
	if !errors.Is(err, ErrImportAmbiguous) {
		t.Fatalf("err = %v, want ErrImportAmbiguous", err)
	}
	folders, _, err := projection.ListFolderContentsContext(context.Background(), db, testChannelID, projection.RootParent)
	if err != nil {
		t.Fatalf("list root: %v", err)
	}
	if len(folders) != 0 {
		t.Fatalf("ambiguous resolve created folders: %+v", folders)
	}
}

func TestFindExistingMatchesLiveSameSizeOnly(t *testing.T) {
	svc, db, _ := newTestService(t)
	holiday, err := svc.Create(testChannelID, "Holiday", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	seedImportFile(t, db, holiday.ID, "a.jpg", 10, false, 0)

	entries := []ImportEntry{
		{FolderPath: "Holiday", Name: "a.jpg", Size: 10},
		{FolderPath: "Holiday", Name: "A.JPG", Size: 10},
		{FolderPath: "Holiday", Name: "a.jpg", Size: 11},
		{FolderPath: "Holiday", Name: "b.jpg", Size: 10},
		{FolderPath: "Missing", Name: "a.jpg", Size: 10},
		{FolderPath: "Holiday", Name: "a.jpg", Size: -1},
	}
	got, err := svc.FindExistingImportEntries(context.Background(), testChannelID, "", entries)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	// A different size under the same name is reported as a name clash, not a
	// match: the upload would be renamed beside the existing file.
	want := []ImportMatch{
		{Index: 0, FolderID: holiday.ID},
		{Index: 1, FolderID: holiday.ID},
		{Index: 2, FolderID: holiday.ID, NameTaken: true},
	}
	if len(got) != len(want) {
		t.Fatalf("matches = %+v, want %+v", got, want)
	}
	for i := range want {
		if importMatchKey(got[i]) != want[i] {
			t.Fatalf("matches = %+v, want %+v", got, want)
		}
	}
}

func TestFindExistingIgnoresTrashedFolders(t *testing.T) {
	svc, db, _ := newTestService(t)
	old, err := svc.Create(testChannelID, "Old", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	seedImportFile(t, db, old.ID, "a.jpg", 10, false, 0)
	if err := svc.Delete(context.Background(), testChannelID, old.ID); err != nil {
		t.Fatalf("delete folder: %v", err)
	}

	got, err := svc.FindExistingImportEntries(context.Background(), testChannelID, "", []ImportEntry{
		{FolderPath: "Old", Name: "a.jpg", Size: 10},
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("trashed folder matched: %+v", got)
	}
}

func TestFindExistingEncryptedPolicy(t *testing.T) {
	svc, db, _ := newTestService(t)
	seedImportFile(t, db, projection.RootParent, "plain.jpg", 10, false, 0)
	seedImportFile(t, db, projection.RootParent, "secret.jpg", 40, true, 30)

	got, err := svc.FindExistingImportEntries(context.Background(), testChannelID, "", []ImportEntry{
		// An encrypted source never matches a plain file of the same name.
		{Name: "plain.jpg", Size: 10, Encrypted: true},
		// Encrypted matches on plaintext size against an encrypted file.
		{Name: "secret.jpg", Size: 30, Encrypted: true},
		// A plain source never matches an encrypted file.
		{Name: "secret.jpg", Size: 40, Encrypted: false},
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	// Only the encrypted same-plaintext entry is a match. The two mismatches
	// share a live name with a file they cannot match, so both are name clashes.
	want := []ImportMatch{
		{Index: 0, FolderID: projection.RootParent, NameTaken: true},
		{Index: 1, FolderID: projection.RootParent},
		{Index: 2, FolderID: projection.RootParent, NameTaken: true},
	}
	if len(got) != len(want) {
		t.Fatalf("matches = %+v, want %+v", got, want)
	}
	for i := range want {
		if importMatchKey(got[i]) != want[i] {
			t.Fatalf("matches = %+v, want %+v", got, want)
		}
	}
}
