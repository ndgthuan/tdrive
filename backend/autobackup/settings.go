package autobackup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidFolderName reports a rename the drive or the folder rules refuse.
var ErrInvalidFolderName = errors.New("autobackup: invalid folder name")

// Permission is the media-library grant as the native host reports it.
type Permission string

const (
	PermissionUndetermined Permission = "undetermined"
	PermissionGranted      Permission = "granted"
	// PermissionLimited means the user chose only some items. Backup may run
	// over those items, and the pane says so.
	PermissionLimited Permission = "limited"
	PermissionDenied  Permission = "denied"
)

// Settings is the persisted automatic-backup choice. It is device-wide, not
// per account: the phone is one device, and the destination it writes to is
// resolved from the signed-in drive when backup runs.
type Settings struct {
	Enabled bool `json:"enabled"`
	// FolderName is the user's rename. Empty means the device default.
	FolderName string `json:"folder_name"`
	// WiFiOnly defaults to true. A zero value in an older file must not
	// silently switch cellular uploads on, so Load applies this default
	// before decoding.
	WiFiOnly bool `json:"wifi_only"`
	// ChargingOnly defaults to false.
	ChargingOnly bool `json:"charging_only"`
	// FolderID is the My Drive folder the backup writes to. It is kept so that a
	// rename moves this folder rather than creating a second one.
	FolderID string `json:"folder_id,omitempty"`
}

// DefaultSettings is what a fresh install starts with.
func DefaultSettings() Settings {
	return Settings{WiFiOnly: true}
}

// LoadSettings reads the settings file. A missing file yields the defaults. A
// corrupt file also yields the defaults with an error, so the caller can log
// it while still starting in a safe, off state.
func LoadSettings(path string) (Settings, error) {
	value := DefaultSettings()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return DefaultSettings(), fmt.Errorf("autobackup: read settings: %w", err)
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return DefaultSettings(), fmt.Errorf("autobackup: decode settings: %w", err)
	}
	return value, nil
}

// SaveSettings writes the file atomically: a temp file in the same directory
// is renamed over the target, so a crash mid-write keeps the old file.
func SaveSettings(path string, value Settings) error {
	if value.FolderName != "" {
		name, err := SanitizeFolderName(value.FolderName)
		if err != nil {
			return err
		}
		value.FolderName = name
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("autobackup: encode settings: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("autobackup: settings directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("autobackup: write settings: %w", err)
	}
	tempName := temp.Name()
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return fmt.Errorf("autobackup: write settings: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("autobackup: write settings: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("autobackup: replace settings: %w", err)
	}
	return nil
}

// TrimmedFolderName reports the stored rename without surrounding space, for
// callers that compare names.
func (s Settings) TrimmedFolderName() string {
	return strings.TrimSpace(s.FolderName)
}
