// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// trash_preview_items bounds how many item names an approval card or a chat
// action record lists. The counts in the summary are always exact.
const trash_preview_items = 50

// trash_row is one trash entry in a list_trash result.
type trash_row struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size,omitempty"`
	ParentPath string `json:"parent_path,omitempty"`
	DeletedAt  string `json:"deleted_at,omitempty"`
}

// trashed_kind classifies a trash entry the same way live items are classified.
func trashed_kind(item TrashedItem) string {
	return item_kind(item.Name, item.IsFolder)
}

// trash_action_items names trash entries for an approval card or a chat record,
// capped at trash_preview_items.
func trash_action_items(items []TrashedItem) []ActionItem {
	n := min(len(items), trash_preview_items)
	out := make([]ActionItem, 0, n)
	for _, item := range items[:n] {
		out = append(out, ActionItem{ID: item.ID, Name: item.Name, Kind: trashed_kind(item)})
	}
	return out
}

// trash_index maps trash ids to entries.
func trash_index(items []TrashedItem) map[string]TrashedItem {
	index := make(map[string]TrashedItem, len(items))
	for _, item := range items {
		index[item.ID] = item
	}
	return index
}

// list_trash_args are the list_trash arguments.
type list_trash_args struct {
	Limit int `json:"limit"`
}

func (s *Service) tool_list_trash(ctx context.Context, env agent_env, raw string) tool_output {
	var args list_trash_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	all, err := env.drive.ListTrash(ctx, env.scope.ChannelID)
	if err != nil {
		return tool_error(err)
	}
	total := len(all)
	limit := clamp_limit(args.Limit, default_list_limit)
	truncated := total > limit
	if truncated {
		all = all[:limit]
	}
	rows := make([]trash_row, 0, len(all))
	for _, item := range all {
		row := trash_row{
			ID: item.ID, Name: item.Name, Kind: trashed_kind(item), Size: item.Size, ParentPath: item.ParentPath,
		}
		if item.DeletedAt > 0 {
			row.DeletedAt = time.Unix(item.DeletedAt, 0).UTC().Format(time.RFC3339)
		}
		rows = append(rows, row)
	}
	return tool_output{content: json_content(map[string]any{
		"drive":     drive_label(env),
		"total":     total,
		"items":     rows,
		"truncated": truncated,
	})}
}

// restore_args are the restore_items arguments.
type restore_args struct {
	ItemIDs []flex_string `json:"item_ids"`
}

// tool_restore_items takes trashed items back to their original place. It runs
// at once; the undo moves the restored items back into the trash.
func (s *Service) tool_restore_items(ctx context.Context, env agent_env, raw string) tool_output {
	var args restore_args
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
	listing, err := env.drive.ListTrash(ctx, channel)
	if err != nil {
		return tool_error(err)
	}
	index := trash_index(listing)
	var failures []string
	var todo []TrashedItem
	for _, id := range ids {
		item, ok := index[id]
		if !ok {
			failures = append(failures, id+": not in the trash")
			continue
		}
		todo = append(todo, item)
	}
	if len(todo) == 0 {
		return tool_error(errors.New("nothing to restore: " + strings.Join(failures, "; ")))
	}

	var done []TrashedItem
	for _, item := range todo {
		if err := env.drive.RestoreItem(ctx, channel, item.ID); err != nil {
			failures = append(failures, item.Name+": "+err.Error())
			continue
		}
		done = append(done, item)
	}
	var undo_token string
	if len(done) > 0 {
		undo_token = s.store_action_undo(channel, func(ctx context.Context, drive Drive) (string, error) {
			moved := 0
			var errs []string
			for _, item := range done {
				if err := drive.TrashItem(ctx, channel, item.ID); err != nil {
					errs = append(errs, item.Name+": "+err.Error())
					continue
				}
				moved++
			}
			if len(errs) > 0 {
				return fmt.Sprintf("Moved %d of %d item(s) back to the trash.", moved, len(done)), errors.New(strings.Join(errs, "; "))
			}
			return fmt.Sprintf("Moved %d item(s) back to the trash.", moved), nil
		})
	}
	summary := fmt.Sprintf("Restored %d of %d item(s) from the trash in %s", len(done), len(todo), drive_label(env))
	action := ChatAction{
		Tool:      "restore_items",
		Summary:   summary,
		OK:        len(done) > 0 && len(failures) == 0,
		UndoToken: undo_token,
		Items:     trash_action_items(done),
	}
	if len(failures) > 0 {
		action.Error = strings.Join(failures, "; ")
	}
	result := json_content(map[string]any{"restored": len(done), "failed": failures})
	if len(done) == 0 {
		return tool_output{content: result, is_error: true, action: s.emit_action(env, action)}
	}
	return tool_output{content: result, action: s.emit_action(env, action)}
}

// empty_args are the empty_trash arguments. ItemIDs is nil when the key is
// absent, which means the whole trash.
type empty_args struct {
	ItemIDs []flex_string `json:"item_ids"`
}

// tool_empty_trash permanently deletes trash entries. It always asks first and
// has no undo. The deletion is the same purge the Trash view runs.
func (s *Service) tool_empty_trash(ctx context.Context, env agent_env, raw string) tool_output {
	var args empty_args
	if err := decode_args(raw, &args); err != nil {
		return tool_error(err)
	}
	channel := env.scope.ChannelID
	var wanted []string
	if args.ItemIDs != nil {
		ids, err := check_item_ids(args.ItemIDs)
		if err != nil {
			return tool_error(err)
		}
		wanted = ids
	}
	if err := organize_guard(ctx, env.drive, channel); err != nil {
		return tool_error(err)
	}
	listing, err := env.drive.ListTrash(ctx, channel)
	if err != nil {
		return tool_error(err)
	}
	var todo []TrashedItem
	var failures []string
	if wanted == nil {
		todo = listing
	} else {
		index := trash_index(listing)
		for _, id := range wanted {
			item, ok := index[id]
			if !ok {
				failures = append(failures, id+": not in the trash")
				continue
			}
			todo = append(todo, item)
		}
		if len(todo) == 0 {
			return tool_error(errors.New("nothing to delete permanently: " + strings.Join(failures, "; ")))
		}
	}
	if len(todo) == 0 {
		return tool_output{content: json_content(map[string]any{
			"drive":   drive_label(env),
			"purged":  0,
			"message": "The trash is already empty.",
		})}
	}

	items := trash_action_items(todo)
	summary := fmt.Sprintf("PERMANENTLY delete %d item(s) from the Trash in %s. This cannot be undone.", len(todo), drive_label(env))
	if len(todo) > trash_preview_items {
		summary += fmt.Sprintf(" The first %d are listed.", trash_preview_items)
	}
	approved, reason := s.await_approval(ctx, env, "empty_trash", summary, items)
	if !approved {
		return s.denied_output(env, "empty_trash", summary, items, reason)
	}

	// Like the Trash view's Empty trash, stop at the first failure so what is
	// left in the trash is exactly what the result reports, not an arbitrary
	// subset.
	var done []TrashedItem
	for _, item := range todo {
		if err := env.drive.PurgeItem(ctx, channel, item.ID); err != nil {
			failures = append(failures, item.Name+": "+err.Error())
			break
		}
		done = append(done, item)
	}
	remaining := len(todo) - len(done)
	action := ChatAction{
		Tool:    "empty_trash",
		Summary: fmt.Sprintf("Permanently deleted %d of %d item(s) from the Trash in %s", len(done), len(todo), drive_label(env)),
		OK:      len(done) == len(todo),
		Items:   trash_action_items(done),
	}
	if len(failures) > 0 {
		action.Error = strings.Join(failures, "; ")
	}
	result := json_content(map[string]any{"purged": len(done), "remaining": remaining, "failed": failures})
	if len(done) == 0 {
		return tool_output{content: result, is_error: true, action: s.emit_action(env, action)}
	}
	return tool_output{content: result, action: s.emit_action(env, action)}
}
