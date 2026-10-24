package drive_roles

import "context"

// Guard is what the file, folder and assistant services need from roles: the
// caller's capabilities in one drive. *Service implements it.
type Guard interface {
	MyRole(ctx context.Context, channel_id int64) (Capabilities, error)
}

// ErrNotManager is returned when a folder is renamed or moved by a member who
// is not a manager. Folders have no uploader, so only managers may change them.
var ErrNotManager = &Error{Code: CodePermissionDenied, Message: "Only managers can rename or move folders in a shared drive."}

// RequireUpload returns ErrViewOnly unless the capabilities allow writing.
func RequireUpload(caps Capabilities) error {
	if !caps.CanUpload {
		return ErrViewOnly
	}
	return nil
}

// RequireFolderManage returns ErrNotManager unless the capabilities may change
// members' content beyond their own files: folders are shared, so renaming or
// moving one is a manager action.
func RequireFolderManage(caps Capabilities) error {
	if err := RequireUpload(caps); err != nil {
		return err
	}
	if !caps.CanDeleteOthers {
		return ErrNotManager
	}
	return nil
}

// RequireFileChange is the one rule for changing an existing file in a shared
// drive (rename, move, trash, restore, purge, delete). Viewers are refused.
// Managers and the owner may change any file. An editor may change only a file
// they uploaded; uploader_id 0 (unknown) is refused for editors.
func RequireFileChange(caps Capabilities, uploader_id, actor_id int64, action string) error {
	if err := RequireUpload(caps); err != nil {
		return err
	}
	if caps.CanDeleteOthers {
		return nil
	}
	if uploader_id == 0 || uploader_id != actor_id {
		return NotUploaderError(action)
	}
	return nil
}

// RequireFolderSubtreeChange is the rule for trashing, restoring or purging a
// folder: the files inside it are what changes. Managers and the owner pass.
// An editor passes only when every file in the subtree is theirs; an empty
// subtree has nothing of anyone else's in it.
func RequireFolderSubtreeChange(caps Capabilities, uploader_ids []int64, actor_id int64, action string) error {
	if err := RequireUpload(caps); err != nil {
		return err
	}
	if caps.CanDeleteOthers {
		return nil
	}
	for _, uploader := range uploader_ids {
		if uploader == 0 || uploader != actor_id {
			return &Error{Code: CodePermissionDenied, Message: "Only the uploader can " + action + " every file in this folder in a shared drive"}
		}
	}
	return nil
}

// RequireFolderChangeManager returns a permission_denied error unless the
// capabilities may act on folders in the shared drive: managers and the owner
// only. action names the change ("restore", "purge").
func RequireFolderChangeManager(caps Capabilities, action string) error {
	if err := RequireUpload(caps); err != nil {
		return err
	}
	if !caps.CanDeleteOthers {
		return &Error{Code: CodePermissionDenied, Message: "Only managers can " + action + " folders in a shared drive."}
	}
	return nil
}
