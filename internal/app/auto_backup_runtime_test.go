package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"TDrive/backend/core"
)

// liveAutoBackupHost is a serviceHost with a drive, so folder work runs against
// the fake store. Events are recorded under a lock because the progress loop
// may emit from another goroutine.
type liveAutoBackupHost struct {
	mu     sync.Mutex
	events []string
}

func (h *liveAutoBackupHost) coreEngine() *core.Engine                  { return nil }
func (h *liveAutoBackupHost) WaitForBackendReady(context.Context) error { return nil }
func (h *liveAutoBackupHost) activeChannelID() int64                    { return 7 }
func (h *liveAutoBackupHost) appContext() context.Context               { return context.Background() }
func (h *liveAutoBackupHost) emit(name string, _ ...any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, name)
}

func (h *liveAutoBackupHost) count(name string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, event := range h.events {
		if event == name {
			n++
		}
	}
	return n
}

// fakeAutoBackupFolders is an in-memory My Drive folder store.
type fakeAutoBackupFolders struct {
	byID    map[string]fakeFolder
	created []string
	renamed []string
}

type fakeFolder struct{ parent, name string }

func newFakeAutoBackupFolders() *fakeAutoBackupFolders {
	return &fakeAutoBackupFolders{byID: map[string]fakeFolder{}}
}

func (f *fakeAutoBackupFolders) find(_ context.Context, _ int64, parentID, name string) (photoBackupFolder, bool, error) {
	for id, folder := range f.byID {
		if folder.parent == parentID && folder.name == name {
			return photoBackupFolder{ID: id, ParentID: parentID, Name: name}, true, nil
		}
	}
	return photoBackupFolder{}, false, nil
}

func (f *fakeAutoBackupFolders) create(_ context.Context, _ int64, parentID, name string) (photoBackupFolder, error) {
	id := fmt.Sprintf("new-%d", len(f.created)+1)
	f.byID[id] = fakeFolder{parent: parentID, name: name}
	f.created = append(f.created, name)
	return photoBackupFolder{ID: id, ParentID: parentID, Name: name}, nil
}

func (f *fakeAutoBackupFolders) rename(_ context.Context, _ int64, id, name string) error {
	folder, ok := f.byID[id]
	if !ok {
		return errors.New("folder not found")
	}
	folder.name = name
	f.byID[id] = folder
	f.renamed = append(f.renamed, name)
	return nil
}

// fakeAutoBackupEngine records the calls automatic backup makes into the
// photo backup engine.
type fakeAutoBackupEngine struct {
	syncs    []string
	runs     int
	pauses   int
	resumes  int
	stops    int
	queue    AutoBackupProgress
}

func (e *fakeAutoBackupEngine) sync(enabled bool, wifiOnly bool) error {
	e.syncs = append(e.syncs, fmt.Sprintf("%v/%v", enabled, wifiOnly))
	return nil
}
func (e *fakeAutoBackupEngine) runNow() error { e.runs++; return nil }
func (e *fakeAutoBackupEngine) pause() error  { e.pauses++; return nil }
func (e *fakeAutoBackupEngine) resume() error { e.resumes++; return nil }
func (e *fakeAutoBackupEngine) stop()         { e.stops++ }
func (e *fakeAutoBackupEngine) progress() (AutoBackupProgress, error) {
	return e.queue, nil
}

func newLiveAutoBackupService(t *testing.T, folders *fakeAutoBackupFolders, engine *fakeAutoBackupEngine, host *liveAutoBackupHost, enabled bool) *AutoBackupService {
	t.Helper()
	service := newAutoBackupService(host, func() (autoBackupFolderStore, error) { return folders, nil })
	service.settingsPath = func() (string, error) { return filepath.Join(t.TempDir(), "auto_backup.json"), nil }
	service.engine = engine
	service.mu.Lock()
	service.ensureLoadedLocked()
	service.settings.Enabled = enabled
	service.platform, service.manufact, service.model = "ios", "Apple", "iPhone"
	service.mu.Unlock()
	return service
}

func TestAutoBackupRenameKeepsExistingFolder(t *testing.T) {
	folders := newFakeAutoBackupFolders()
	engine := &fakeAutoBackupEngine{}
	host := &liveAutoBackupHost{}
	service := newLiveAutoBackupService(t, folders, engine, host, true)
	first, err := service.destinationFolderID(context.Background(), folders, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders.created) != 1 || folders.created[0] != "iPhone" {
		t.Fatalf("created = %v, want one iPhone folder", folders.created)
	}
	state, err := service.SetFolderName("Office")
	if err != nil {
		t.Fatal(err)
	}
	if len(folders.created) != 1 {
		t.Fatalf("rename created a folder: %v", folders.created)
	}
	if len(folders.renamed) != 1 || folders.renamed[0] != "Office" {
		t.Fatalf("renamed = %v", folders.renamed)
	}
	if state.FolderID != first || folders.byID[first].name != "Office" {
		t.Fatalf("folder %q not renamed in place: state=%+v", first, state)
	}
}

func TestAutoBackupDestinationFindsFolderAndRemembersID(t *testing.T) {
	folders := newFakeAutoBackupFolders()
	folders.byID["existing"] = fakeFolder{parent: "", name: "iPhone"}
	engine := &fakeAutoBackupEngine{}
	service := newLiveAutoBackupService(t, folders, engine, &liveAutoBackupHost{}, true)
	id, err := service.destinationFolderID(context.Background(), folders, 7)
	if err != nil {
		t.Fatal(err)
	}
	if id != "existing" || len(folders.created) != 0 {
		t.Fatalf("id = %q, created = %v", id, folders.created)
	}
	if service.settings.FolderID != "existing" {
		t.Fatalf("folder id not remembered: %+v", service.settings)
	}
}

func TestAutoBackupDestinationIsOffWhenDisabled(t *testing.T) {
	folders := newFakeAutoBackupFolders()
	service := newLiveAutoBackupService(t, folders, &fakeAutoBackupEngine{}, &liveAutoBackupHost{}, false)
	if _, err := service.destinationFolderID(context.Background(), folders, 7); !errors.Is(err, errAutoBackupOff) {
		t.Fatalf("err = %v, want off", err)
	}
	if len(folders.created) != 0 {
		t.Fatalf("created folders while off: %v", folders.created)
	}
}

func TestAutoBackupChargingOnlyFailsClosedUntilCharging(t *testing.T) {
	folders := newFakeAutoBackupFolders()
	engine := &fakeAutoBackupEngine{}
	service := newLiveAutoBackupService(t, folders, engine, &liveAutoBackupHost{}, true)
	service.mu.Lock()
	service.settings.ChargingOnly = true
	service.mu.Unlock()

	if err := service.uploadGateError(time.Now()); !errors.Is(err, errAutoBackupPowerUnknown) {
		t.Fatalf("no reading: err = %v", err)
	}
	if _, err := service.SetPowerState(false, 80, false); err != nil {
		t.Fatal(err)
	}
	if err := service.uploadGateError(time.Now()); !errors.Is(err, errAutoBackupCharging) {
		t.Fatalf("on battery: err = %v", err)
	}
	if engine.stops != 1 {
		t.Fatalf("stops = %d, want uploads stopped on the charger rule", engine.stops)
	}
	if _, err := service.SetPowerState(true, 80, false); err != nil {
		t.Fatal(err)
	}
	if err := service.uploadGateError(time.Now()); err != nil {
		t.Fatalf("charging: err = %v", err)
	}
	if engine.runs != 1 {
		t.Fatalf("runs = %d, want a restart once charging", engine.runs)
	}
}

func TestAutoBackupLowBatteryStopsWithoutChargingRule(t *testing.T) {
	folders := newFakeAutoBackupFolders()
	service := newLiveAutoBackupService(t, folders, &fakeAutoBackupEngine{}, &liveAutoBackupHost{}, true)
	if _, err := service.SetPowerState(false, 15, false); err != nil {
		t.Fatal(err)
	}
	if err := service.uploadGateError(time.Now()); !errors.Is(err, errAutoBackupLowBattery) {
		t.Fatalf("15%% unplugged: err = %v", err)
	}
	if _, err := service.SetPowerState(false, 60, true); err != nil {
		t.Fatal(err)
	}
	if err := service.uploadGateError(time.Now()); !errors.Is(err, errAutoBackupLowBattery) {
		t.Fatalf("low power mode: err = %v", err)
	}
}

func TestAutoBackupPublishesProgressOnlyOnChange(t *testing.T) {
	folders := newFakeAutoBackupFolders()
	engine := &fakeAutoBackupEngine{queue: AutoBackupProgress{Done: 1, Total: 3, CurrentName: "a.jpg"}}
	host := &liveAutoBackupHost{}
	service := newLiveAutoBackupService(t, folders, engine, host, true)
	service.publishProgress()
	service.publishProgress()
	if got := host.count("auto_backup:progress"); got != 1 {
		t.Fatalf("progress events = %d, want 1 for an unchanged queue", got)
	}
	engine.queue.Done = 2
	service.publishProgress()
	if got := host.count("auto_backup:progress"); got != 2 {
		t.Fatalf("progress events = %d, want 2 after a change", got)
	}
	progress, err := service.GetProgress()
	if err != nil {
		t.Fatal(err)
	}
	if !progress.Enabled || progress.Done != 2 || progress.Total != 3 || progress.FolderName != "iPhone" {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestAutoBackupBackUpNowNeedsTurnedOn(t *testing.T) {
	folders := newFakeAutoBackupFolders()
	engine := &fakeAutoBackupEngine{}
	off := newLiveAutoBackupService(t, folders, engine, &liveAutoBackupHost{}, false)
	if _, err := off.BackUpNow(); err == nil {
		t.Fatal("BackUpNow while off succeeded")
	}
	if engine.runs != 0 {
		t.Fatalf("runs = %d while off", engine.runs)
	}
	on := newLiveAutoBackupService(t, folders, engine, &liveAutoBackupHost{}, true)
	if _, err := on.BackUpNow(); err != nil {
		t.Fatal(err)
	}
	if _, err := on.PauseBackup(); err != nil {
		t.Fatal(err)
	}
	if _, err := on.ResumeBackup(); err != nil {
		t.Fatal(err)
	}
	if engine.runs != 1 || engine.pauses != 1 || engine.resumes != 1 {
		t.Fatalf("runs=%d pauses=%d resumes=%d", engine.runs, engine.pauses, engine.resumes)
	}
}

func TestAutoBackupPolicyChangesReachTheEngine(t *testing.T) {
	folders := newFakeAutoBackupFolders()
	engine := &fakeAutoBackupEngine{}
	service := newLiveAutoBackupService(t, folders, engine, &liveAutoBackupHost{}, true)
	if _, err := service.SetWifiOnly(false); err != nil {
		t.Fatal(err)
	}
	if len(engine.syncs) != 1 || engine.syncs[0] != "true/false" {
		t.Fatalf("syncs = %v, want the enabled and Wi-Fi choice pushed", engine.syncs)
	}
}
