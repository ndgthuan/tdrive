// Package importledger remembers, per drive and destination folder, the files a
// folder import last uploaded. It never decides that a file is on the drive.
// That is the live drive state's job (see services/folder). The ledger can only
// veto a skip: when live state shows a same-size file but the ledger recorded
// the same name with a different, known modification time, the edited file must
// upload again.
//
// A key is the channel, the destination folder id, the canonical path segments
// of the picked folder and file, and the encryption flag. Canonical names match
// the projection's rules, so NFC/NFD and letter case do not split one file into
// two keys.
package importledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"TDrive/backend/projection"
)

// DirName is the folder under the app data directory that holds one ledger
// file per channel.
const DirName = "import-ledger"

// MaxEntries bounds one channel's ledger. Past it the oldest records are
// evicted: a forgotten file can only upload again, which is the safe direction.
const MaxEntries = 50_000

// maxKeyBytes drops absurdly long keys rather than letting one bloat the file.
const maxKeyBytes = 8192

// Entry is one uploaded file. FolderPath is relative to the picked import
// folder's parent: its first segment is the picked folder's name. Modified is
// the source file's modification time in unix milliseconds, 0 when unknown.
type Entry struct {
	FolderPath string `json:"folder_path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Modified   int64  `json:"modified"`
	Encrypted  bool   `json:"encrypted"`
}

type record struct {
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	Seq      uint64 `json:"seq"`
}

type channelFile struct {
	Version int               `json:"version"`
	Seq     uint64            `json:"seq"`
	Entries map[string]record `json:"entries"`
}

type channelState struct {
	entries map[string]record
	seq     uint64
	dirty   bool
}

// Ledger is safe for concurrent use. Each channel's file is read on first use
// and then kept in memory.
type Ledger struct {
	dir      string
	mu       sync.Mutex
	channels map[int64]*channelState
}

// New returns a ledger that keeps its files in dir. Nothing is read until a
// channel is first used.
func New(dir string) *Ledger {
	return &Ledger{dir: dir, channels: make(map[int64]*channelState)}
}

// Veto reports, for each entry, whether the ledger forbids skipping it. An entry
// is vetoed only when a record exists for its key with a known modification time
// that differs from the entry's known modification time. A missing record never
// vetoes, and neither does an unknown time, so live state alone decides then.
func (l *Ledger) Veto(channelID int64, parentID string, entries []Entry) []bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.load(channelID)
	vetoed := make([]bool, len(entries))
	for i, entry := range entries {
		key, err := keyFor(parentID, entry)
		if err != nil {
			continue
		}
		known, ok := state.entries[key]
		if ok && known.Modified != 0 && entry.Modified != 0 && known.Modified != entry.Modified {
			vetoed[i] = true
		}
	}
	return vetoed
}

// Record remembers entries as uploaded to parentID on channelID. Call it only
// for files whose staged bytes were confirmed uploaded. The file is saved once
// per call when anything changed, and a call with no entries retries a save
// that failed earlier. A save failure keeps the channel dirty for the next call.
func (l *Ledger) Record(channelID int64, parentID string, entries []Entry) error {
	if channelID == 0 {
		return errors.New("importledger: record needs a channel id")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.load(channelID)
	for _, entry := range entries {
		key, err := keyFor(parentID, entry)
		if err != nil || entry.Modified == 0 {
			continue
		}
		state.seq++
		state.entries[key] = record{Size: entry.Size, Modified: entry.Modified, Seq: state.seq}
		state.dirty = true
	}
	state.evict()
	if !state.dirty {
		return nil
	}
	if err := l.save(channelID, state); err != nil {
		return err
	}
	state.dirty = false
	return nil
}

// Forget drops everything recorded for channelID, in memory and on disk. A
// channel that was never recorded is not an error.
func (l *Ledger) Forget(channelID int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.channels, channelID)
	if err := os.Remove(l.fileFor(channelID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("importledger: forget channel %d: %w", channelID, err)
	}
	return nil
}

// keyFor builds the scoped key for one entry. It rejects names the projection
// would never accept, since those cannot exist on the drive.
func keyFor(parentID string, entry Entry) (string, error) {
	var parts []string
	for _, segment := range strings.Split(entry.FolderPath, "/") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		key, err := projection.CanonicalNameKey(segment)
		if err != nil {
			return "", err
		}
		parts = append(parts, key)
	}
	name, err := projection.CanonicalNameKey(strings.TrimSpace(entry.Name))
	if err != nil {
		return "", err
	}
	parts = append(parts, name)
	enc := "p"
	if entry.Encrypted {
		enc = "e"
	}
	key := parentID + "|" + enc + "|" + strings.Join(parts, "/")
	if len(key) > maxKeyBytes {
		return "", errors.New("importledger: key too long")
	}
	return key, nil
}

// evict drops the oldest records once the channel is over MaxEntries.
func (s *channelState) evict() {
	excess := len(s.entries) - MaxEntries
	if excess <= 0 {
		return
	}
	keys := make([]string, 0, len(s.entries))
	for key := range s.entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return s.entries[keys[i]].Seq < s.entries[keys[j]].Seq
	})
	for _, key := range keys[:excess] {
		delete(s.entries, key)
	}
	s.dirty = true
}

// load returns channelID's state, reading the file on first use. A missing or
// unreadable file starts the channel empty, which only costs a re-upload. The
// caller must hold l.mu.
func (l *Ledger) load(channelID int64) *channelState {
	if state, ok := l.channels[channelID]; ok {
		return state
	}
	state := &channelState{entries: make(map[string]record)}
	if data, err := os.ReadFile(l.fileFor(channelID)); err == nil {
		var file channelFile
		if json.Unmarshal(data, &file) == nil && file.Entries != nil {
			state.entries = file.Entries
			state.seq = file.Seq
		}
	}
	l.channels[channelID] = state
	return state
}

// save replaces the channel's file atomically: a temp file in the same
// directory is fsynced and renamed over the old one, so a crash keeps either
// the previous ledger or the new one. The caller must hold l.mu.
func (l *Ledger) save(channelID int64, state *channelState) error {
	fail := func(err error) error {
		state.dirty = true
		return fmt.Errorf("importledger: write channel %d: %w", channelID, err)
	}
	data, err := json.Marshal(channelFile{Version: 1, Seq: state.seq, Entries: state.entries})
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return fail(err)
	}
	target := l.fileFor(channelID)
	temp, err := os.CreateTemp(l.dir, filepath.Base(target)+".*.tmp")
	if err != nil {
		return fail(err)
	}
	tempName := temp.Name()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return fail(err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return fail(err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return fail(err)
	}
	if err := os.Rename(tempName, target); err != nil {
		_ = os.Remove(tempName)
		return fail(err)
	}
	return nil
}

func (l *Ledger) fileFor(channelID int64) string {
	return filepath.Join(l.dir, fmt.Sprintf("channel-%d.json", channelID))
}
