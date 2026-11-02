package file

import (
	"context"
	"fmt"
	"testing"

	"TDrive/backend/projection"
)

func TestReplaceImportedTrashesOldAndNamesNew(t *testing.T) {
	isolate_data_dir(t)
	svc, db, _, _ := newTestService(t)
	project(t, db, personalChannelID, 60, 7, projection.Op{
		Type:           projection.OpFileUpload,
		Parent:         "",
		Name:           "a.txt",
		FileSize:       3,
		FileUploadTime: 1,
	})
	// The import kept the new content under its copy name while it was uploaded.
	project(t, db, personalChannelID, 61, 7, projection.Op{
		Type:           projection.OpFileUpload,
		Parent:         "",
		Name:           "a_1.txt",
		FileSize:       9,
		FileUploadTime: 2,
	})

	if err := svc.ReplaceImported(context.Background(), personalChannelID, 60, 61, "a.txt"); err != nil {
		t.Fatalf("replace imported: %v", err)
	}

	oldEntry, found, err := projection.DirentByID(db, personalChannelID, fmt.Sprintf("%s%d", projection.FileIDPrefix, 60))
	if err != nil {
		t.Fatalf("old dirent: %v", err)
	}
	if !found || !oldEntry.Tombstoned {
		t.Fatalf("old file = %+v found=%v, want trashed", oldEntry, found)
	}
	newFile, found, err := projection.FileByID(db, personalChannelID, 61)
	if err != nil {
		t.Fatalf("new file: %v", err)
	}
	if !found || newFile.Name != "a.txt" {
		t.Fatalf("new file = %+v found=%v, want named a.txt", newFile, found)
	}
}

func TestReplaceImportedKeepsOldWhenNewCopyIsGone(t *testing.T) {
	isolate_data_dir(t)
	svc, db, _, _ := newTestService(t)
	project(t, db, personalChannelID, 60, 7, projection.Op{
		Type:           projection.OpFileUpload,
		Parent:         "",
		Name:           "a.txt",
		FileSize:       3,
		FileUploadTime: 1,
	})
	// The copy 61 is never projected, as if it was removed before the swap.

	if err := svc.ReplaceImported(context.Background(), personalChannelID, 60, 61, "a.txt"); err == nil {
		t.Fatalf("replace with a missing copy succeeded")
	}
	oldEntry, found, err := projection.DirentByID(db, personalChannelID, fmt.Sprintf("%s%d", projection.FileIDPrefix, 60))
	if err != nil {
		t.Fatalf("old dirent: %v", err)
	}
	if !found || oldEntry.Tombstoned {
		t.Fatalf("old file = %+v found=%v, want still live", oldEntry, found)
	}
}

func TestValidateUploadName(t *testing.T) {
	for _, name := range []string{"", "a.txt", "a_1.txt", "Ảnh 1.jpg"} {
		if err := validateUploadName(name); err != nil {
			t.Fatalf("validateUploadName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range []string{" ", ".", "..", "x/y.txt", `x\y.txt`} {
		if err := validateUploadName(name); err == nil {
			t.Fatalf("validateUploadName(%q) accepted a name that is not one segment", name)
		}
	}
}
