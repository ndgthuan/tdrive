package app

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"TDrive/backend/ai"
	"TDrive/backend/projection"
	fileservice "TDrive/backend/services/file"
	folderservice "TDrive/backend/services/folder"
	readservice "TDrive/backend/services/read"
)

// aiDrive implements ai.Drive on top of the existing file, folder and read
// services. It adds no new write path: moves and folder creation go through the
// same services the file browser uses, and nothing here deletes.
type aiDrive struct {
	files   *fileservice.Service
	reads   *readservice.Service
	folders *folderservice.Service
}

// CanOrganize allows only channels the device keeps as TDrive drives. Channel
// sources are Telegram channels the user merely watches, so they are read-only.
func (d *aiDrive) CanOrganize(channel_id int64) error {
	channels, err := projection.ListChannels(d.reads.DB)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if channel.ChannelID == channel_id {
			return nil
		}
	}
	return errors.New("ai: this is a channel source, which is read-only; choose a drive to organize")
}

// DriveName is the root name the user sees: My Drive for the personal drive, the
// title for a shared one. It is empty when the channel row cannot be read.
func (d *aiDrive) DriveName(channel_id int64) string {
	channel, err := projection.GetChannel(d.reads.DB, channel_id)
	if err != nil {
		return ""
	}
	if channel.Kind == projection.KindPersonal {
		return "My Drive"
	}
	return strings.TrimSpace(channel.Title)
}

// ListFolder returns the direct children of folder_id from the projection.
func (d *aiDrive) ListFolder(ctx context.Context, channel_id int64, folder_id string) ([]ai.FolderNode, []ai.FileNode, error) {
	listing, err := d.reads.FolderContentsContext(ctx, channel_id, folder_id)
	if err != nil {
		return nil, nil, err
	}
	folders := make([]ai.FolderNode, 0, len(listing.Folders))
	for _, folder := range listing.Folders {
		folders = append(folders, ai.FolderNode{ID: folder.ID, Name: folder.Name, ParentID: folder.ParentID})
	}
	files := make([]ai.FileNode, 0, len(listing.Files))
	for _, file := range listing.Files {
		files = append(files, ai.FileNode{ID: file.MsgID, Name: file.Name, ParentID: file.ParentID, Size: file.Size})
	}
	return folders, files, nil
}

// Thumbnail returns the stored thumbnail rendition, generating it through the
// same pipeline the gallery uses. Originals are never downloaded for this.
func (d *aiDrive) Thumbnail(ctx context.Context, channel_id int64, file_id int64) ([]byte, error) {
	rendition, err := d.files.Rendition(ctx, channel_id, file_id, 0, "thumbnail")
	if err != nil {
		return nil, err
	}
	return rendition.Bytes, nil
}

// CreateFolder creates a folder through the folder service.
func (d *aiDrive) CreateFolder(ctx context.Context, channel_id int64, name string, parent_id string) (string, error) {
	folder, err := d.folders.CreateContext(ctx, channel_id, name, parent_id)
	if err != nil {
		return "", err
	}
	return folder.ID, nil
}

// ParentOf reads the file's current folder from the projection.
func (d *aiDrive) ParentOf(ctx context.Context, channel_id int64, file_id int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return projection.FileParent(d.reads.DB, channel_id, file_id)
}

// MoveFile moves one file with the same file service the browser uses.
func (d *aiDrive) MoveFile(ctx context.Context, channel_id int64, file_id int64, parent_id string) error {
	return d.files.Move(ctx, channel_id, int(file_id), parent_id)
}

// aiItemObjectID maps an ai item id to the projection object id: a folder is
// already "d:<uuid>", a file is its message id with the "f:" prefix.
func aiItemObjectID(item_id string) string {
	if strings.HasPrefix(item_id, projection.FolderIDPrefix) {
		return item_id
	}
	return projection.FileIDPrefix + item_id
}

// aiFileMsgID reads the message id of a file item id.
func aiFileMsgID(item_id string) (int, error) {
	n, err := strconv.ParseInt(item_id, 10, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("ai: not a file id: " + item_id)
	}
	return int(n), nil
}

// Lookup reads one live file or folder from the projection. Trashed and unknown
// ids are errors, so tools never act on something the drive does not show.
func (d *aiDrive) Lookup(ctx context.Context, channel_id int64, item_id string) (ai.Item, error) {
	if err := ctx.Err(); err != nil {
		return ai.Item{}, err
	}
	entry, found, err := projection.DirentByID(d.reads.DB, channel_id, aiItemObjectID(item_id))
	if err != nil {
		return ai.Item{}, err
	}
	if !found {
		return ai.Item{}, errors.New("item not found in this drive")
	}
	if entry.Tombstoned {
		return ai.Item{}, errors.New("item is in the trash")
	}
	return ai.Item{
		ID:       item_id,
		Name:     entry.DisplayName,
		IsFolder: entry.ObjectKind == projection.ObjectKindFolder,
		ParentID: entry.ParentID,
	}, nil
}

// RenameItem renames through the file or folder service, which apply the shared
// drive owner rule and the encryption checks.
func (d *aiDrive) RenameItem(ctx context.Context, channel_id int64, item_id string, new_name string) error {
	if strings.HasPrefix(item_id, projection.FolderIDPrefix) {
		return d.folders.RenameContext(ctx, channel_id, item_id, new_name)
	}
	msg, err := aiFileMsgID(item_id)
	if err != nil {
		return err
	}
	return d.files.Rename(ctx, channel_id, msg, new_name)
}

// MoveItem moves a file or folder through the same services the browser uses.
func (d *aiDrive) MoveItem(ctx context.Context, channel_id int64, item_id string, parent_id string) error {
	if strings.HasPrefix(item_id, projection.FolderIDPrefix) {
		return d.folders.MoveContext(ctx, channel_id, item_id, parent_id)
	}
	msg, err := aiFileMsgID(item_id)
	if err != nil {
		return err
	}
	return d.files.Move(ctx, channel_id, msg, parent_id)
}

// TrashItem moves a file or folder to the trash. Nothing is destroyed.
func (d *aiDrive) TrashItem(ctx context.Context, channel_id int64, item_id string) error {
	if strings.HasPrefix(item_id, projection.FolderIDPrefix) {
		return d.folders.Delete(ctx, channel_id, item_id)
	}
	msg, err := aiFileMsgID(item_id)
	if err != nil {
		return err
	}
	return d.files.Delete(ctx, channel_id, msg)
}

// RestoreItem takes a trashed file or folder back out of the trash.
func (d *aiDrive) RestoreItem(ctx context.Context, channel_id int64, item_id string) error {
	return d.files.RestoreObject(ctx, channel_id, aiItemObjectID(item_id))
}
