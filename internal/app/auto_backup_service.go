package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"TDrive/backend/autobackup"
	"TDrive/backend/datadir"
)

// autoBackupSettingsFile holds the device-wide automatic backup choice. It is
// separate from the photo backup ledger on purpose: the ledger is scoped per
// account and drive, while this choice belongs to the phone.
const autoBackupSettingsFile = "auto_backup.json"

// autoBackupFolderTimeout bounds the remote folder work a toggle or rename
// performs. The toggle must answer promptly even when the network is slow.
const autoBackupFolderTimeout = 30 * time.Second

// autoBackupProgressInterval is how often an enabled backup checks its queue
// and emits auto_backup:progress when something changed.
const autoBackupProgressInterval = time.Second

var (
	errAutoBackupOff          = errors.New("auto backup: turned off")
	errAutoBackupCharging     = errors.New("Waiting for the charger.")
	errAutoBackupLowBattery   = errors.New("Paused to save battery. It resumes when the phone is charged or above 15%.")
	errAutoBackupPowerUnknown = errors.New("Waiting for a battery reading from this phone.")
)

// AutoBackupState is what the automatic backup pane renders.
type AutoBackupState struct {
	Enabled           bool     `json:"enabled"`
	FolderName        string   `json:"folder_name"`
	DefaultFolderName string   `json:"default_folder_name"`
	FolderNameCustom  bool     `json:"folder_name_custom"`
	FolderID          string   `json:"folder_id"`
	WiFiOnly          bool     `json:"wifi_only"`
	ChargingOnly      bool     `json:"charging_only"`
	Permission        string   `json:"permission"`
	Phase             string   `json:"phase"`
	PowerBlock        string   `json:"power_block"`
	Platform          string   `json:"platform"`
	Errors            []string `json:"errors"`
}

// AutoBackupProgress is the queue summary the upload tray and the pane show.
// Done and Total count this phone's library items; the current file fields
// describe the upload in flight, if any.
type AutoBackupProgress struct {
	Enabled          bool   `json:"enabled"`
	Done             int64  `json:"done"`
	Total            int64  `json:"total"`
	CurrentName      string `json:"current_name"`
	CurrentBytes     int64  `json:"current_bytes"`
	CurrentBytesDone int64  `json:"current_bytes_done"`
	Paused           bool   `json:"paused"`
	FolderName       string `json:"folder_name"`
}

// autoBackupEngine is the part of the photo backup engine that automatic backup
// drives. The App implements it; tests use a fake.
type autoBackupEngine interface {
	// sync stores the device-library choice for the current drive.
	sync(enabled bool, wifiOnly bool) error
	runNow() error
	pause() error
	resume() error
	// stop cancels uploads in flight without recording a user pause.
	stop()
	progress() (AutoBackupProgress, error)
}

// autoBackupFolderStore is the photo backup folder store plus the rename that
// keeps the existing My Drive folder, and its files, when the name changes.
type autoBackupFolderStore interface {
	photoBackupFolderStore
	rename(ctx context.Context, channelID int64, folderID, name string) error
}

// AutoBackupService owns the phone's automatic backup toggle, its folder and
// its policy. Discovery and upload remain in the photo backup engine, which is
// the single queue; this service decides where those uploads belong, when they
// may run and what the user sees. Its methods are bound to the webview.
type AutoBackupService struct {
	host         serviceHost
	engine       autoBackupEngine
	settingsPath func() (string, error)
	folders      func() (autoBackupFolderStore, error)

	mu           sync.Mutex
	loaded       bool
	settings     autobackup.Settings
	platform     string
	manufact     string
	model        string
	permission   autobackup.Permission
	power        autobackup.PowerState
	lastBlock    autobackup.Block
	lastError    string
	progressOnce sync.Once
	progressLast AutoBackupProgress
}

func newAutoBackupService(host serviceHost, folders func() (autoBackupFolderStore, error)) *AutoBackupService {
	return &AutoBackupService{
		host:    host,
		folders: folders,
		settingsPath: func() (string, error) {
			dir, err := datadir.Dir()
			if err != nil {
				return "", err
			}
			return filepath.Join(dir, autoBackupSettingsFile), nil
		},
		permission: autobackup.PermissionUndetermined,
	}
}

// ensureLoadedLocked reads the settings file once. A corrupt file leaves the
// safe defaults in place and records the problem for the pane.
func (s *AutoBackupService) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	path, err := s.settingsPath()
	if err != nil {
		s.lastError = "Automatic backup settings are unavailable."
		s.settings = autobackup.DefaultSettings()
		return
	}
	value, err := autobackup.LoadSettings(path)
	s.settings = value
	if err != nil {
		s.lastError = "Automatic backup settings could not be read and were reset."
	}
}

func (s *AutoBackupService) effectiveNameLocked() string {
	return autobackup.ResolveFolderName(s.settings.FolderName, s.platform, s.manufact, s.model)
}

func (s *AutoBackupService) stateLocked() AutoBackupState {
	platform := s.platform
	defaultName := autobackup.DeviceFolderName(platform, s.manufact, s.model)
	block := autobackup.GateUpload(s.settings, s.power, time.Now())
	pending := s.progressLast.Total - s.progressLast.Done
	if pending < 0 {
		pending = 0
	}
	phase := autobackup.DecidePhase(autobackup.PhaseInput{
		Settings:     s.settings,
		Permission:   s.permission,
		Paused:       s.progressLast.Paused,
		PendingItems: pending,
		PowerBlock:   block,
	})
	state := AutoBackupState{
		Enabled:           s.settings.Enabled,
		FolderName:        s.effectiveNameLocked(),
		DefaultFolderName: defaultName,
		FolderNameCustom:  s.settings.TrimmedFolderName() != "",
		FolderID:          s.settings.FolderID,
		WiFiOnly:          s.settings.WiFiOnly,
		ChargingOnly:      s.settings.ChargingOnly,
		Permission:        string(s.permission),
		Phase:             string(phase),
		PowerBlock:        string(block),
		Platform:          platform,
		Errors:            []string{},
	}
	if s.lastError != "" {
		state.Errors = append(state.Errors, s.lastError)
	}
	return state
}

// GetState reports the current toggle, folder, policy, permission and phase.
func (s *AutoBackupService) GetState() (AutoBackupState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return s.stateLocked(), nil
}

// SetDeviceIdentity records what the native host reports about this phone. It
// only decides the default folder name; nothing about the drive is read from it.
func (s *AutoBackupService) SetDeviceIdentity(platform, manufacturer, model string) (AutoBackupState, error) {
	s.mu.Lock()
	s.ensureLoadedLocked()
	s.platform = strings.ToLower(strings.TrimSpace(platform))
	s.manufact = strings.TrimSpace(manufacturer)
	s.model = strings.TrimSpace(model)
	state := s.stateLocked()
	s.mu.Unlock()
	return state, nil
}

// SetPermission records the media-library grant the native host reported.
func (s *AutoBackupService) SetPermission(status string) (AutoBackupState, error) {
	s.mu.Lock()
	s.ensureLoadedLocked()
	switch autobackup.Permission(strings.ToLower(strings.TrimSpace(status))) {
	case autobackup.PermissionGranted, autobackup.PermissionLimited, autobackup.PermissionDenied:
		s.permission = autobackup.Permission(strings.ToLower(strings.TrimSpace(status)))
	default:
		s.permission = autobackup.PermissionUndetermined
	}
	state := s.stateLocked()
	s.mu.Unlock()
	return state, nil
}

// SetPowerState records one native battery reading. The host pushes these
// regularly while the pane is alive; a reading goes stale after two minutes
// and then counts as unknown. Level is 0 to 100, or negative when unknown.
func (s *AutoBackupService) SetPowerState(charging bool, levelPercent int, lowPowerMode bool) (AutoBackupState, error) {
	if levelPercent > 100 {
		levelPercent = 100
	}
	if levelPercent < 0 {
		levelPercent = -1
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	now := time.Now()
	s.power = autobackup.PowerState{Known: true, Charging: charging, LevelPercent: levelPercent, LowPowerMode: lowPowerMode, ObservedAt: now}
	enabled := s.settings.Enabled
	block := autobackup.GateUpload(s.settings, s.power, now)
	previous := s.lastBlock
	s.lastBlock = block
	s.mu.Unlock()
	if enabled && s.engine != nil {
		// Stop uploads the moment a rule blocks them, and restart them the
		// moment every rule allows again. Transitions only: a steady state
		// does not restart a run on every reading.
		switch {
		case block != autobackup.BlockNone:
			s.engine.stop()
		case previous != autobackup.BlockNone:
			if err := s.engine.runNow(); err != nil {
				slog.Debug("auto backup: restart after power change", "reason", err)
			}
		}
	}
	return s.GetState()
}

// SetEnabled turns automatic backup on or off. Turning it on makes the folder
// in My Drive, idempotently, and starts the device-library queue; turning it off
// stops uploads and keeps the folder and its files.
func (s *AutoBackupService) SetEnabled(enabled bool) (AutoBackupState, error) {
	s.mu.Lock()
	s.ensureLoadedLocked()
	next := s.settings
	next.Enabled = enabled
	if err := s.saveLocked(next); err != nil {
		s.mu.Unlock()
		return AutoBackupState{}, err
	}
	s.mu.Unlock()
	if enabled {
		if err := s.ensureFolder(false); err != nil {
			s.recordError(err)
		}
		s.startProgressLoop()
	}
	s.syncEngine()
	s.emitState()
	return s.GetState()
}

// SetFolderName renames the destination folder. An empty value restores the
// device default. The existing My Drive folder is renamed in place, so the
// files already in it stay where they are.
func (s *AutoBackupService) SetFolderName(name string) (AutoBackupState, error) {
	trimmed := strings.TrimSpace(name)
	var clean string
	if trimmed != "" {
		sanitized, err := autobackup.SanitizeFolderName(trimmed)
		if err != nil {
			return AutoBackupState{}, errors.New("Folder names cannot be empty, contain / \\ : * ? \" < > | or be longer than 200 bytes.")
		}
		clean = sanitized
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	next := s.settings
	next.FolderName = clean
	if err := s.saveLocked(next); err != nil {
		s.mu.Unlock()
		return AutoBackupState{}, err
	}
	enabled := next.Enabled
	s.mu.Unlock()
	if enabled {
		if err := s.ensureFolder(true); err != nil {
			s.recordError(err)
		}
	}
	s.emitState()
	return s.GetState()
}

// SetWifiOnly chooses whether automatic uploads wait for Wi-Fi.
func (s *AutoBackupService) SetWifiOnly(value bool) (AutoBackupState, error) {
	return s.updatePolicy(func(settings *autobackup.Settings) { settings.WiFiOnly = value })
}

// SetChargingOnly chooses whether automatic uploads wait for the charger. The
// battery reading that enforces it comes from SetPowerState.
func (s *AutoBackupService) SetChargingOnly(value bool) (AutoBackupState, error) {
	return s.updatePolicy(func(settings *autobackup.Settings) { settings.ChargingOnly = value })
}

func (s *AutoBackupService) updatePolicy(change func(*autobackup.Settings)) (AutoBackupState, error) {
	s.mu.Lock()
	s.ensureLoadedLocked()
	next := s.settings
	change(&next)
	if err := s.saveLocked(next); err != nil {
		s.mu.Unlock()
		return AutoBackupState{}, err
	}
	s.mu.Unlock()
	s.syncEngine()
	s.emitState()
	return s.GetState()
}

// BackUpNow starts an upload run now, unless the phone's rules currently
// block it or the user paused backup.
func (s *AutoBackupService) BackUpNow() (AutoBackupState, error) {
	if !s.isEnabled() {
		return AutoBackupState{}, errors.New("Turn on automatic backup first.")
	}
	if s.engine == nil {
		return AutoBackupState{}, errBackendUnavailable
	}
	if err := s.engine.runNow(); err != nil {
		return AutoBackupState{}, err
	}
	s.emitState()
	return s.GetState()
}

// PauseBackup stops uploads and keeps them stopped until ResumeBackup. It
// uses the photo backup pause, so it survives restarts.
func (s *AutoBackupService) PauseBackup() (AutoBackupState, error) {
	if s.engine == nil {
		return AutoBackupState{}, errBackendUnavailable
	}
	if err := s.engine.pause(); err != nil {
		return AutoBackupState{}, err
	}
	s.emitState()
	return s.GetState()
}

// ResumeBackup clears a user pause and starts uploads again if the rules allow.
func (s *AutoBackupService) ResumeBackup() (AutoBackupState, error) {
	if s.engine == nil {
		return AutoBackupState{}, errBackendUnavailable
	}
	if err := s.engine.resume(); err != nil {
		return AutoBackupState{}, err
	}
	s.emitState()
	return s.GetState()
}

// GetProgress reports the queue summary now.
func (s *AutoBackupService) GetProgress() (AutoBackupProgress, error) {
	return s.progressSnapshot()
}

func (s *AutoBackupService) isEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return s.settings.Enabled
}

// saveLocked persists value and only then makes it the live setting, so a
// failed write never leaves the pane showing a choice that was not kept.
func (s *AutoBackupService) saveLocked(value autobackup.Settings) error {
	path, err := s.settingsPath()
	if err != nil {
		return fmt.Errorf("auto backup: settings location: %w", err)
	}
	if err := autobackup.SaveSettings(path, value); err != nil {
		return err
	}
	s.settings = value
	s.lastError = ""
	return nil
}

// syncEngine pushes the enabled and Wi-Fi choices into the photo backup
// engine, which owns the queue. It runs outside s.mu because the engine takes
// its own locks and calls back into the policy gate.
func (s *AutoBackupService) syncEngine() {
	if s.engine == nil {
		return
	}
	s.mu.Lock()
	enabled, wifi := s.settings.Enabled, s.settings.WiFiOnly
	s.mu.Unlock()
	if err := s.engine.sync(enabled, wifi); err != nil {
		s.recordMessage("Automatic backup could not update this drive's photo settings. It will try again when you change the setting.")
		slog.Debug("auto backup: sync engine", "reason", err)
	}
}

// ensureFolder makes sure the destination folder exists in My Drive. When
// renameExisting is set, the folder this service already made is renamed to
// the current name instead of a second one being created.
func (s *AutoBackupService) ensureFolder(renameExisting bool) error {
	s.mu.Lock()
	s.ensureLoadedLocked()
	name := s.effectiveNameLocked()
	currentID := s.settings.FolderID
	s.mu.Unlock()
	name = photoBackupFolderName(name, autobackup.DefaultFolderName)

	channelID := s.host.activeChannelID()
	if channelID <= 0 || s.folders == nil {
		return errBackendUnavailable
	}
	store, err := s.folders()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.host.appContext(), autoBackupFolderTimeout)
	defer cancel()

	photoBackupDestinationMu.Lock()
	id, err := placeAutoBackupFolder(ctx, store, channelID, name, currentID, renameExisting)
	photoBackupDestinationMu.Unlock()
	if err != nil {
		return fmt.Errorf("auto backup: prepare folder %q: %w", name, err)
	}
	return s.rememberFolderID(id)
}

// destinationFolderID is the upload-side lookup. The caller holds
// photoBackupDestinationMu. It never renames: uploads find the folder by name.
func (s *AutoBackupService) destinationFolderID(ctx context.Context, store autoBackupFolderStore, channelID int64) (string, error) {
	s.mu.Lock()
	s.ensureLoadedLocked()
	if !s.settings.Enabled {
		s.mu.Unlock()
		return "", errAutoBackupOff
	}
	name := photoBackupFolderName(s.effectiveNameLocked(), autobackup.DefaultFolderName)
	currentID := s.settings.FolderID
	s.mu.Unlock()
	id, err := placeAutoBackupFolder(ctx, store, channelID, name, currentID, false)
	if err != nil {
		return "", err
	}
	if err := s.rememberFolderID(id); err != nil {
		slog.Debug("auto backup: remember folder id", "reason", err)
	}
	return id, nil
}

// placeAutoBackupFolder returns the folder for name. With renameExisting set
// and a folder already known, that folder is renamed, which keeps its files.
// Otherwise, or when the rename is refused because the folder is gone or the
// name is taken, the folder is found or created by name, idempotently.
func placeAutoBackupFolder(ctx context.Context, store autoBackupFolderStore, channelID int64, name, currentID string, renameExisting bool) (string, error) {
	if renameExisting && currentID != "" {
		if err := store.rename(ctx, channelID, currentID, name); err == nil {
			return currentID, nil
		}
	}
	return autobackup.EnsureFolder(ctx, autoBackupFolderAdapter{store: store, channelID: channelID}, "", name)
}

// rememberFolderID stores the folder id so a later rename can find the folder.
func (s *AutoBackupService) rememberFolderID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settings.FolderID == id {
		return nil
	}
	next := s.settings
	next.FolderID = id
	return s.saveLocked(next)
}

// uploadGateError is the policy check the photo backup engine runs before each
// upload. It is nil when automatic backup is off or the phone's rules allow it.
func (s *AutoBackupService) uploadGateError(now time.Time) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if !s.settings.Enabled {
		return nil
	}
	switch autobackup.GateUpload(s.settings, s.power, now) {
	case autobackup.BlockCharging:
		return errAutoBackupCharging
	case autobackup.BlockLowBattery:
		return errAutoBackupLowBattery
	case autobackup.BlockPowerUnknown:
		return errAutoBackupPowerUnknown
	}
	return nil
}

func (s *AutoBackupService) recordError(err error) {
	message := "Automatic backup could not prepare its folder. It will try again when you turn it on or rename it."
	if errors.Is(err, errBackendUnavailable) {
		message = "Automatic backup is waiting for the drive to finish starting."
	}
	s.recordMessage(message)
}

// recordMessage keeps the first problem, so a later, less specific one does
// not hide what the user needs to act on.
func (s *AutoBackupService) recordMessage(message string) {
	s.mu.Lock()
	if s.lastError == "" {
		s.lastError = message
	}
	s.mu.Unlock()
}

func (s *AutoBackupService) emitState() {
	state, err := s.GetState()
	if err != nil {
		return
	}
	s.host.emit("auto_backup:state", state)
}

// progressSnapshot reads the queue summary from the engine.
func (s *AutoBackupService) progressSnapshot() (AutoBackupProgress, error) {
	s.mu.Lock()
	s.ensureLoadedLocked()
	enabled := s.settings.Enabled
	name := s.effectiveNameLocked()
	s.mu.Unlock()
	if s.engine == nil {
		return AutoBackupProgress{Enabled: enabled, FolderName: name}, nil
	}
	progress, err := s.engine.progress()
	if err != nil {
		return AutoBackupProgress{}, err
	}
	progress.Enabled = enabled
	progress.FolderName = name
	return progress, nil
}

// startProgressLoop starts the ticker that emits auto_backup:progress. It runs
// once per service and ends with the app context.
func (s *AutoBackupService) startProgressLoop() {
	s.progressOnce.Do(func() {
		ctx := s.host.appContext()
		go func() {
			ticker := time.NewTicker(autoBackupProgressInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.publishProgress()
				}
			}
		}()
	})
}

func (s *AutoBackupService) publishProgress() {
	if s.engine == nil || !s.isEnabled() {
		return
	}
	progress, err := s.progressSnapshot()
	if err != nil {
		return
	}
	s.mu.Lock()
	changed := progress != s.progressLast
	s.progressLast = progress
	s.mu.Unlock()
	if changed {
		s.host.emit("auto_backup:progress", progress)
	}
}

// autoBackupFolderAdapter presents the folder store as the narrow interface the
// pure EnsureFolder needs, bound to one drive.
type autoBackupFolderAdapter struct {
	store     photoBackupFolderStore
	channelID int64
}

func (a autoBackupFolderAdapter) Find(ctx context.Context, parentID, name string) (string, bool, error) {
	folder, found, err := a.store.find(ctx, a.channelID, parentID, name)
	return folder.ID, found, err
}

func (a autoBackupFolderAdapter) Create(ctx context.Context, parentID, name string) (string, error) {
	folder, err := a.store.create(ctx, a.channelID, parentID, name)
	return folder.ID, err
}
