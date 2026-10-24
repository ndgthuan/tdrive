package drive_roles

import (
	"context"
	"errors"
	"testing"
)

// The file rule: viewers are refused, managers and the owner pass, and an editor
// passes only for a file they uploaded. Every refusal is permission_denied.
func TestRequireFileChangeMatrix(t *testing.T) {
	const actor int64 = 42
	cases := []struct {
		name     string
		role     Role
		uploader int64
		wantCode Code
	}{
		{"viewer refused on own file", RoleViewer, actor, CodePermissionDenied},
		{"viewer refused on other file", RoleViewer, 9, CodePermissionDenied},
		{"editor passes own file", RoleEditor, actor, ""},
		{"editor refused on other file", RoleEditor, 9, CodePermissionDenied},
		{"editor refused when uploader unknown", RoleEditor, 0, CodePermissionDenied},
		{"manager passes other file", RoleManager, 9, ""},
		{"owner passes other file", RoleOwner, 9, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireFileChange(CapabilitiesFor(tc.role), tc.uploader, actor, "rename")
			if got := CodeOf(err); got != tc.wantCode {
				t.Fatalf("code = %q (err %v), want %q", got, err, tc.wantCode)
			}
		})
	}
}

// Trashing a folder: the editor passes only when every file inside is theirs.
func TestRequireFolderSubtreeChange(t *testing.T) {
	const actor int64 = 42
	cases := []struct {
		name      string
		role      Role
		uploaders []int64
		wantErr   bool
	}{
		{"editor, all own files", RoleEditor, []int64{actor, actor}, false},
		{"editor, one foreign file", RoleEditor, []int64{actor, 9}, true},
		{"editor, empty folder", RoleEditor, nil, false},
		{"editor, unknown uploader", RoleEditor, []int64{0}, true},
		{"manager, foreign files", RoleManager, []int64{9, 10}, false},
		{"viewer, own files", RoleViewer, []int64{actor}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireFolderSubtreeChange(CapabilitiesFor(tc.role), tc.uploaders, actor, "trash")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && CodeOf(err) != CodePermissionDenied {
				t.Fatalf("code = %q, want permission_denied", CodeOf(err))
			}
		})
	}
}

// Restoring or purging a folder is a manager action.
func TestRequireFolderChangeManagerIsManagersOnly(t *testing.T) {
	if err := RequireFolderChangeManager(CapabilitiesFor(RoleEditor), "purge"); CodeOf(err) != CodePermissionDenied {
		t.Fatalf("editor purge folder = %v, want permission_denied", err)
	}
	if err := RequireFolderChangeManager(CapabilitiesFor(RoleViewer), "restore"); CodeOf(err) != CodePermissionDenied {
		t.Fatalf("viewer restore folder = %v, want permission_denied", err)
	}
	if err := RequireFolderChangeManager(CapabilitiesFor(RoleManager), "restore"); err != nil {
		t.Fatalf("manager restore folder = %v, want nil", err)
	}
}

// The codes the UI translates are fixed strings.
func TestStableErrorCodes(t *testing.T) {
	want := map[error]Code{
		ErrViewOnly:         "permission_denied",
		ErrPermissionDenied: "permission_denied",
		ErrNotMember:        "not_a_member",
		ErrUserNotFound:     "user_not_found",
		ErrOwnerImmutable:   "cannot_change_owner",
		ErrLastManager:      "last_manager",
		ErrInvalidRole:      "invalid_role",
		ErrRoleUnavailable:  "role_unavailable",
		ErrNotSharedDrive:   "not_shared_drive",
	}
	for err, code := range want {
		if got := CodeOf(err); got != code {
			t.Fatalf("CodeOf(%v) = %q, want %q", err, got, code)
		}
	}
}

// A viewer-by-default drive shows plain members as viewers: Telegram applies the
// default restrictions to members with no record of their own.
func TestPlainMembersTakeTheDefaultRights(t *testing.T) {
	tg := new_fake_tg(5, creator_of(1, "Owner"), editor_of(5, "Me"), editor_of(6, "Bo"))
	tg.defaults = ViewerRights()
	svc, _ := new_test_service(t, tg, DriveKindShared)
	ctx := context.Background()

	caps, err := svc.MyRole(ctx, test_channel)
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if caps.Role != RoleViewer || caps.CanUpload {
		t.Fatalf("plain member under viewer default = %+v, want viewer without upload", caps)
	}

	page, err := svc.ListMembers(ctx, test_channel, "", 0, 50)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if got := member_role(page, 6); got != RoleViewer {
		t.Fatalf("member 6 role = %q, want viewer", got)
	}
	if got := member_role(page, 1); got != RoleOwner {
		t.Fatalf("owner role = %q, want owner", got)
	}
}

// With no default restriction a plain member is an editor, as before.
func TestPlainMembersAreEditorsWithoutDefaultRestriction(t *testing.T) {
	tg := new_fake_tg(5, creator_of(1, "Owner"), editor_of(5, "Me"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	caps, err := svc.MyRole(context.Background(), test_channel)
	if err != nil {
		t.Fatalf("MyRole: %v", err)
	}
	if caps.Role != RoleEditor || !caps.CanUpload {
		t.Fatalf("plain member = %+v, want editor with upload", caps)
	}
}

// Only the owner and managers may change the default, and the role must be one
// of the two a newcomer can get.
func TestSetDefaultRoleNeedsManageRightAndValidRole(t *testing.T) {
	tg := new_fake_tg(5, creator_of(1, "Owner"), editor_of(5, "Me"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	ctx := context.Background()

	if err := svc.SetDefaultRole(ctx, test_channel, RoleViewer); CodeOf(err) != CodePermissionDenied {
		t.Fatalf("editor SetDefaultRole = %v, want permission_denied", err)
	}
	tg.self = 1
	if _, err := svc.RefreshMyRole(ctx, test_channel); err != nil {
		t.Fatalf("RefreshMyRole: %v", err)
	}
	if err := svc.SetDefaultRole(ctx, test_channel, RoleManager); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("SetDefaultRole(manager) = %v, want invalid_role", err)
	}
	if err := svc.SetDefaultRole(ctx, test_channel, RoleViewer); err != nil {
		t.Fatalf("owner SetDefaultRole(viewer) = %v", err)
	}
	if got, err := svc.GetDefaultRole(ctx, test_channel); err != nil || got != RoleViewer {
		t.Fatalf("GetDefaultRole = %q, %v; want viewer", got, err)
	}
}

// A viewer cannot invite; the link is not exported.
func TestCreateInviteRefusesViewer(t *testing.T) {
	tg := new_fake_tg(5, creator_of(1, "Owner"), viewer_of(5, "Me"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	_, err := svc.CreateInvite(context.Background(), test_channel, "", false)
	if CodeOf(err) != CodePermissionDenied {
		t.Fatalf("viewer CreateInvite = %v, want permission_denied", err)
	}
	if tg.invite_calls != 0 {
		t.Fatalf("viewer exported %d invite links", tg.invite_calls)
	}
}
