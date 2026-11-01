package importledger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func photo(name string, modified int64) Entry {
	return Entry{FolderPath: "Holiday", Name: name, Size: 10, Modified: modified}
}

func TestVetoOnlyWhenModifiedTimeChanged(t *testing.T) {
	l := New(t.TempDir())
	if err := l.Record(7, "root", []Entry{photo("a.jpg", 100)}); err != nil {
		t.Fatalf("record: %v", err)
	}
	got := l.Veto(7, "root", []Entry{photo("a.jpg", 100), photo("a.jpg", 200)})
	if got[0] || !got[1] {
		t.Fatalf("veto = %v, want [false true] (same time skips, edited time vetoes)", got)
	}
}

func TestNoRecordNeverVetoes(t *testing.T) {
	l := New(t.TempDir())
	got := l.Veto(7, "root", []Entry{photo("a.jpg", 100)})
	if got[0] {
		t.Fatalf("a file with no record was vetoed")
	}
}

func TestUnknownModifiedNeverVetoes(t *testing.T) {
	l := New(t.TempDir())
	// Unknown times are never recorded, so there is nothing to compare.
	if err := l.Record(7, "root", []Entry{photo("a.jpg", 0)}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := l.Record(7, "root", []Entry{photo("b.jpg", 100)}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if got := l.Veto(7, "root", []Entry{photo("a.jpg", 300), photo("b.jpg", 0)}); got[0] || got[1] {
		t.Fatalf("unknown times vetoed: %v", got)
	}
}

func TestScopedByChannelParentAndEncryption(t *testing.T) {
	l := New(t.TempDir())
	if err := l.Record(7, "root", []Entry{photo("a.jpg", 100)}); err != nil {
		t.Fatalf("record: %v", err)
	}
	edited := photo("a.jpg", 200)
	if got := l.Veto(7, "trips", []Entry{edited}); got[0] {
		t.Fatalf("a record under root vetoed an import into trips")
	}
	if got := l.Veto(8, "root", []Entry{edited}); got[0] {
		t.Fatalf("a record on channel 7 vetoed channel 8")
	}
	enc := edited
	enc.Encrypted = true
	if got := l.Veto(7, "root", []Entry{enc}); got[0] {
		t.Fatalf("a plain record vetoed an encrypted entry")
	}
}

func TestCanonicalNamesShareOneKey(t *testing.T) {
	l := New(t.TempDir())
	if err := l.Record(7, "root", []Entry{photo("Café.jpg", 100)}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if got := l.Veto(7, "root", []Entry{photo("CAFÉ.JPG", 200)}); !got[0] {
		t.Fatalf("NFD and case-folded name did not share the NFC record")
	}
}

func TestRecordPersistsAcrossLedgers(t *testing.T) {
	dir := t.TempDir()
	if err := New(dir).Record(7, "root", []Entry{photo("a.jpg", 100)}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if got := New(dir).Veto(7, "root", []Entry{photo("a.jpg", 200)}); !got[0] {
		t.Fatalf("record did not survive a reload")
	}
}

func TestSaveFailureKeepsDirtyAndRetries(t *testing.T) {
	base := t.TempDir()
	blocked := filepath.Join(base, "blocked")
	// A regular file where the ledger directory belongs makes every save fail.
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("block dir: %v", err)
	}
	l := New(blocked)
	if err := l.Record(7, "root", []Entry{photo("a.jpg", 100)}); err == nil {
		t.Fatalf("record into a blocked directory reported success")
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	// An empty record call retries the save that failed earlier.
	if err := l.Record(7, "root", nil); err != nil {
		t.Fatalf("retry save: %v", err)
	}
	if got := New(blocked).Veto(7, "root", []Entry{photo("a.jpg", 200)}); !got[0] {
		t.Fatalf("retried save did not persist the record")
	}
}

func TestForgetClearsChannel(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	if err := l.Record(7, "root", []Entry{photo("a.jpg", 100)}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := l.Forget(7); err != nil {
		t.Fatalf("forget: %v", err)
	}
	if got := New(dir).Veto(7, "root", []Entry{photo("a.jpg", 200)}); got[0] {
		t.Fatalf("forgotten channel still vetoes")
	}
	if err := l.Forget(7); err != nil {
		t.Fatalf("forgetting an unknown channel: %v", err)
	}
}

func TestRecordEvictsOldestPastBound(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	entries := make([]Entry, 0, MaxEntries+1)
	for i := 0; i <= MaxEntries; i++ {
		entries = append(entries, Entry{FolderPath: "Bulk", Name: fmt.Sprintf("f%06d.jpg", i), Size: 1, Modified: int64(i + 1)})
	}
	if err := l.Record(7, "root", entries); err != nil {
		t.Fatalf("record: %v", err)
	}
	reloaded := New(dir)
	oldest := reloaded.Veto(7, "root", []Entry{{FolderPath: "Bulk", Name: "f000000.jpg", Size: 1, Modified: 999}})
	if oldest[0] {
		t.Fatalf("oldest record survived the bound")
	}
	newest := reloaded.Veto(7, "root", []Entry{{FolderPath: "Bulk", Name: fmt.Sprintf("f%06d.jpg", MaxEntries), Size: 1, Modified: 999}})
	if !newest[0] {
		t.Fatalf("newest record was evicted")
	}
}

func TestRecordRejectsChannelZero(t *testing.T) {
	if err := New(t.TempDir()).Record(0, "root", []Entry{photo("a.jpg", 1)}); err == nil || !strings.Contains(err.Error(), "channel") {
		t.Fatalf("record on channel 0 = %v, want an error", err)
	}
}
