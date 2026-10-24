package file

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"TDrive/backend/drive_roles"
	"TDrive/backend/projection"
)

// Shared-drive write gates. Every mutating entry point of this package calls one
// of these before it touches Telegram or the projection, and every refusal is a
// drive_roles error with the permission_denied code. The rules themselves live
// in drive_roles (guard.go); this file only decides which rule a call needs.
//
// Without a Roles guard (tests and offline tools) a shared drive is treated as
// an editor: the uploader rule applies and nothing else is checked. The personal
// drive and channel sources are never gated here.

// shared_caps returns the caller's capabilities when channelID is a shared drive.
// The bool is false for every other drive, which then passes the gates.
func (s *Service) shared_caps(ctx context.Context, channelID int64) (drive_roles.Capabilities, bool, error) {
	if s.DB == nil {
		// Without a projection there is no drive kind to read. The caller's own
		// dependency check reports the missing database with a clearer error.
		return drive_roles.Capabilities{}, false, nil
	}
	ch, err := projection.GetChannel(s.DB, channelID)
	if err != nil {
		return drive_roles.Capabilities{}, false, err
	}
	if ch.Kind != projection.KindShared {
		return drive_roles.Capabilities{}, false, nil
	}
	if s.Roles == nil {
		return drive_roles.CapabilitiesFor(drive_roles.RoleEditor), true, nil
	}
	caps, err := s.Roles.MyRole(ctx, channelID)
	if err != nil {
		return drive_roles.Capabilities{}, true, err
	}
	return caps, true, nil
}

// actor_id returns the calling account's Telegram user id.
func (s *Service) actor_id(ctx context.Context) (int64, error) {
	if s.ActorID == nil {
		return 0, fmt.Errorf("actor resolver not ready")
	}
	return s.ActorID(ctx)
}

// requireUploadRight gates every write that adds content: uploads, imports,
// hidden uploads, resumed uploads, folder creation and metadata adoption.
// Viewers are refused.
func (s *Service) requireUploadRight(ctx context.Context, channelID int64) error {
	caps, shared, err := s.shared_caps(ctx, channelID)
	if err != nil || !shared {
		return err
	}
	return drive_roles.RequireUpload(caps)
}

// requireOwnerForShared gates a change to one existing file (rename, move,
// delete, trash, restore, purge, previews). Viewers are refused, managers and
// the owner may change any file, and editors may change only their own uploads.
func (s *Service) requireOwnerForShared(ctx context.Context, channelID int64, msgID int, action string) error {
	caps, shared, err := s.shared_caps(ctx, channelID)
	if err != nil || !shared {
		return err
	}
	if err := drive_roles.RequireUpload(caps); err != nil {
		return err
	}
	if caps.CanDeleteOthers {
		return nil
	}
	actorID, err := s.actor_id(ctx)
	if err != nil {
		return err
	}
	uploader, err := projection.FileUploader(s.DB, channelID, int64(msgID))
	if err != nil {
		return err
	}
	return drive_roles.RequireFileChange(caps, uploader, actorID, action)
}

// requireObjectWrite gates a trash, restore or purge of a file or folder object.
//   - A file follows requireOwnerForShared.
//   - Trashing a folder is allowed to an editor only when every file inside it is
//     their own, the same rule the folder service applies to a folder delete.
//   - Restoring or purging a folder is a manager action: a trashed folder's
//     contents are not visible to the subtree walk, so their owners cannot be
//     checked.
func (s *Service) requireObjectWrite(ctx context.Context, channelID int64, objectID string, action string) error {
	if msg, ok := file_msg_id(objectID); ok {
		return s.requireOwnerForShared(ctx, channelID, msg, action)
	}
	caps, shared, err := s.shared_caps(ctx, channelID)
	if err != nil || !shared {
		return err
	}
	if action != "trash" {
		return drive_roles.RequireFolderChangeManager(caps, action)
	}
	files, err := projection.FolderSubtreeFiles(s.DB, channelID, objectID)
	if err != nil {
		return err
	}
	actorID, err := s.actor_id(ctx)
	if err != nil {
		return err
	}
	uploaders := make([]int64, 0, len(files))
	for _, file := range files {
		uploaders = append(uploaders, file.UploaderID)
	}
	return drive_roles.RequireFolderSubtreeChange(caps, uploaders, actorID, action)
}

// file_msg_id reads the Telegram message id out of a file object id.
func file_msg_id(objectID string) (int, bool) {
	if !strings.HasPrefix(objectID, projection.FileIDPrefix) {
		return 0, false
	}
	msg, err := strconv.Atoi(strings.TrimPrefix(objectID, projection.FileIDPrefix))
	if err != nil || msg <= 0 {
		return 0, false
	}
	return msg, true
}
