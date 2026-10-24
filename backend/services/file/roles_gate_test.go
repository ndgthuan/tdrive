package file

import (
	"context"
	"fmt"
	"testing"

	"TDrive/backend/drive_roles"
	"TDrive/backend/projection"
)

// fixed_guard answers every role lookup with one capability set.
type fixed_guard struct{ caps drive_roles.Capabilities }

func (g fixed_guard) MyRole(context.Context, int64) (drive_roles.Capabilities, error) {
	return g.caps, nil
}

// A file the actor (7) uploaded, in the shared drive, for the gate checks.
const gate_msg int64 = 555

func seed_shared_file(t *testing.T, svc *Service) {
	t.Helper()
	op := projection.Op{
		Type:           projection.OpMeta,
		Obj:            fmt.Sprintf("%s%d", projection.FileIDPrefix, gate_msg),
		Parent:         projection.RootParent,
		Name:           "a.txt",
		FileSize:       10,
		FileUploadTime: 1234,
	}
	if _, err := projection.ProjectFromOp(svc.DB, sharedChannelID, gate_msg, op, 7, projection.Format(op)); err != nil {
		t.Fatalf("seed shared file: %v", err)
	}
}

func TestSharedGatesRefuseViewerEverywhere(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	seed_shared_file(t, svc)
	svc.Roles = fixed_guard{caps: drive_roles.CapabilitiesFor(drive_roles.RoleViewer)}
	ctx := context.Background()

	if err := svc.requireUploadRight(ctx, sharedChannelID); drive_roles.CodeOf(err) != drive_roles.CodePermissionDenied {
		t.Fatalf("viewer upload gate = %v, want permission_denied", err)
	}
	if err := svc.requireOwnerForShared(ctx, sharedChannelID, int(gate_msg), "rename"); drive_roles.CodeOf(err) != drive_roles.CodePermissionDenied {
		t.Fatalf("viewer rename gate = %v, want permission_denied", err)
	}
	if err := svc.requireObjectWrite(ctx, sharedChannelID, projection.FileIDPrefix+"555", "trash"); drive_roles.CodeOf(err) != drive_roles.CodePermissionDenied {
		t.Fatalf("viewer trash gate = %v, want permission_denied", err)
	}
}

// An editor may change the file they uploaded and nobody else's.
func TestSharedGatesFollowTheUploaderForEditors(t *testing.T) {
	svc, _, _, actor := newTestService(t)
	seed_shared_file(t, svc)
	svc.Roles = fixed_guard{caps: drive_roles.CapabilitiesFor(drive_roles.RoleEditor)}
	ctx := context.Background()

	if err := svc.requireOwnerForShared(ctx, sharedChannelID, int(gate_msg), "rename"); err != nil {
		t.Fatalf("editor on own file = %v, want nil", err)
	}
	*actor = 8
	if err := svc.requireOwnerForShared(ctx, sharedChannelID, int(gate_msg), "rename"); drive_roles.CodeOf(err) != drive_roles.CodePermissionDenied {
		t.Fatalf("editor on another's file = %v, want permission_denied", err)
	}
	if err := svc.requireUploadRight(ctx, sharedChannelID); err != nil {
		t.Fatalf("editor upload gate = %v, want nil", err)
	}
}

// A manager may change any file, even one another member uploaded.
func TestSharedGatesLetManagersChangeAnyFile(t *testing.T) {
	svc, _, _, actor := newTestService(t)
	seed_shared_file(t, svc)
	svc.Roles = fixed_guard{caps: drive_roles.CapabilitiesFor(drive_roles.RoleManager)}
	*actor = 8
	if err := svc.requireOwnerForShared(context.Background(), sharedChannelID, int(gate_msg), "delete"); err != nil {
		t.Fatalf("manager on another's file = %v, want nil", err)
	}
}

// The personal drive is never gated, whatever role the guard reports.
func TestPersonalDriveIsNeverGated(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	svc.Roles = fixed_guard{caps: drive_roles.CapabilitiesFor(drive_roles.RoleViewer)}
	if err := svc.requireUploadRight(context.Background(), personalChannelID); err != nil {
		t.Fatalf("personal drive upload gate = %v, want nil", err)
	}
}
