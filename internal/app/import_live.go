package app

import (
	"context"
	"fmt"

	folderservice "TDrive/backend/services/folder"
)

// requireImportDrive checks that the drive an import started on is still the
// active one. Every live import call names that drive so a switch in the middle
// of a job stops the job instead of writing into the wrong drive.
func (a *App) requireImportDrive(driveID int64) (*folderservice.Service, error) {
	if driveID == 0 || a.ActiveChannelID() != driveID {
		return nil, fmt.Errorf("the active drive changed, so the import was stopped")
	}
	return a.requireFolderService()
}

// ImportResolveFolders returns the drive folder id for each folder path under
// parentID, reusing live folders with the same canonical name and creating the
// rest. A name taken by a file is a conflict.
func (a *App) ImportResolveFolders(driveID int64, parentID string, folderPaths []string) (map[string]string, error) {
	svc, err := a.requireImportDrive(driveID)
	if err != nil {
		return nil, err
	}
	return svc.ResolveImportFolders(context.Background(), driveID, parentID, folderPaths)
}

// ImportFindExisting reports which entries already exist live on the drive
// under parentID with the same size and encryption policy. The ledger can only
// veto a match, so the frontend combines the two answers.
func (a *App) ImportFindExisting(driveID int64, parentID string, entries []folderservice.ImportEntry) ([]folderservice.ImportMatch, error) {
	svc, err := a.requireImportDrive(driveID)
	if err != nil {
		return nil, err
	}
	return svc.FindExistingImportEntries(context.Background(), driveID, parentID, entries)
}

// ImportReplaceFile puts a file the import just uploaded in place of the older
// file of the same name: the older one goes to the trash and the new one takes
// its name. See fileservice.ReplaceImported.
func (a *App) ImportReplaceFile(driveID int64, oldMsgID int64, newMsgID int64, name string) error {
	if _, err := a.requireImportDrive(driveID); err != nil {
		return err
	}
	svc, err := a.requireFileService()
	if err != nil {
		return err
	}
	return svc.ReplaceImported(context.Background(), driveID, oldMsgID, newMsgID, name)
}
