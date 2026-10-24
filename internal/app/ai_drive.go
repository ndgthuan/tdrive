package app

import (
	"context"
	"errors"
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
		files = append(files, ai.FileNode{ID: file.MsgID, Name: file.Name, ParentID: file.ParentID})
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
