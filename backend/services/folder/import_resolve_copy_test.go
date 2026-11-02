package folder

import (
	"context"
	"testing"

	"TDrive/backend/projection"
)

// importMatchKey is the part of a match the placement tests pin: which entry
// matched, where, and whether it is a name clash. The file details vary per seed.
func importMatchKey(match ImportMatch) ImportMatch {
	return ImportMatch{Index: match.Index, FolderID: match.FolderID, NameTaken: match.NameTaken}
}

func TestFindExistingSuggestsFreeCopyNames(t *testing.T) {
	svc, db, _ := newTestService(t)
	holiday, err := svc.Create(testChannelID, "Holiday", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	seedImportFile(t, db, holiday.ID, "a.jpg", 10, false, 0)
	seedImportFile(t, db, holiday.ID, "a_1.jpg", 10, false, 0)
	seedImportFile(t, db, holiday.ID, "b.jpg", 5, false, 0)

	got, err := svc.FindExistingImportEntries(context.Background(), testChannelID, "", []ImportEntry{
		{FolderPath: "Holiday", Name: "a.jpg", Size: 11},
		// b_1.jpg is also in this batch, so the copy of b.jpg must skip it.
		{FolderPath: "Holiday", Name: "b.jpg", Size: 6},
		{FolderPath: "Holiday", Name: "b_1.jpg", Size: 3},
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("matches = %+v, want the two clashes", got)
	}
	if !got[0].NameTaken || got[0].SuggestedName != "a_2.jpg" {
		t.Fatalf("a.jpg clash = %+v, want suggested a_2.jpg", got[0])
	}
	if got[0].ExistingSize != 10 || got[0].ExistingMsgID == 0 {
		t.Fatalf("a.jpg clash does not describe the live file: %+v", got[0])
	}
	if !got[1].NameTaken || got[1].SuggestedName != "b_2.jpg" {
		t.Fatalf("b.jpg clash = %+v, want suggested b_2.jpg", got[1])
	}
}

func TestFindExistingSkipsNamesOfFoldersTheImportCreates(t *testing.T) {
	svc, db, _ := newTestService(t)
	holiday, err := svc.Create(testChannelID, "Holiday", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	seedImportFile(t, db, holiday.ID, "a.jpg", 10, false, 0)

	got, err := svc.FindExistingImportEntries(context.Background(), testChannelID, "", []ImportEntry{
		{FolderPath: "Holiday", Name: "a.jpg", Size: 11},
		// The import creates a folder named a_1.jpg, so the copy must skip that name.
		{FolderPath: "Holiday", Name: "a_1.jpg", Folder: true},
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 1 || !got[0].NameTaken || got[0].Index != 0 {
		t.Fatalf("matches = %+v, want only the a.jpg clash", got)
	}
	if got[0].SuggestedName != "a_2.jpg" {
		t.Fatalf("suggested = %q, want a_2.jpg", got[0].SuggestedName)
	}
}

func TestFindExistingSameContentCarriesExistingFile(t *testing.T) {
	svc, db, _ := newTestService(t)
	holiday, err := svc.Create(testChannelID, "Holiday", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	seedImportFile(t, db, holiday.ID, "a.jpg", 10, false, 0)

	got, err := svc.FindExistingImportEntries(context.Background(), testChannelID, "", []ImportEntry{
		{FolderPath: "Holiday", Name: "a.jpg", Size: 10},
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 1 || got[0].NameTaken {
		t.Fatalf("matches = %+v, want one same-content match", got)
	}
	// The caller may need the live file for a ledger check, so it is always described.
	if got[0].ExistingSize != 10 || got[0].ExistingMsgID == 0 {
		t.Fatalf("same-content match does not describe the live file: %+v", got[0])
	}
}

func TestKeptCopyNameSkipsTakenNames(t *testing.T) {
	keyed := func(names ...string) map[string]bool {
		taken := map[string]bool{}
		for _, name := range names {
			key, err := projection.CanonicalNameKey(name)
			if err != nil {
				t.Fatalf("key %q: %v", name, err)
			}
			taken[key] = true
		}
		return taken
	}
	cases := []struct {
		name  string
		taken map[string]bool
		want  string
	}{
		{"a.txt", keyed(), "a_1.txt"},
		{"a.txt", keyed("a_1.txt"), "a_2.txt"},
		// The taken names compare canonically, so case does not free a name.
		{"a.txt", keyed("A_1.TXT"), "a_2.txt"},
		{"README", keyed(), "README_1"},
		{"archive.tar.gz", keyed(), "archive.tar_1.gz"},
		// A leading dot is part of the stem, not an extension.
		{".bashrc", keyed(), ".bashrc_1"},
	}
	for _, tc := range cases {
		got, _, err := keptCopyName(tc.name, tc.taken)
		if err != nil {
			t.Fatalf("keptCopyName(%q): %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("keptCopyName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
