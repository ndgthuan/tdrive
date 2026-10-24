package ai

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	// plan_ttl bounds how long an unapplied plan stays available.
	plan_ttl = 2 * time.Hour
	// undo_ttl bounds how long an undo token stays valid after an apply.
	undo_ttl = 24 * time.Hour
)

// ApplyProgress is the payload of the ai:apply_progress event.
type ApplyProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// ApplyFailure is one file that could not be moved.
type ApplyFailure struct {
	FileID int64  `json:"file_id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ApplyResult reports what ApplyPlan did. UndoToken restores the moved files
// and is empty when nothing moved.
type ApplyResult struct {
	PlanID         string         `json:"plan_id"`
	Moved          int            `json:"moved"`
	CreatedFolders []string       `json:"created_folders"`
	Failed         []ApplyFailure `json:"failed"`
	UndoToken      string         `json:"undo_token"`
}

// UndoIssue is one file the undo step did not restore.
type UndoIssue struct {
	FileID int64  `json:"file_id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// UndoResult reports an undo run. Created folders are left in place.
type UndoResult struct {
	Restored int         `json:"restored"`
	Skipped  []UndoIssue `json:"skipped"`
	Failed   []UndoIssue `json:"failed"`
}

// undo_move records one applied move so it can be reversed.
type undo_move struct {
	file_id     int64
	name        string
	from_parent string
	to_parent   string
}

// undo_record is the server-side memory behind an undo token.
type undo_record struct {
	channel_id int64
	moves      []undo_move
	created_ms int64
}

// ErrPlanNotFound reports an unknown or expired plan id.
var ErrPlanNotFound = errors.New("ai: plan not found or expired; plan the organization again")

// ApplyPlan moves the planned files into their target folders, creating missing
// folders first. excluded lists file ids the user removed from the plan. Files
// are only ever moved, never deleted. Progress is sent as ai:apply_progress.
func (s *Service) ApplyPlan(ctx context.Context, plan_id string, excluded []int64) (ApplyResult, error) {
	plan, err := s.claim_plan(plan_id)
	if err != nil {
		return ApplyResult{}, err
	}
	started := false
	defer func() {
		if !started {
			s.release_plan(plan)
		}
	}()
	drive, err := s.drive()
	if err != nil {
		return ApplyResult{}, err
	}
	channel_id := plan.Scope.ChannelID
	if err := drive.CanOrganize(channel_id); err != nil {
		return ApplyResult{}, err
	}
	skip := make(map[int64]bool, len(excluded))
	for _, id := range excluded {
		skip[id] = true
	}
	total := 0
	for _, folder := range plan.Folders {
		for _, file := range folder.Files {
			if !skip[file.FileID] {
				total++
			}
		}
	}
	s.emit(EventApplyProgress, ApplyProgress{Done: 0, Total: total})
	started = true

	result := ApplyResult{
		PlanID:         plan.PlanID,
		CreatedFolders: []string{},
		Failed:         []ApplyFailure{},
	}
	var moves []undo_move
	done := 0
	for _, folder := range plan.Folders {
		var pending []PlannedFile
		for _, file := range folder.Files {
			if !skip[file.FileID] {
				pending = append(pending, file)
			}
		}
		if len(pending) == 0 {
			continue
		}
		target, err := folder_target(ctx, drive, channel_id, plan.Scope, folder)
		if err != nil {
			for _, file := range pending {
				result.Failed = append(result.Failed, ApplyFailure{FileID: file.FileID, Name: file.Name, Reason: err.Error()})
				done++
				s.emit(EventApplyProgress, ApplyProgress{Done: done, Total: total})
			}
			continue
		}
		if folder.IsNew {
			result.CreatedFolders = append(result.CreatedFolders, folder.Name)
		}
		for _, file := range pending {
			if err := ctx.Err(); err != nil {
				result.Failed = append(result.Failed, ApplyFailure{FileID: file.FileID, Name: file.Name, Reason: "cancelled"})
			} else if err := drive.MoveFile(ctx, channel_id, file.FileID, target); err != nil {
				result.Failed = append(result.Failed, ApplyFailure{FileID: file.FileID, Name: file.Name, Reason: err.Error()})
			} else {
				moves = append(moves, undo_move{
					file_id:     file.FileID,
					name:        file.Name,
					from_parent: file.FromParentID,
					to_parent:   target,
				})
			}
			done++
			s.emit(EventApplyProgress, ApplyProgress{Done: done, Total: total})
		}
	}
	result.Moved = len(moves)
	if len(moves) > 0 {
		token := uuid.NewString()
		s.store_undo(token, &undo_record{channel_id: channel_id, moves: moves, created_ms: s.now().UnixMilli()})
		result.UndoToken = token
	}
	return result, nil
}

// folder_target returns the folder id a planned folder's files go into, creating
// the folder when the plan marked it new.
func folder_target(ctx context.Context, drive Drive, channel_id int64, scope Scope, folder PlannedFolder) (string, error) {
	if !folder.IsNew {
		if folder.ExistingFolderID == "" {
			return "", fmt.Errorf("folder %q has no id", folder.Name)
		}
		return folder.ExistingFolderID, nil
	}
	id, err := drive.CreateFolder(ctx, channel_id, folder.Name, scope.FolderID)
	if err != nil {
		return "", fmt.Errorf("create folder %q: %w", folder.Name, err)
	}
	return id, nil
}

// UndoApply moves files applied by token back to where they were. A file that
// has been moved again since the apply is skipped rather than overwritten. The
// token is single use, and folders created by the apply stay in place.
func (s *Service) UndoApply(ctx context.Context, token string) (UndoResult, error) {
	record, err := s.take_undo(token)
	if err != nil {
		return UndoResult{}, err
	}
	drive, err := s.drive()
	if err != nil {
		return UndoResult{}, err
	}
	if err := drive.CanOrganize(record.channel_id); err != nil {
		return UndoResult{}, err
	}
	result := UndoResult{Skipped: []UndoIssue{}, Failed: []UndoIssue{}}
	for _, move := range record.moves {
		if err := ctx.Err(); err != nil {
			result.Failed = append(result.Failed, UndoIssue{FileID: move.file_id, Name: move.name, Reason: "cancelled"})
			continue
		}
		current, err := drive.ParentOf(ctx, record.channel_id, move.file_id)
		if err != nil {
			result.Failed = append(result.Failed, UndoIssue{FileID: move.file_id, Name: move.name, Reason: err.Error()})
			continue
		}
		if current != move.to_parent {
			result.Skipped = append(result.Skipped, UndoIssue{FileID: move.file_id, Name: move.name, Reason: "moved again since the organize step"})
			continue
		}
		if err := drive.MoveFile(ctx, record.channel_id, move.file_id, move.from_parent); err != nil {
			result.Failed = append(result.Failed, UndoIssue{FileID: move.file_id, Name: move.name, Reason: err.Error()})
			continue
		}
		result.Restored++
	}
	return result, nil
}

// claim_plan marks a plan as being applied so two applies cannot race. It is
// released again if the apply fails before any file moves.
func (s *Service) claim_plan(plan_id string) (*Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune_locked(s.now().UnixMilli())
	plan, ok := s.plans[plan_id]
	if !ok {
		return nil, ErrPlanNotFound
	}
	if plan.applied {
		return nil, errors.New("ai: this plan was already applied")
	}
	plan.applied = true
	return plan, nil
}

func (s *Service) release_plan(plan *Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan.applied = false
}

func (s *Service) store_undo(token string, record *undo_record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune_locked(s.now().UnixMilli())
	s.undos[token] = record
}

// take_undo removes and returns the record for token, so each token works once.
func (s *Service) take_undo(token string) (*undo_record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune_locked(s.now().UnixMilli())
	record, ok := s.undos[token]
	if !ok {
		return nil, errors.New("ai: undo token not found or already used")
	}
	delete(s.undos, token)
	return record, nil
}

// prune_locked drops plans and undo records past their lifetime. The caller
// holds s.mu.
func (s *Service) prune_locked(now_ms int64) {
	s.prune_action_undos_locked(now_ms)
	plan_cutoff := now_ms - plan_ttl.Milliseconds()
	for id, plan := range s.plans {
		if plan.created_ms < plan_cutoff && !plan.applied {
			delete(s.plans, id)
		}
	}
	undo_cutoff := now_ms - undo_ttl.Milliseconds()
	for token, record := range s.undos {
		if record.created_ms < undo_cutoff {
			delete(s.undos, token)
		}
	}
}
