package app

import (
	"context"

	"TDrive/backend/photobackup"
)

// photoBackupDeviceLibraryRoot is the fixed root of the whole-library source.
// The phone's media is read through the native bridge, so the root is a name,
// not a path; the client cannot choose it.
const photoBackupDeviceLibraryRoot = "device:library"

// photoBackupDeviceLibraryName is the source's display name in the ledger.
const photoBackupDeviceLibraryName = "Phone library"

// appAutoBackupEngine drives the photo backup engine for the device-library
// source. It holds no state of its own: every call reads the ledger.
type appAutoBackupEngine struct{ app *App }

func (e appAutoBackupEngine) sync(enabled bool, wifiOnly bool) error {
	a := e.app
	ctx := a.appContext()
	engine, err := a.photoBackupEngine()
	if err != nil {
		return err
	}
	scope, err := a.photoBackupScope(ctx)
	if err != nil {
		return err
	}
	settings, err := engine.GetSettings(ctx, scope)
	if err != nil {
		return err
	}
	next := settings
	next.Enabled = enabled
	next.Photos = true
	next.Videos = true
	next.WiFiOnly = wifiOnly
	if err := engine.PutSettings(ctx, next); err != nil {
		return err
	}
	source := photobackup.Source{
		Scope:   scope,
		ID:      photobackup.DeviceLibrarySourceID,
		Kind:    photobackup.DeviceLibrarySourceKind,
		Root:    photoBackupDeviceLibraryRoot,
		Name:    photoBackupDeviceLibraryName,
		Enabled: enabled,
	}
	if err := engine.UpsertSource(ctx, source); err != nil {
		return err
	}
	if !enabled {
		a.stopPhotoBackup()
	}
	return nil
}

func (e appAutoBackupEngine) runNow() error  { return e.app.runPhotoBackup() }
func (e appAutoBackupEngine) pause() error   { return e.app.pausePhotoBackup() }
func (e appAutoBackupEngine) resume() error  { return e.app.resumePhotoBackup() }
func (e appAutoBackupEngine) stop()          { e.app.stopPhotoBackup() }

func (e appAutoBackupEngine) progress() (AutoBackupProgress, error) {
	a := e.app
	ctx := a.appContext()
	engine, err := a.photoBackupEngine()
	if err != nil {
		return AutoBackupProgress{}, err
	}
	scope, err := a.photoBackupScope(ctx)
	if err != nil {
		return AutoBackupProgress{}, err
	}
	status, err := engine.Status(ctx, scope)
	if err != nil {
		return AutoBackupProgress{}, err
	}
	settings, err := engine.GetSettings(ctx, scope)
	if err != nil {
		return AutoBackupProgress{}, err
	}
	current := a.withPhotoBackupProgress(PhotoBackupState{}, scope).Status
	return AutoBackupProgress{
		Done:             status.Complete,
		Total:            status.Pending + status.Uploading + status.Complete,
		CurrentName:      current.CurrentFile,
		CurrentBytes:     current.CurrentFileBytesTotal,
		CurrentBytesDone: current.CurrentFileBytesDone,
		Paused:           settings.ManualPaused,
	}, nil
}

// appAutoBackupFolderStore is the folder store automatic backup uses, built
// from the app folder service.
type appAutoBackupFolderStore struct{ appPhotoBackupFolderStore }

func (s appAutoBackupFolderStore) rename(ctx context.Context, channelID int64, folderID, name string) error {
	if s.service == nil {
		return errBackendUnavailable
	}
	return s.service.RenameContext(ctx, channelID, folderID, name)
}
