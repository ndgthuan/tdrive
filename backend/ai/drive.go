package ai

import "context"

// Drive is the TDrive surface the organize planner and applier need. The
// Wails layer implements it on top of the file, folder and read services; tests
// implement it with an in-memory fake. Every call names its drive explicitly so
// an in-flight operation never follows a later drive switch.
type Drive interface {
	// CanOrganize returns nil when the drive may be reorganized. Channel
	// sources are read-only and must return an error.
	CanOrganize(channel_id int64) error
	// ListFolder returns the direct child folders and files of folder_id.
	// An empty folder_id is the drive root.
	ListFolder(ctx context.Context, channel_id int64, folder_id string) ([]FolderNode, []FileNode, error)
	// Thumbnail returns a small preview of the file as encoded image bytes.
	Thumbnail(ctx context.Context, channel_id int64, file_id int64) ([]byte, error)
	// CreateFolder creates a folder under parent_id and returns its id.
	CreateFolder(ctx context.Context, channel_id int64, name string, parent_id string) (string, error)
	// ParentOf returns the folder id that currently holds the file. Empty is root.
	ParentOf(ctx context.Context, channel_id int64, file_id int64) (string, error)
	// MoveFile moves one file into parent_id. It never deletes anything.
	MoveFile(ctx context.Context, channel_id int64, file_id int64, parent_id string) error
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
}
