// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// audio_extensions and document_extensions classify files for list and search
// filters. Anything else is reported as kind "file".
var audio_extensions = map[string]bool{
	".mp3": true, ".m4a": true, ".aac": true, ".flac": true, ".wav": true,
	".ogg": true, ".opus": true,
}

var document_extensions = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".txt": true, ".md": true,
	".rtf": true, ".xls": true, ".xlsx": true, ".csv": true, ".ppt": true, ".pptx": true,
}

// default_list_limit and default_search_limit cap results when no limit is given.
const (
	default_list_limit   = 100
	default_search_limit = 50
	max_result_limit     = 200
)

// tool_item is one file or folder in a tool result.
type tool_item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Size     int64  `json:"size,omitempty"`
	ParentID string `json:"parent_id"`
	IsFolder bool   `json:"is_folder"`
	Path     string `json:"path,omitempty"`
}

// item_kind classifies a live item by its name.
func item_kind(name string, is_folder bool) string {
	if is_folder {
		return "folder"
	}
	ext := strings.ToLower(path.Ext(name))
	switch {
	case image_extensions[ext]:
		return "image"
	case video_extensions[ext]:
		return "video"
	case audio_extensions[ext]:
		return "audio"
	case document_extensions[ext]:
		return "document"
	default:
		return "file"
	}
}

// normalize_kind_filter maps a kind argument to one canonical filter. "any" is
// the default and matches everything.
func normalize_kind_filter(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "any", "all":
		return "any", nil
	case "image", "images", "photo", "photos", "picture", "pictures":
		return "image", nil
	case "video", "videos":
		return "video", nil
	case "audio":
		return "audio", nil
	case "document", "documents", "doc", "docs":
		return "document", nil
	case "folder", "folders":
		return "folder", nil
	default:
		return "", fmt.Errorf("kind must be image, video, audio, document, folder or any; got %q", kind)
	}
}

func kind_matches(filter, kind string) bool {
	return filter == "any" || filter == kind
}

// clamp_limit applies the default and the hard cap to a limit argument.
func clamp_limit(limit, fallback int) int {
	if limit <= 0 {
		return fallback
	}
	return min(limit, max_result_limit)
}

// parse_file_id reads a file id, which is its decimal message id.
func parse_file_id(id string) (int64, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a file id", id)
	}
	return n, nil
}

// parent_label is the id the model sees for a folder's parent.
func parent_label(parent string) string {
	if parent == "" {
		return "root"
	}
	return parent
}

// json_content encodes a tool result as compact JSON for the model.
func json_content(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("encode result: %v", err)
	}
	return string(data)
}

// agent_tool_specs declares the tools for every provider. The list is the same
// with or without vision; view_media and organize_photos explain themselves when
// the active model cannot see media.
func agent_tool_specs() []ToolSpec {
	kind_schema := map[string]any{
		"type":        "string",
		"enum":        []string{"image", "video", "audio", "document", "folder", "any"},
		"description": "Filter by kind. Default any.",
	}
	id_list := func(description string) map[string]any {
		return map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"minItems":    1,
			"maxItems":    max_tool_items,
			"description": description,
		}
	}
	return []ToolSpec{
		{
			Name:        "list_files",
			Description: "List the files and folders directly inside a folder of the current drive. Omit folder_id for the folder the user is in (or the root).",
			Parameters: object_schema(map[string]any{
				"folder_id": map[string]any{"type": "string", "description": "Folder id (d:...) or \"root\"."},
				"kind":      kind_schema,
				"limit":     map[string]any{"type": "integer", "description": "Max items, up to 200."},
			}, nil),
		},
		{
			Name:        "search_files",
			Description: "Find files and folders whose name contains the query (case-insensitive) anywhere in the current drive. Results include the folder path.",
			Parameters: object_schema(map[string]any{
				"query": map[string]any{"type": "string", "description": "Text the name must contain."},
				"kind":  kind_schema,
				"limit": map[string]any{"type": "integer", "description": "Max results, up to 200."},
			}, []string{"query"}),
		},
		{
			Name:        "create_folder",
			Description: "Create a folder. parent_id defaults to the folder the user is in.",
			Parameters: object_schema(map[string]any{
				"name":      map[string]any{"type": "string"},
				"parent_id": map[string]any{"type": "string", "description": "Folder id (d:...) or \"root\"."},
			}, []string{"name"}),
		},
		{
			Name:        "rename_item",
			Description: "Rename one file or folder. Renaming a folder asks the user to approve.",
			Parameters: object_schema(map[string]any{
				"item_id":  map[string]any{"type": "string"},
				"new_name": map[string]any{"type": "string"},
			}, []string{"item_id", "new_name"}),
		},
		{
			Name:        "move_items",
			Description: "Move files and/or folders into a destination folder. Moves of more than 25 items ask the user to approve.",
			Parameters: object_schema(map[string]any{
				"item_ids":              id_list("File ids (decimal) or folder ids (d:...)."),
				"destination_folder_id": map[string]any{"type": "string", "description": "Folder id (d:...) or \"root\"."},
			}, []string{"item_ids", "destination_folder_id"}),
		},
		{
			Name:        "delete_items",
			Description: "Move files and/or folders to the trash. Always needs the user's approval. Nothing is destroyed; the user can restore from the trash or undo.",
			Parameters: object_schema(map[string]any{
				"item_ids": id_list("File ids (decimal) or folder ids (d:...)."),
			}, []string{"item_ids"}),
		},
		{
			Name:        "list_trash",
			Description: "List what is in the trash of the current drive: id, name, kind, size, the folder it came from and when it was deleted. Read-only.",
			Parameters: object_schema(map[string]any{
				"limit": map[string]any{"type": "integer", "description": "Max items, up to 200."},
			}, nil),
		},
		{
			Name:        "restore_items",
			Description: "Restore files and/or folders from the trash to where they were (or the drive root if that folder is gone). Runs at once without approval; the user can undo it, which moves them back to the trash.",
			Parameters: object_schema(map[string]any{
				"item_ids": id_list("Ids of trashed files (decimal) or folders (d:...), as list_trash reports them."),
			}, []string{"item_ids"}),
		},
		{
			Name:        "empty_trash",
			Description: "PERMANENTLY delete items from the trash. Omit item_ids to empty the whole trash; give item_ids to delete only those. Always needs the user's approval. It cannot be undone.",
			Parameters: object_schema(map[string]any{
				"item_ids": id_list("Optional ids of trashed items to delete permanently. Omit to empty the whole trash."),
			}, nil),
		},
		{
			Name:        "view_media",
			Description: "Look at up to 12 pictures or videos (videos show their poster frame). Returns each item's name and the images. Needs a vision-enabled model.",
			Parameters: object_schema(map[string]any{
				"item_ids": id_list("File ids (decimal) of images or videos."),
			}, []string{"item_ids"}),
		},
		{
			Name:        "organize_photos",
			Description: "Sort the pictures and/or videos in the current folder by their content with the vision model. It proposes a plan the user reviews and applies; nothing moves yet. Use it for large jobs.",
			Parameters: object_schema(map[string]any{
				"instruction": map[string]any{"type": "string", "description": "How to group the items, e.g. by people, places or events."},
				"kind": map[string]any{
					"type":        "string",
					"enum":        []string{"images", "videos", "both"},
					"description": "What to sort. Default both.",
				},
			}, []string{"instruction"}),
		},
	}
}

func object_schema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// list_args are the list_files arguments.
type list_args struct {
	FolderID flex_string `json:"folder_id"`
	Kind     string      `json:"kind"`
	Limit    int         `json:"limit"`
}

func (s *Service) tool_list_files(ctx context.Context, env agent_env, raw string) tool_output {
	var args list_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	filter, err := normalize_kind_filter(args.Kind)
	if err != nil {
		return tool_error(err)
	}
	folder := env.scope.FolderID
	if given := string(args.FolderID); given != "" {
		folder, err = parent_arg(given)
		if err != nil {
			return tool_error(err)
		}
	}
	folders, files, err := env.drive.ListFolder(ctx, env.scope.ChannelID, folder)
	if err != nil {
		return tool_error(err)
	}
	items := listing_items(folders, files, filter, "")
	limit := clamp_limit(args.Limit, default_list_limit)
	truncated := len(items) > limit
	if truncated {
		items = items[:limit]
	}
	return tool_output{content: json_content(map[string]any{
		"drive":     drive_label(env),
		"role":      role_label(ctx, env.drive, env.scope.ChannelID),
		"folder_id": parent_label(folder),
		"items":     items,
		"truncated": truncated,
	})}
}

// listing_items converts one folder listing into tool items, applying the kind
// filter. Path is the folder's own path, used by search.
func listing_items(folders []FolderNode, files []FileNode, filter, folder_path string) []tool_item {
	items := make([]tool_item, 0, len(folders)+len(files))
	for _, folder := range folders {
		if !kind_matches(filter, "folder") {
			continue
		}
		items = append(items, tool_item{
			ID: folder.ID, Name: folder.Name, Kind: "folder", ParentID: parent_label(folder.ParentID),
			IsFolder: true, Path: folder_path,
		})
	}
	for _, file := range files {
		kind := item_kind(file.Name, false)
		if !kind_matches(filter, kind) {
			continue
		}
		items = append(items, tool_item{
			ID: strconv.FormatInt(file.ID, 10), Name: file.Name, Kind: kind, Size: file.Size,
			ParentID: parent_label(file.ParentID), Path: folder_path,
		})
	}
	return items
}

// search_args are the search_files arguments.
type search_args struct {
	Query string `json:"query"`
	Kind  string `json:"kind"`
	Limit int    `json:"limit"`
}

func (s *Service) tool_search_files(ctx context.Context, env agent_env, raw string) tool_output {
	var args search_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	query := strings.ToLower(strings.TrimSpace(args.Query))
	if query == "" {
		return tool_error(errors.New("query is required"))
	}
	filter, err := normalize_kind_filter(args.Kind)
	if err != nil {
		return tool_error(err)
	}
	limit := clamp_limit(args.Limit, default_search_limit)

	type queued struct {
		folder_id string
		folder    string
	}
	queue := []queued{{folder_id: ""}}
	visited := map[string]bool{"": true}
	visits := 0
	var matches []tool_item
	truncated := false
	complete := true
	for len(queue) > 0 && !truncated {
		if err := ctx.Err(); err != nil {
			return tool_error(err)
		}
		current := queue[0]
		queue = queue[1:]
		visits++
		if visits > search_walk_limit {
			complete = false
			break
		}
		folders, files, err := env.drive.ListFolder(ctx, env.scope.ChannelID, current.folder_id)
		if err != nil {
			return tool_error(err)
		}
		for _, item := range listing_items(folders, files, filter, current.folder) {
			if !strings.Contains(strings.ToLower(item.Name), query) {
				continue
			}
			if len(matches) == limit {
				truncated = true
				break
			}
			matches = append(matches, item)
		}
		for _, folder := range folders {
			if visited[folder.ID] {
				continue
			}
			visited[folder.ID] = true
			child := folder.Name
			if current.folder != "" {
				child = current.folder + "/" + folder.Name
			}
			queue = append(queue, queued{folder_id: folder.ID, folder: child})
		}
	}
	if matches == nil {
		matches = []tool_item{}
	}
	return tool_output{content: json_content(map[string]any{
		"drive":            drive_label(env),
		"query":            args.Query,
		"items":            matches,
		"truncated":        truncated,
		"searched_folders": visits,
		"complete":         complete && !truncated,
	})}
}

// create_args are the create_folder arguments.
type create_args struct {
	Name     string      `json:"name"`
	ParentID flex_string `json:"parent_id"`
}

func (s *Service) tool_create_folder(ctx context.Context, env agent_env, raw string) tool_output {
	var args create_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return tool_error(errors.New("name is required"))
	}
	if err := organize_guard(ctx, env.drive, env.scope.ChannelID); err != nil {
		return tool_error(err)
	}
	parent := env.scope.FolderID
	if given := string(args.ParentID); given != "" {
		parsed, err := parent_arg(given)
		if err != nil {
			return tool_error(err)
		}
		parent = parsed
	}
	summary := fmt.Sprintf("Create folder %q in %s", name, drive_label(env))
	id, err := env.drive.CreateFolder(ctx, env.scope.ChannelID, name, parent)
	if err != nil {
		action := s.emit_action(env, ChatAction{Tool: "create_folder", Summary: summary, OK: false, Error: err.Error()})
		return tool_output{content: err.Error(), is_error: true, action: action}
	}
	channel := env.scope.ChannelID
	undo := s.store_action_undo(channel, func(ctx context.Context, drive Drive) (string, error) {
		if err := drive.TrashItem(ctx, channel, id); err != nil {
			return "", err
		}
		return fmt.Sprintf("Moved folder %q to the trash.", name), nil
	})
	items := []ActionItem{{ID: id, Name: name, Kind: "folder"}}
	action := s.emit_action(env, ChatAction{Tool: "create_folder", Summary: summary, OK: true, UndoToken: undo, Items: items})
	return tool_output{content: json_content(map[string]any{"id": id, "name": name}), action: action}
}

// rename_args are the rename_item arguments.
type rename_args struct {
	ItemID  flex_string `json:"item_id"`
	NewName string      `json:"new_name"`
}

func (s *Service) tool_rename_item(ctx context.Context, env agent_env, raw string) tool_output {
	var args rename_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	id := string(args.ItemID)
	if !valid_item_id(id) {
		return tool_error(fmt.Errorf("%q is not a valid file or folder id", id))
	}
	new_name := strings.TrimSpace(args.NewName)
	if new_name == "" {
		return tool_error(errors.New("new_name is required"))
	}
	channel := env.scope.ChannelID
	if err := organize_guard(ctx, env.drive, channel); err != nil {
		return tool_error(err)
	}
	item, err := env.drive.Lookup(ctx, channel, id)
	if err != nil {
		return tool_error(err)
	}
	if item.Name == new_name {
		return tool_error(fmt.Errorf("%q is already named %q", id, new_name))
	}
	kind := item_kind(item.Name, item.IsFolder)
	items := []ActionItem{{ID: id, Name: item.Name, Kind: kind}}
	summary := fmt.Sprintf("Rename %q to %q in %s", item.Name, new_name, drive_label(env))
	if item.IsFolder {
		approved, reason := s.await_approval(ctx, env, "rename_item", summary, items)
		if !approved {
			return s.denied_output(env, "rename_item", summary, items, reason)
		}
	}
	if err := env.drive.RenameItem(ctx, channel, id, new_name); err != nil {
		action := s.emit_action(env, ChatAction{Tool: "rename_item", Summary: summary, OK: false, Error: err.Error(), Items: items})
		return tool_output{content: err.Error(), is_error: true, action: action}
	}
	old_name := item.Name
	undo := s.store_action_undo(channel, func(ctx context.Context, drive Drive) (string, error) {
		if err := drive.RenameItem(ctx, channel, id, old_name); err != nil {
			return "", err
		}
		return fmt.Sprintf("Renamed %q back to %q.", new_name, old_name), nil
	})
	action := s.emit_action(env, ChatAction{Tool: "rename_item", Summary: summary, OK: true, UndoToken: undo, Items: items})
	return tool_output{content: fmt.Sprintf("Renamed %q to %q.", old_name, new_name), action: action}
}

// move_args are the move_items arguments.
type move_args struct {
	ItemIDs             []flex_string `json:"item_ids"`
	DestinationFolderID flex_string   `json:"destination_folder_id"`
}

func (s *Service) tool_move_items(ctx context.Context, env agent_env, raw string) tool_output {
	var args move_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	ids, err := check_item_ids(args.ItemIDs)
	if err != nil {
		return tool_error(err)
	}
	if string(args.DestinationFolderID) == "" {
		return tool_error(errors.New("destination_folder_id is required: a folder id or \"root\""))
	}
	dest, err := parent_arg(string(args.DestinationFolderID))
	if err != nil {
		return tool_error(err)
	}
	channel := env.scope.ChannelID
	if err := organize_guard(ctx, env.drive, channel); err != nil {
		return tool_error(err)
	}
	dest_name := "the root of " + drive_label(env)
	if dest != "" {
		target, err := env.drive.Lookup(ctx, channel, dest)
		if err != nil {
			return tool_error(err)
		}
		if !target.IsFolder {
			return tool_error(errors.New("destination_folder_id is not a folder"))
		}
		dest_name = fmt.Sprintf("%q", target.Name)
	}

	var failures []string
	var todo []Item
	var items []ActionItem
	for _, id := range ids {
		item, err := env.drive.Lookup(ctx, channel, id)
		if err != nil {
			failures = append(failures, id+": "+err.Error())
			continue
		}
		if item.ParentID == dest {
			failures = append(failures, fmt.Sprintf("%q is already in %s", item.Name, dest_name))
			continue
		}
		todo = append(todo, item)
		items = append(items, ActionItem{ID: item.ID, Name: item.Name, Kind: item_kind(item.Name, item.IsFolder)})
	}
	if len(todo) == 0 {
		return tool_error(errors.New("nothing to move: " + strings.Join(failures, "; ")))
	}
	summary := fmt.Sprintf("Move %d item(s) to %s in %s", len(todo), dest_name, drive_label(env))
	if len(todo) > bulk_move_approval {
		approved, reason := s.await_approval(ctx, env, "move_items", summary, items)
		if !approved {
			return s.denied_output(env, "move_items", summary, items, reason)
		}
	}

	type moved struct {
		id     string
		name   string
		parent string
	}
	var done []moved
	for _, item := range todo {
		if err := env.drive.MoveItem(ctx, channel, item.ID, dest); err != nil {
			failures = append(failures, item.Name+": "+err.Error())
			continue
		}
		done = append(done, moved{id: item.ID, name: item.Name, parent: item.ParentID})
	}
	moved_items := make([]ActionItem, 0, len(done))
	for _, m := range done {
		moved_items = append(moved_items, ActionItem{ID: m.id, Name: m.name, Kind: "file"})
	}
	var undo_token string
	if len(done) > 0 {
		undo_token = s.store_action_undo(channel, func(ctx context.Context, drive Drive) (string, error) {
			restored := 0
			var errs []string
			for _, m := range done {
				if err := drive.MoveItem(ctx, channel, m.id, m.parent); err != nil {
					errs = append(errs, m.name+": "+err.Error())
					continue
				}
				restored++
			}
			if len(errs) > 0 {
				return fmt.Sprintf("Moved %d of %d item(s) back.", restored, len(done)), errors.New(strings.Join(errs, "; "))
			}
			return fmt.Sprintf("Moved %d item(s) back.", restored), nil
		})
	}
	action := ChatAction{
		Tool:      "move_items",
		Summary:   fmt.Sprintf("Moved %d of %d item(s) to %s in %s", len(done), len(todo), dest_name, drive_label(env)),
		OK:        len(done) > 0 && len(failures) == 0,
		UndoToken: undo_token,
		Items:     moved_items,
	}
	if len(failures) > 0 {
		action.Error = strings.Join(failures, "; ")
	}
	result := json_content(map[string]any{"moved": len(done), "failed": failures})
	if len(done) == 0 {
		return tool_output{content: result, is_error: true, action: s.emit_action(env, action)}
	}
	return tool_output{content: result, action: s.emit_action(env, action)}
}

// delete_args are the delete_items arguments.
type delete_args struct {
	ItemIDs []flex_string `json:"item_ids"`
}

func (s *Service) tool_delete_items(ctx context.Context, env agent_env, raw string) tool_output {
	var args delete_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	ids, err := check_item_ids(args.ItemIDs)
	if err != nil {
		return tool_error(err)
	}
	channel := env.scope.ChannelID
	if err := organize_guard(ctx, env.drive, channel); err != nil {
		return tool_error(err)
	}
	var failures []string
	var todo []ActionItem
	for _, id := range ids {
		item, err := env.drive.Lookup(ctx, channel, id)
		if err != nil {
			failures = append(failures, id+": "+err.Error())
			continue
		}
		todo = append(todo, ActionItem{ID: item.ID, Name: item.Name, Kind: item_kind(item.Name, item.IsFolder)})
	}
	if len(todo) == 0 {
		return tool_error(errors.New("nothing to delete: " + strings.Join(failures, "; ")))
	}
	summary := fmt.Sprintf("Move %d item(s) to the trash in %s (recoverable)", len(todo), drive_label(env))
	approved, reason := s.await_approval(ctx, env, "delete_items", summary, todo)
	if !approved {
		return s.denied_output(env, "delete_items", summary, todo, reason)
	}

	var done []ActionItem
	for _, item := range todo {
		if err := env.drive.TrashItem(ctx, channel, item.ID); err != nil {
			failures = append(failures, item.Name+": "+err.Error())
			continue
		}
		done = append(done, item)
	}
	var undo_token string
	if len(done) > 0 {
		undo_token = s.store_action_undo(channel, func(ctx context.Context, drive Drive) (string, error) {
			restored := 0
			var errs []string
			for _, item := range done {
				if err := drive.RestoreItem(ctx, channel, item.ID); err != nil {
					errs = append(errs, item.Name+": "+err.Error())
					continue
				}
				restored++
			}
			if len(errs) > 0 {
				return fmt.Sprintf("Restored %d of %d item(s) from the trash.", restored, len(done)), errors.New(strings.Join(errs, "; "))
			}
			return fmt.Sprintf("Restored %d item(s) from the trash.", restored), nil
		})
	}
	action := ChatAction{
		Tool:      "delete_items",
		Summary:   fmt.Sprintf("Moved %d of %d item(s) to the trash in %s", len(done), len(todo), drive_label(env)),
		OK:        len(done) > 0 && len(failures) == 0,
		UndoToken: undo_token,
		Items:     done,
	}
	if len(failures) > 0 {
		action.Error = strings.Join(failures, "; ")
	}
	result := json_content(map[string]any{"trashed": len(done), "failed": failures})
	if len(done) == 0 {
		return tool_output{content: result, is_error: true, action: s.emit_action(env, action)}
	}
	return tool_output{content: result, action: s.emit_action(env, action)}
}

// organize_args are the organize_photos arguments.
type organize_args struct {
	Instruction string `json:"instruction"`
	Kind        string `json:"kind"`
}

func (s *Service) tool_organize_photos(ctx context.Context, env agent_env, raw string) tool_output {
	var args organize_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	if !env.vision {
		return tool_error(errors.New(no_vision_message))
	}
	instruction := strings.TrimSpace(args.Instruction)
	if instruction == "" {
		return tool_error(errors.New("instruction is required"))
	}
	kind, err := parse_media_kind(args.Kind)
	if err != nil {
		return tool_error(err)
	}
	plan, err := s.PlanOrganize(ctx, PlanRequest{
		Scope:       env.scope,
		Instruction: instruction,
		ModelID:     env.model_id,
		Kind:        kind,
	})
	if err != nil {
		return tool_error(err)
	}
	type folder_line struct {
		Name  string `json:"name"`
		IsNew bool   `json:"is_new"`
		Count int    `json:"count"`
	}
	folders := make([]folder_line, 0, len(plan.Folders))
	for _, folder := range plan.Folders {
		folders = append(folders, folder_line{Name: folder.Name, IsNew: folder.IsNew, Count: len(folder.Files)})
	}
	action := s.emit_action(env, ChatAction{
		Tool:    "organize_photos",
		Summary: fmt.Sprintf("Planned %d item(s) into %d folder(s) in %s; nothing moved yet", plan.Total, len(plan.Folders), drive_label(env)),
		OK:      true,
		PlanID:  plan.PlanID,
	})
	return tool_output{content: json_content(map[string]any{
		"plan_id":  plan.PlanID,
		"total":    plan.Total,
		"skipped":  plan.Skipped,
		"folders":  folders,
		"notes":    plan.Notes,
		"approval": "Nothing has moved. The user applies this plan from the organize preview.",
	}), action: action}
}

// no_vision_message tells the model why media tools cannot run.
const no_vision_message = "The active model cannot see images or video (vision is off for it). Tell the user to choose a vision-enabled model in the model settings, then try again."

// view_args are the view_media arguments.
type view_args struct {
	ItemIDs []flex_string `json:"item_ids"`
}

// max_preview_bytes bounds the raw preview bytes read per item before decoding.
const max_preview_bytes = 16 << 20

func (s *Service) tool_view_media(ctx context.Context, env agent_env, raw string) tool_output {
	var args view_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	if !env.vision {
		return tool_error(errors.New(no_vision_message))
	}
	ids, err := check_item_ids(args.ItemIDs)
	if err != nil {
		return tool_error(err)
	}
	if len(ids) > max_view_items {
		return tool_error(fmt.Errorf("at most %d items per call; split the list", max_view_items))
	}
	channel := env.scope.ChannelID
	var images []Image
	var lines []string
	payload := 0
	// ids were de-duplicated by check_item_ids, so each preview is fetched once
	// per call.
	for _, id := range ids {
		if is_folder_id(id) {
			lines = append(lines, id+": a folder, not a picture or video")
			continue
		}
		item, err := env.drive.Lookup(ctx, channel, id)
		if err != nil {
			lines = append(lines, id+": "+err.Error())
			continue
		}
		kind := item_kind(item.Name, false)
		label := fmt.Sprintf("%s: %s (%s", id, item.Name, kind)
		if item.Size > 0 {
			label += fmt.Sprintf(", %d bytes", item.Size)
		}
		if kind != "image" && kind != "video" {
			lines = append(lines, label+"): not a picture or video")
			continue
		}
		file_id, err := parse_file_id(id)
		if err != nil {
			lines = append(lines, label+"): "+err.Error())
			continue
		}
		preview, err := env.drive.Thumbnail(ctx, channel, file_id)
		if err != nil {
			lines = append(lines, label+"): no preview available")
			continue
		}
		if len(preview) > max_preview_bytes {
			lines = append(lines, label+"): preview too large")
			continue
		}
		jpeg, err := prepare_image(preview)
		if err != nil {
			lines = append(lines, label+"): preview could not be decoded")
			continue
		}
		if payload+len(jpeg) > max_view_payload {
			lines = append(lines, label+"): skipped, payload limit reached; view it in a smaller call")
			continue
		}
		payload += len(jpeg)
		images = append(images, Image{MIMEType: "image/jpeg", Data: jpeg})
		lines = append(lines, fmt.Sprintf("%s, shown as image %d)", label, len(images)))
	}
	return tool_output{content: strings.Join(lines, "\n"), images: images}
}
