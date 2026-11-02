package folder

import (
	"context"
	"testing"

	"TDrive/backend/projection"
)

// A file named DCIM that is still live when a folder DCIM is synced takes the
// name, so the folder's own namespace entry gets a collision alias. Once that
// file leaves the drive the folder is still shown under the root, and an import
// must reuse it rather than ask CreateContext for a second DCIM. A file already
// inside that folder must then be found as already imported, not uploaded again.
func TestResolveReusesFolderWhoseNameEntryWasAliased(t *testing.T) {
	svc, db, _ := newTestService(t)
	seedImportFile(t, db, projection.RootParent, "DCIM", 3, false, 0)
	dcim, err := svc.Create(testChannelID, "DCIM", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE dirents SET tombstoned=1
		WHERE channel_id=? AND object_kind='file' AND display_name='DCIM'
	`, testChannelID); err != nil {
		t.Fatalf("trash file entry: %v", err)
	}
	if _, found, err := projection.LiveDirentByName(db, testChannelID, projection.RootParent, "DCIM"); err != nil || found {
		t.Fatalf("precondition: DCIM still resolves by name (found=%v err=%v)", found, err)
	}

	ids, err := svc.ResolveImportFolders(context.Background(), testChannelID, "", []string{"DCIM"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ids["DCIM"] != dcim.ID {
		t.Fatalf("resolve DCIM = %q, want reuse of %q", ids["DCIM"], dcim.ID)
	}

	seedImportFile(t, db, dcim.ID, "a.txt", 3, false, 0)
	matches, err := svc.FindExistingImportEntries(context.Background(), testChannelID, "", []ImportEntry{
		{FolderPath: "DCIM", Name: "a.txt", Size: 3},
	})
	if err != nil {
		t.Fatalf("find existing: %v", err)
	}
	if len(matches) != 1 || matches[0].FolderID != dcim.ID {
		t.Fatalf("matches = %+v, want a.txt found in %q", matches, dcim.ID)
	}
}
