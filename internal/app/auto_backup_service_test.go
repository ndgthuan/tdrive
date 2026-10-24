package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"TDrive/backend/core"
)

// autoBackupTestHost is a serviceHost with no drive, so ensureFolder reports
// the backend as unavailable without reaching for real folder storage.
type autoBackupTestHost struct {
	events []string
}

func (h *autoBackupTestHost) coreEngine() *core.Engine                  { return nil }
func (h *autoBackupTestHost) WaitForBackendReady(context.Context) error { return nil }
func (h *autoBackupTestHost) activeChannelID() int64                    { return 0 }
func (h *autoBackupTestHost) appContext() context.Context               { return context.Background() }
func (h *autoBackupTestHost) emit(name string, _ ...any)                { h.events = append(h.events, name) }

func newAutoBackupTestService(t *testing.T, path string) *AutoBackupService {
	t.Helper()
	host := &autoBackupTestHost{}
	service := newAutoBackupService(host, func() (autoBackupFolderStore, error) { return nil, errBackendUnavailable })
	service.settingsPath = func() (string, error) { return path, nil }
	return service
}

func TestAutoBackupDefaultsAreOffWithWiFiOnly(t *testing.T) {
	service := newAutoBackupTestService(t, filepath.Join(t.TempDir(), "auto_backup.json"))
	state, err := service.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Enabled || !state.WiFiOnly || state.ChargingOnly {
		t.Fatalf("defaults = %+v", state)
	}
	if state.Phase != "off" || state.Permission != "undetermined" || state.FolderName != "Phone" {
		t.Fatalf("defaults = %+v", state)
	}
}

func TestAutoBackupDeviceIdentityChoosesFolderName(t *testing.T) {
	service := newAutoBackupTestService(t, filepath.Join(t.TempDir(), "auto_backup.json"))
	state, err := service.SetDeviceIdentity("ios", "Apple", "iPad")
	if err != nil {
		t.Fatal(err)
	}
	if state.FolderName != "iPad" || state.DefaultFolderName != "iPad" || state.FolderNameCustom {
		t.Fatalf("state = %+v", state)
	}
	state, err = service.SetDeviceIdentity("android", "samsung", "SM-S928B")
	if err != nil {
		t.Fatal(err)
	}
	if state.FolderName != "Samsung" {
		t.Fatalf("android folder = %q", state.FolderName)
	}
}

func TestAutoBackupEnableWithoutDriveKeepsSettingAndReportsWaiting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auto_backup.json")
	service := newAutoBackupTestService(t, path)
	state, err := service.SetEnabled(true)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled {
		t.Fatalf("enabled not kept: %+v", state)
	}
	if len(state.Errors) != 1 || !strings.Contains(state.Errors[0], "starting") {
		t.Fatalf("errors = %v", state.Errors)
	}
	reloaded := newAutoBackupTestService(t, path)
	again, err := reloaded.GetState()
	if err != nil || !again.Enabled {
		t.Fatalf("persisted state = %+v, %v", again, err)
	}
}

func TestAutoBackupRejectsInvalidRenameWithoutChange(t *testing.T) {
	service := newAutoBackupTestService(t, filepath.Join(t.TempDir(), "auto_backup.json"))
	if _, err := service.SetFolderName("bad/name"); err == nil {
		t.Fatal("expected rename error")
	}
	state, _ := service.GetState()
	if state.FolderNameCustom {
		t.Fatalf("invalid rename changed the folder: %+v", state)
	}
}

func TestAutoBackupRenameAndPolicyPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auto_backup.json")
	service := newAutoBackupTestService(t, path)
	state, err := service.SetFolderName("  Work phone  ")
	if err != nil {
		t.Fatal(err)
	}
	if state.FolderName != "Work phone" || !state.FolderNameCustom {
		t.Fatalf("rename = %+v", state)
	}
	if _, err := service.SetWifiOnly(false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetChargingOnly(true); err != nil {
		t.Fatal(err)
	}
	reloaded := newAutoBackupTestService(t, path)
	again, _ := reloaded.GetState()
	if again.FolderName != "Work phone" || again.WiFiOnly || !again.ChargingOnly {
		t.Fatalf("reloaded = %+v", again)
	}
	if _, err := service.SetFolderName(""); err != nil {
		t.Fatal(err)
	}
	cleared, _ := service.GetState()
	if cleared.FolderNameCustom || cleared.FolderName != "Phone" {
		t.Fatalf("empty rename must restore the default: %+v", cleared)
	}
}

func TestAutoBackupPermissionDeniedBlocksPhase(t *testing.T) {
	service := newAutoBackupTestService(t, filepath.Join(t.TempDir(), "auto_backup.json"))
	if _, err := service.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	state, err := service.SetPermission("denied")
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != "needs_permission" || state.Permission != "denied" {
		t.Fatalf("state = %+v", state)
	}
	state, _ = service.SetPermission("garbage")
	if state.Permission != "undetermined" {
		t.Fatalf("unknown permission must be undetermined, got %q", state.Permission)
	}
}

func TestAutoBackupCorruptSettingsLoadSafely(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auto_backup.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := newAutoBackupTestService(t, path)
	state, err := service.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Enabled || !state.WiFiOnly || len(state.Errors) == 0 {
		t.Fatalf("corrupt file state = %+v", state)
	}
}

func TestAutoBackupStateEventsEmitted(t *testing.T) {
	host := &autoBackupTestHost{}
	service := newAutoBackupService(host, func() (autoBackupFolderStore, error) { return nil, errBackendUnavailable })
	service.settingsPath = func() (string, error) { return filepath.Join(t.TempDir(), "s.json"), nil }
	if _, err := service.SetWifiOnly(false); err != nil {
		t.Fatal(err)
	}
	if len(host.events) != 1 || host.events[0] != "auto_backup:state" {
		t.Fatalf("events = %v", host.events)
	}
}
