package ai

import "context"

// Drive is the TDrive surface the assistant needs. The Wails layer implements
// it on top of the file, folder and read services; tests implement it with an
// in-memory fake. Every call names its drive explicitly so an in-flight
// operation never follows a later drive switch.
//
// Item ids in the string-based methods use the file service's own ids: a file
// is its decimal Telegram message id ("2615"), a folder is "d:<uuid>". The
// empty string is the drive root where a parent is expected.
type Drive interface {
	// CanOrganize returns nil when the drive may be changed. Channel sources
	// are read-only and must return an error.
	CanOrganize(channel_id int64) error
	// DriveName returns the name the user sees for the drive's root: "My Drive"
	// for the personal drive, otherwise the shared drive's title. Empty when the
	// drive is unknown, in which case prompts fall back to the drive id.
	DriveName(channel_id int64) string
	// ListFolder returns the direct child folders and files of folder_id.
	// An empty folder_id is the drive root.
	ListFolder(ctx context.Context, channel_id int64, folder_id string) ([]FolderNode, []FileNode, error)
	// Thumbnail returns a small preview of the file as encoded image bytes. For
	// videos this is the poster frame.
	Thumbnail(ctx context.Context, channel_id int64, file_id int64) ([]byte, error)
	// CreateFolder creates a folder under parent_id and returns its id.
	CreateFolder(ctx context.Context, channel_id int64, name string, parent_id string) (string, error)
	// ParentOf returns the folder id that currently holds the file. Empty is root.
	ParentOf(ctx context.Context, channel_id int64, file_id int64) (string, error)
	// MoveFile moves one file into parent_id. It never deletes anything.
	MoveFile(ctx context.Context, channel_id int64, file_id int64, parent_id string) error
	// Lookup returns one live (not trashed) file or folder of the drive. It
	// fails when the id is unknown to the drive or the item is in the trash.
	Lookup(ctx context.Context, channel_id int64, item_id string) (Item, error)
	// RenameItem changes the name of one file or folder.
	RenameItem(ctx context.Context, channel_id int64, item_id string, new_name string) error
	// MoveItem moves one file or folder into parent_id ("" is the root).
	MoveItem(ctx context.Context, channel_id int64, item_id string, parent_id string) error
	// TrashItem moves one file or folder into the trash. Nothing is destroyed.
	TrashItem(ctx context.Context, channel_id int64, item_id string) error
	// RestoreItem takes one trashed file or folder back out of the trash.
	RestoreItem(ctx context.Context, channel_id int64, item_id string) error
}

// Item is one live file or folder as Drive.Lookup reports it.
type Item struct {
	ID       string
	Name     string
	IsFolder bool
	ParentID string
	Size     int64
}

// FolderNode is one folder returned by Drive.ListFolder.
type FolderNode struct {
	ID       string
	Name     string
	ParentID string
}

// FileNode is one file returned by Drive.ListFolder.
type FileNode struct {
	ID       int64
	Name     string
	ParentID string
	Size     int64
}
