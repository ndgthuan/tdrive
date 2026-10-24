package drive_roles

import (
	"context"
	"errors"
	"testing"
	"time"

	"TDrive/backend/tgclient"
)

const test_channel int64 = 100

type fake_clock struct{ now time.Time }

func (c *fake_clock) Now() time.Time { return c.now }

// new_test_service wires a Service to a fake Telegram channel. The clock is
// fixed so cache expiry is driven explicitly by the tests.
func new_test_service(t *testing.T, tg *fake_tg, kind DriveKind) (*Service, *fake_clock) {
	t.Helper()
	clock := &fake_clock{now: time.Unix(1_700_000_000, 0)}
	svc := NewService(
		func(int64) (DriveKind, error) { return kind, nil },
		func(_ context.Context, channel_id int64) (tgclient.InputPeer, error) {
			return tgclient.InputPeer{ChannelID: channel_id, AccessHash: 7}, nil
		},
		func(context.Context) (int64, error) { return tg.self, nil },
		tg,
	)
	svc.Now = clock.Now
	return svc, clock
}

// The permission matrix for SetRole, checked on the pure rule function.
func TestSetRolePermissionMatrix(t *testing.T) {
	cases := []struct {
		name      string
		actor     Role
		current   Role
		requested Role
		is_self   bool
		managers  int
		want      error
	}{
		{"owner promotes an editor to manager", RoleOwner, RoleEditor, RoleManager, false, 1, nil},
		{"owner demotes a manager to viewer", RoleOwner, RoleManager, RoleViewer, false, 2, nil},
		{"owner demotes the last manager", RoleOwner, RoleManager, RoleEditor, false, 1, nil},
		{"owner cannot change the owner", RoleOwner, RoleOwner, RoleEditor, false, 1, ErrOwnerImmutable},
		{"manager sets an editor to viewer", RoleManager, RoleEditor, RoleViewer, false, 2, nil},
		{"manager lifts a viewer to editor", RoleManager, RoleViewer, RoleEditor, false, 2, nil},
		{"manager cannot promote to manager", RoleManager, RoleEditor, RoleManager, false, 2, ErrPermissionDenied},
		{"manager cannot demote another manager", RoleManager, RoleManager, RoleEditor, false, 2, ErrPermissionDenied},
		{"manager cannot touch the owner", RoleManager, RoleOwner, RoleViewer, false, 2, ErrOwnerImmutable},
		{"manager may step down with another manager left", RoleManager, RoleManager, RoleEditor, true, 2, nil},
		{"last manager cannot step down", RoleManager, RoleManager, RoleEditor, true, 1, ErrLastManager},
		{"editor cannot change roles", RoleEditor, RoleViewer, RoleEditor, false, 1, ErrPermissionDenied},
		{"viewer cannot change roles", RoleViewer, RoleEditor, RoleViewer, false, 1, ErrPermissionDenied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := check_role_change(tc.actor, tc.current, tc.requested, tc.is_self, tc.managers)
			if !errors.Is(got, tc.want) && got != tc.want {
				t.Fatalf("check_role_change = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSetRoleEditorToViewerWritesRestrictions(t *testing.T) {
	tg := new_fake_tg(1, creator_of(1, "Owner"), editor_of(2, "Eve"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	if err := svc.SetRole(context.Background(), test_channel, 2, RoleViewer); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	if got := tg.calls; len(got) != 1 || got[0] != "editBanned 2 viewer" {
		t.Fatalf("calls = %v", got)
	}
	page, err := svc.ListMembers(context.Background(), test_channel, "", 0, 10)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if role := member_role(page, 2); role != RoleViewer {
		t.Fatalf("user 2 role after change = %q", role)
	}
}

func TestSetRoleManagerToViewerClearsAdminFirst(t *testing.T) {
	tg := new_fake_tg(1, creator_of(1, "Owner"), manager_of(2, "Max"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	if err := svc.SetRole(context.Background(), test_channel, 2, RoleViewer); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	want := []string{"editAdmin 2 clear", "editBanned 2 viewer"}
	if len(tg.calls) != len(want) || tg.calls[0] != want[0] || tg.calls[1] != want[1] {
		t.Fatalf("calls = %v, want %v", tg.calls, want)
	}
}

func TestSetRoleViewerToManagerLiftsRestrictionFirst(t *testing.T) {
	tg := new_fake_tg(1, creator_of(1, "Owner"), viewer_of(2, "Vi"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	if err := svc.SetRole(context.Background(), test_channel, 2, RoleManager); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	want := []string{"editBanned 2 lift", "editAdmin 2 manager"}
	if len(tg.calls) != len(want) || tg.calls[0] != want[0] || tg.calls[1] != want[1] {
		t.Fatalf("calls = %v, want %v", tg.calls, want)
	}
}

func TestSetRoleRejectsOwnerAndInvalidRoles(t *testing.T) {
	tg := new_fake_tg(1, creator_of(1, "Owner"), editor_of(2, "Eve"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	ctx := context.Background()
	if err := svc.SetRole(ctx, test_channel, 1, RoleEditor); !errors.Is(err, ErrOwnerImmutable) {
		t.Fatalf("changing owner: %v", err)
	}
	if err := svc.SetRole(ctx, test_channel, 2, RoleOwner); !errors.Is(err, ErrOwnerImmutable) {
		t.Fatalf("assigning owner: %v", err)
	}
	if err := svc.SetRole(ctx, test_channel, 2, Role("admin")); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("invalid role: %v", err)
	}
	if err := svc.SetRole(ctx, test_channel, 0, RoleEditor); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user id: %v", err)
	}
	if len(tg.calls) != 0 {
		t.Fatalf("refused changes must not reach Telegram, calls = %v", tg.calls)
	}
}

func TestSetRoleForNonMemberIsNotMember(t *testing.T) {
	tg := new_fake_tg(1, creator_of(1, "Owner"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	if err := svc.SetRole(context.Background(), test_channel, 77, RoleEditor); !errors.Is(err, ErrNotMember) {
		t.Fatalf("SetRole for stranger = %v, want not_member", err)
	}
}

func TestManagerCannotSetRoleOfOtherManagerThroughService(t *testing.T) {
	tg := new_fake_tg(2, creator_of(1, "Owner"), manager_of(2, "Max"), manager_of(3, "Mia"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	err := svc.SetRole(context.Background(), test_channel, 3, RoleViewer)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("manager demoting manager = %v", err)
	}
	if len(tg.calls) != 0 {
		t.Fatalf("denied change reached Telegram: %v", tg.calls)
	}
}

func TestLastManagerCannotStepDownThroughService(t *testing.T) {
	tg := new_fake_tg(2, creator_of(1, "Owner"), manager_of(2, "Max"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	if err := svc.SetRole(context.Background(), test_channel, 2, RoleEditor); !errors.Is(err, ErrLastManager) {
		t.Fatalf("last manager stepping down = %v", err)
	}
}

func TestViewerCannotManageMembers(t *testing.T) {
	tg := new_fake_tg(2, creator_of(1, "Owner"), viewer_of(2, "Vi"), editor_of(3, "Eve"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	ctx := context.Background()
	if err := svc.SetRole(ctx, test_channel, 3, RoleViewer); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("viewer SetRole = %v", err)
	}
	if err := svc.RemoveMember(ctx, test_channel, 3, false); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("viewer RemoveMember = %v", err)
	}
	if _, err := svc.CreateInvite(ctx, test_channel, "", false); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("editor CreateInvite = %v", err)
	}
}

func TestMemberOperationsRequireSharedDrive(t *testing.T) {
	tg := new_fake_tg(1, creator_of(1, "Owner"))
	ctx := context.Background()
	for _, kind := range []DriveKind{DriveKindPersonal, DriveKindReadOnly} {
		svc, _ := new_test_service(t, tg, kind)
		if err := svc.SetRole(ctx, test_channel, 1, RoleEditor); !errors.Is(err, ErrNotSharedDrive) {
			t.Fatalf("kind %d SetRole = %v", kind, err)
		}
		if _, err := svc.ListMembers(ctx, test_channel, "", 0, 10); !errors.Is(err, ErrNotSharedDrive) {
			t.Fatalf("kind %d ListMembers = %v", kind, err)
		}
	}
}

func TestRemoveMemberKicksAndOptionallyLiftsBan(t *testing.T) {
	ctx := context.Background()

	tg := new_fake_tg(1, creator_of(1, "Owner"), editor_of(2, "Eve"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	if err := svc.RemoveMember(ctx, test_channel, 2, true); err != nil {
		t.Fatalf("RemoveMember ban: %v", err)
	}
	if got := tg.calls; len(got) != 1 || got[0] != "editBanned 2 kick" {
		t.Fatalf("ban calls = %v", got)
	}
	if p := tg.people[2]; p.IsMember() {
		t.Fatal("banned member must not remain a member")
	}

	tg2 := new_fake_tg(1, creator_of(1, "Owner"), editor_of(2, "Eve"))
	svc2, _ := new_test_service(t, tg2, DriveKindShared)
	if err := svc2.RemoveMember(ctx, test_channel, 2, false); err != nil {
		t.Fatalf("RemoveMember kick: %v", err)
	}
	if p := tg2.people[2]; p.Kind != KindMember {
		t.Fatalf("kicked member without ban must be able to rejoin, kind = %q", p.Kind)
	}
}

func TestRemoveMemberRules(t *testing.T) {
	ctx := context.Background()
	tg := new_fake_tg(2, creator_of(1, "Owner"), manager_of(2, "Max"), manager_of(3, "Mia"), editor_of(4, "Eve"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	if err := svc.RemoveMember(ctx, test_channel, 1, true); !errors.Is(err, ErrOwnerImmutable) {
		t.Fatalf("removing owner = %v", err)
	}
	if err := svc.RemoveMember(ctx, test_channel, 3, true); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("manager removing manager = %v", err)
	}
	if err := svc.RemoveMember(ctx, test_channel, 2, true); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("removing self = %v", err)
	}

	// The owner may remove a manager: admin rights are cleared first.
	owner := new_fake_tg(1, creator_of(1, "Owner"), manager_of(3, "Mia"))
	svc_owner, _ := new_test_service(t, owner, DriveKindShared)
	if err := svc_owner.RemoveMember(ctx, test_channel, 3, true); err != nil {
		t.Fatalf("owner removing manager: %v", err)
	}
	if got := owner.calls; len(got) != 2 || got[0] != "editAdmin 3 clear" || got[1] != "editBanned 3 kick" {
		t.Fatalf("owner removal calls = %v", got)
	}
}

func TestListMembersPagesSortedByRoleAndHidesKicked(t *testing.T) {
	kicked := Participant{UserID: 9, AccessHash: 90, DisplayName: "Kicked", Kind: KindBanned, BannedRights: BannedRights{ViewMessages: true}}
	tg := new_fake_tg(3,
		editor_of(5, "Zed"),
		viewer_of(4, "Vi"),
		manager_of(2, "Max"),
		creator_of(1, "Owner"),
		editor_of(3, "Ann"),
		kicked,
	)
	svc, _ := new_test_service(t, tg, DriveKindShared)
	ctx := context.Background()

	first, err := svc.ListMembers(ctx, test_channel, "", 0, 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if first.Total != 5 || !first.HasMore || first.NextOffset != 2 {
		t.Fatalf("first page paging = %+v", first)
	}
	if first.Members[0].Role != RoleOwner || first.Members[1].Role != RoleManager {
		t.Fatalf("first page order = %+v", first.Members)
	}
	if !first.Members[0].IsSelf && first.Members[0].UserID != 1 {
		t.Fatalf("owner row = %+v", first.Members[0])
	}

	rest, err := svc.ListMembers(ctx, test_channel, "", first.NextOffset, 10)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if rest.HasMore || len(rest.Members) != 3 {
		t.Fatalf("second page = %+v", rest)
	}
	if role := member_role(rest, 4); role != RoleViewer {
		t.Fatalf("viewer row role = %q", role)
	}
	for _, m := range append(first.Members, rest.Members...) {
		if m.UserID == 9 {
			t.Fatal("kicked member must not be listed")
		}
	}
}

func TestListMembersSearchFiltersByName(t *testing.T) {
	tg := new_fake_tg(1, creator_of(1, "Owner"), editor_of(2, "Anna"), editor_of(3, "Bob"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	page, err := svc.ListMembers(context.Background(), test_channel, "ann", 0, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Members) != 1 || page.Members[0].UserID != 2 {
		t.Fatalf("search members = %+v", page.Members)
	}
}

func TestListMembersMarksSelf(t *testing.T) {
	tg := new_fake_tg(2, creator_of(1, "Owner"), editor_of(2, "Me"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	page, err := svc.ListMembers(context.Background(), test_channel, "", 0, 10)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	for _, m := range page.Members {
		if m.IsSelf != (m.UserID == 2) {
			t.Fatalf("IsSelf wrong for %+v", m)
		}
	}
}

func TestMyRoleCachesAndRefreshReportsChanges(t *testing.T) {
	ctx := context.Background()
	tg := new_fake_tg(2, creator_of(1, "Owner"), editor_of(2, "Me"))
	svc, clock := new_test_service(t, tg, DriveKindShared)
	var changed []Role
	svc.OnRoleChanged = func(channel_id int64, role Role) {
		if channel_id != test_channel {
			t.Errorf("event for channel %d", channel_id)
		}
		changed = append(changed, role)
	}

	caps, err := svc.MyRole(ctx, test_channel)
	if err != nil || caps.Role != RoleEditor || !caps.CanUpload || caps.CanDeleteOthers {
		t.Fatalf("MyRole = %+v, %v", caps, err)
	}
	if _, err := svc.MyRole(ctx, test_channel); err != nil {
		t.Fatalf("second MyRole: %v", err)
	}
	if tg.self_calls != 1 {
		t.Fatalf("cached MyRole asked Telegram %d times", tg.self_calls)
	}

	clock.now = clock.now.Add(2 * time.Minute)
	tg.people[2] = viewer_of(2, "Me")
	caps, err = svc.RefreshMyRole(ctx, test_channel)
	if err != nil || caps.Role != RoleViewer || caps.CanUpload {
		t.Fatalf("RefreshMyRole after demotion = %+v, %v", caps, err)
	}
	if len(changed) != 1 || changed[0] != RoleViewer {
		t.Fatalf("role change events = %v", changed)
	}
}

func TestMyRoleFallsBackToLastKnownWhenTelegramIsDown(t *testing.T) {
	ctx := context.Background()
	tg := new_fake_tg(2, creator_of(1, "Owner"), manager_of(2, "Me"))
	svc, clock := new_test_service(t, tg, DriveKindShared)
	if _, err := svc.MyRole(ctx, test_channel); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	clock.now = clock.now.Add(2 * time.Minute)
	tg.fail_self = errors.New("network down")
	caps, err := svc.MyRole(ctx, test_channel)
	if err != nil || caps.Role != RoleManager {
		t.Fatalf("offline MyRole = %+v, %v; want last known manager", caps, err)
	}

	cold := new_fake_tg(2, creator_of(1, "Owner"), manager_of(2, "Me"))
	cold.fail_self = errors.New("network down")
	cold_svc, _ := new_test_service(t, cold, DriveKindShared)
	if _, err := cold_svc.MyRole(ctx, test_channel); !errors.Is(err, ErrRoleUnavailable) {
		t.Fatalf("offline with no cache = %v, want role_unavailable", err)
	}
}

func TestMyRoleForPersonalAndReadOnlyDrivesMakesNoCall(t *testing.T) {
	tg := new_fake_tg(2)
	ctx := context.Background()
	personal, _ := new_test_service(t, tg, DriveKindPersonal)
	caps, err := personal.MyRole(ctx, test_channel)
	if err != nil || caps.Role != RoleOwner || !caps.CanDeleteOthers || !caps.CanManageMembers || !caps.CanInvite {
		t.Fatalf("personal MyRole = %+v, %v", caps, err)
	}
	source, _ := new_test_service(t, tg, DriveKindReadOnly)
	caps, err = source.MyRole(ctx, test_channel)
	if err != nil || caps.CanUpload {
		t.Fatalf("source MyRole = %+v, %v", caps, err)
	}
	if tg.self_calls != 0 {
		t.Fatalf("personal and source drives asked Telegram %d times", tg.self_calls)
	}
}

func TestMyRoleForNonMemberIsNotMember(t *testing.T) {
	tg := new_fake_tg(9, creator_of(1, "Owner"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	if _, err := svc.MyRole(context.Background(), test_channel); !errors.Is(err, ErrNotMember) {
		t.Fatalf("MyRole for non-member = %v", err)
	}
}

func TestSetDefaultRoleAndGetDefaultRole(t *testing.T) {
	ctx := context.Background()
	tg := new_fake_tg(1, creator_of(1, "Owner"), manager_of(2, "Max"))
	svc, _ := new_test_service(t, tg, DriveKindShared)

	if err := svc.SetDefaultRole(ctx, test_channel, RoleViewer); err != nil {
		t.Fatalf("SetDefaultRole viewer: %v", err)
	}
	if role, err := svc.GetDefaultRole(ctx, test_channel); err != nil || role != RoleViewer {
		t.Fatalf("GetDefaultRole after viewer = %q, %v", role, err)
	}
	if err := svc.SetDefaultRole(ctx, test_channel, RoleEditor); err != nil {
		t.Fatalf("SetDefaultRole editor: %v", err)
	}
	if role, err := svc.GetDefaultRole(ctx, test_channel); err != nil || role != RoleEditor {
		t.Fatalf("GetDefaultRole after editor = %q, %v", role, err)
	}
	if err := svc.SetDefaultRole(ctx, test_channel, RoleManager); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("default manager = %v", err)
	}

	// A manager may set the default; an editor may not.
	manager_svc, _ := new_test_service(t, new_fake_tg(2, creator_of(1, "Owner"), manager_of(2, "Max")), DriveKindShared)
	if err := manager_svc.SetDefaultRole(ctx, test_channel, RoleViewer); err != nil {
		t.Fatalf("manager SetDefaultRole: %v", err)
	}
	editor_svc, _ := new_test_service(t, new_fake_tg(2, creator_of(1, "Owner"), editor_of(2, "Eve")), DriveKindShared)
	if err := editor_svc.SetDefaultRole(ctx, test_channel, RoleViewer); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("editor SetDefaultRole = %v", err)
	}
}

func TestCreateInviteAppliesDefaultAndReturnsLink(t *testing.T) {
	ctx := context.Background()
	tg := new_fake_tg(1, creator_of(1, "Owner"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	result, err := svc.CreateInvite(ctx, test_channel, RoleViewer, true)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if result.Link != "https://t.me/+fake" || result.DefaultRole != RoleViewer {
		t.Fatalf("CreateInvite result = %+v", result)
	}
	if !tg.invite_flag {
		t.Fatal("approval flag was not passed to Telegram")
	}
	if tg.default_calls != 1 {
		t.Fatalf("default role writes = %d, want 1", tg.default_calls)
	}
}

func TestCreateInviteWithoutDefaultLeavesDefaultAlone(t *testing.T) {
	tg := new_fake_tg(1, creator_of(1, "Owner"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	result, err := svc.CreateInvite(context.Background(), test_channel, "", false)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if tg.default_calls != 0 || result.DefaultRole != RoleEditor {
		t.Fatalf("default writes = %d, result = %+v", tg.default_calls, result)
	}
}

func TestMemberSnapshotIsCachedUntilAWriteInvalidatesIt(t *testing.T) {
	ctx := context.Background()
	tg := new_fake_tg(1, creator_of(1, "Owner"), editor_of(2, "Eve"))
	svc, _ := new_test_service(t, tg, DriveKindShared)
	if _, err := svc.ListMembers(ctx, test_channel, "", 0, 10); err != nil {
		t.Fatalf("first list: %v", err)
	}
	before := tg.list_calls
	if _, err := svc.ListMembers(ctx, test_channel, "", 0, 10); err != nil {
		t.Fatalf("second list: %v", err)
	}
	if tg.list_calls != before {
		t.Fatalf("cached list asked Telegram again")
	}
	if err := svc.SetRole(ctx, test_channel, 2, RoleViewer); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	page, err := svc.ListMembers(ctx, test_channel, "", 0, 10)
	if err != nil {
		t.Fatalf("list after write: %v", err)
	}
	if role := member_role(page, 2); role != RoleViewer {
		t.Fatalf("stale member list after write: role = %q", role)
	}
}

func member_role(page MemberPage, user_id int64) Role {
	for _, m := range page.Members {
		if m.UserID == user_id {
			return m.Role
		}
	}
	return ""
}
