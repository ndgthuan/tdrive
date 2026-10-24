// Copyright (c) 2026 Nguyễn Dương Gia Thuận

package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// planned_photo_setup plans the photo_drive fixture and returns the service,
// drive, event log and plan.
func planned_photo_setup(t *testing.T) (*Service, *fake_drive, *event_log, Plan) {
	t.Helper()
	drive := photo_drive(t)
	fake := new_fake_vision_server(t, photo_rule)
	svc, log := new_organize_service(t, drive, fake.server.URL+"/v1", true)
	plan, err := svc.PlanOrganize(context.Background(), plan_request(""))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return svc, drive, log, plan
}

func TestApplyPlanMovesFilesAndCreatesFolders(t *testing.T) {
	svc, drive, log, plan := planned_photo_setup(t)

	// The user removes the food photo from the plan before applying.
	result, err := svc.ApplyPlan(context.Background(), plan.PlanID, []int64{5})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Moved != 5 || len(result.Failed) != 0 {
		t.Fatalf("moved=%d failed=%v", result.Moved, result.Failed)
	}
	if len(result.CreatedFolders) != 1 || result.CreatedFolders[0] != "City" {
		t.Fatalf("only City should be created (Food was excluded), got %v", result.CreatedFolders)
	}
	if result.UndoToken == "" {
		t.Fatalf("apply that moved files must return an undo token")
	}
	if drive.file_parent(5) != "" {
		t.Fatalf("excluded file must not move")
	}
	if drive.file_parent(1) != "beach" || drive.file_parent(8) != "beach" {
		t.Fatalf("beach photos should land in the existing Beach folder")
	}
	city := drive.file_parent(3)
	if city == "" || city == "missing" || !strings.HasPrefix(city, "new-") {
		t.Fatalf("city photo should land in the created folder, got %q", city)
	}
	if drive.file_parent(3) != drive.file_parent(7) {
		t.Fatalf("city photos should share one folder")
	}

	progress := log.names("ai:apply_progress")
	last := progress[len(progress)-1].(ApplyProgress)
	if last.Done != 5 || last.Total != 5 {
		t.Fatalf("final apply progress = %+v", last)
	}

	if _, err := svc.ApplyPlan(context.Background(), plan.PlanID, nil); err == nil || !strings.Contains(err.Error(), "already applied") {
		t.Fatalf("a plan must not apply twice, got %v", err)
	}
	if _, err := svc.ApplyPlan(context.Background(), "missing", nil); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("unknown plan should be ErrPlanNotFound, got %v", err)
	}
}

func TestApplyPlanReportsFailuresAndContinues(t *testing.T) {
	svc, drive, _, plan := planned_photo_setup(t)
	drive.fail_move[4] = true

	result, err := svc.ApplyPlan(context.Background(), plan.PlanID, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.Failed) != 1 || result.Failed[0].FileID != 4 {
		t.Fatalf("file 4 should be the only failure, got %+v", result.Failed)
	}
	if result.Moved != 5 {
		t.Fatalf("other files should still move, moved=%d", result.Moved)
	}
}

func TestUndoApplyRestoresFiles(t *testing.T) {
	svc, drive, _, plan := planned_photo_setup(t)
	applied, err := svc.ApplyPlan(context.Background(), plan.PlanID, []int64{5})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	undone, err := svc.UndoApply(context.Background(), applied.UndoToken)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if undone.Restored != 5 || len(undone.Skipped) != 0 || len(undone.Failed) != 0 {
		t.Fatalf("undo result = %+v", undone)
	}
	if drive.file_parent(1) != "" || drive.file_parent(8) != "trips" || drive.file_parent(3) != "" || drive.file_parent(7) != "trips" {
		t.Fatalf("files should return to their original folders")
	}
	if _, err := svc.UndoApply(context.Background(), applied.UndoToken); err == nil {
		t.Fatalf("undo token must be single use")
	}
}

func TestUndoApplySkipsFilesMovedSinceApply(t *testing.T) {
	svc, drive, _, plan := planned_photo_setup(t)
	applied, err := svc.ApplyPlan(context.Background(), plan.PlanID, []int64{5})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	// The user moves city_1 again after the organize step.
	if err := drive.MoveFile(context.Background(), 42, 3, "trips"); err != nil {
		t.Fatalf("manual move: %v", err)
	}

	undone, err := svc.UndoApply(context.Background(), applied.UndoToken)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if len(undone.Skipped) != 1 || undone.Skipped[0].FileID != 3 {
		t.Fatalf("file 3 should be skipped, got %+v", undone.Skipped)
	}
	if drive.file_parent(3) != "trips" {
		t.Fatalf("a file moved by the user must not be overwritten by undo")
	}
	if undone.Restored != 4 {
		t.Fatalf("restored = %d, want 4", undone.Restored)
	}
}

func TestApplyPlanRefusesReadOnlySourceAtApplyTime(t *testing.T) {
	svc, drive, _, plan := planned_photo_setup(t)
	drive.read_only = true
	if _, err := svc.ApplyPlan(context.Background(), plan.PlanID, nil); err == nil {
		t.Fatalf("apply must re-check the read-only rule")
	}
	drive.read_only = false
	if _, err := svc.ApplyPlan(context.Background(), plan.PlanID, nil); err != nil {
		t.Fatalf("plan should still be applicable after a refused attempt: %v", err)
	}
}
