package drive_roles

import (
	"errors"
	"testing"
)

// Role mapping is checked in both directions: Telegram rights to a role, and a
// role to the rights written for it. The expected values here are spelled out
// from the product rules, not computed by the functions under test.
func TestRoleOfParticipantFromTelegramRights(t *testing.T) {
	cases := []struct {
		name   string
		p      Participant
		role   Role
		member bool
	}{
		{"creator is owner", Participant{Kind: KindCreator}, RoleOwner, true},
		{"admin with manager rights is manager", Participant{Kind: KindAdmin, AdminRights: AdminRights{DeleteMessages: true, PinMessages: true, InviteUsers: true, BanUsers: true}}, RoleManager, true},
		{"admin with only ban_users is manager", Participant{Kind: KindAdmin, AdminRights: AdminRights{BanUsers: true}}, RoleManager, true},
		{"admin with only change_info is editor", Participant{Kind: KindAdmin, AdminRights: AdminRights{ChangeInfo: true}}, RoleEditor, true},
		{"plain member is editor", Participant{Kind: KindMember}, RoleEditor, true},
		{"restricted from sending is viewer", Participant{Kind: KindBanned, BannedRights: ViewerRights()}, RoleViewer, true},
		{"restricted from stickers only is editor", Participant{Kind: KindBanned, BannedRights: BannedRights{SendStickers: true}}, RoleEditor, true},
		{"kicked member is not a member", Participant{Kind: KindBanned, BannedRights: BannedRights{ViewMessages: true}}, "", false},
		{"left member is not a member", Participant{Kind: KindLeft}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role, member := RoleOf(tc.p)
			if member != tc.member || role != tc.role {
				t.Fatalf("RoleOf() = (%q, %t), want (%q, %t)", role, member, tc.role, tc.member)
			}
		})
	}
}

func TestManagerRightsAreTheExactModerationSet(t *testing.T) {
	want := AdminRights{DeleteMessages: true, PinMessages: true, InviteUsers: true, BanUsers: true}
	if got := ManagerRights(); got != want {
		t.Fatalf("ManagerRights() = %+v, want %+v", got, want)
	}
	if ManagerRights().AddAdmins || ManagerRights().ChangeInfo {
		t.Fatal("managers must not hold add_admins or change_info")
	}
}

func TestRoleToRightsRoundTrip(t *testing.T) {
	// A manager's rights read back as a manager; an editor's rights are empty
	// and read back as an editor.
	if got, _ := RoleOf(Participant{Kind: KindAdmin, AdminRights: AdminRightsFor(RoleManager)}); got != RoleManager {
		t.Fatalf("manager rights read back as %q", got)
	}
	if rights := AdminRightsFor(RoleEditor); rights != (AdminRights{}) {
		t.Fatalf("editor admin rights = %+v, want none", rights)
	}
	if got, _ := RoleOf(Participant{Kind: KindBanned, BannedRights: BannedRightsFor(RoleViewer)}); got != RoleViewer {
		t.Fatalf("viewer restrictions read back as %q", got)
	}
	for _, role := range []Role{RoleOwner, RoleManager, RoleEditor} {
		if rights := BannedRightsFor(role); rights != (BannedRights{}) {
			t.Fatalf("%s must carry no restrictions, got %+v", role, rights)
		}
	}
}

func TestViewerRightsBanSendsButKeepViewing(t *testing.T) {
	rights := ViewerRights()
	if rights.ViewMessages {
		t.Fatal("viewers must keep view_messages")
	}
	sends := map[string]bool{
		"send_messages": rights.SendMessages,
		"send_media":    rights.SendMedia,
		"send_stickers": rights.SendStickers,
		"send_gifs":     rights.SendGifs,
		"send_games":    rights.SendGames,
		"send_inline":   rights.SendInline,
		"embed_links":   rights.EmbedLinks,
		"send_polls":    rights.SendPolls,
	}
	for name, banned := range sends {
		if !banned {
			t.Errorf("viewer rights must ban %s", name)
		}
	}
}

func TestCapabilitiesForEachRole(t *testing.T) {
	cases := []struct {
		role                                          Role
		upload, delete_others, manage_members, invite bool
	}{
		{RoleOwner, true, true, true, true},
		{RoleManager, true, true, true, true},
		{RoleEditor, true, false, false, false},
		{RoleViewer, false, false, false, false},
	}
	for _, tc := range cases {
		got := CapabilitiesFor(tc.role)
		if got.Role != tc.role || got.CanUpload != tc.upload || got.CanDeleteOthers != tc.delete_others ||
			got.CanManageMembers != tc.manage_members || got.CanInvite != tc.invite {
			t.Errorf("CapabilitiesFor(%s) = %+v", tc.role, got)
		}
	}
}

func TestPlanRoleChangeOrdersAdminBeforeRestrictions(t *testing.T) {
	cases := []struct {
		current, requested Role
		want               []role_step
	}{
		{RoleEditor, RoleViewer, []role_step{step_restrict_viewer}},
		{RoleViewer, RoleEditor, []role_step{step_lift_restrictions}},
		{RoleEditor, RoleManager, []role_step{step_grant_manager}},
		{RoleManager, RoleEditor, []role_step{step_clear_admin}},
		{RoleManager, RoleViewer, []role_step{step_clear_admin, step_restrict_viewer}},
		{RoleViewer, RoleManager, []role_step{step_lift_restrictions, step_grant_manager}},
	}
	for _, tc := range cases {
		got := plan_role_change(tc.current, tc.requested)
		if len(got) != len(tc.want) {
			t.Fatalf("plan(%s -> %s) = %v, want %v", tc.current, tc.requested, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("plan(%s -> %s) = %v, want %v", tc.current, tc.requested, got, tc.want)
			}
		}
	}
}

func TestErrorCodesMatchWithErrorsIs(t *testing.T) {
	wrapped := wrap_error(ErrNotMember, errors.New("USER_NOT_PARTICIPANT"))
	if !errors.Is(wrapped, ErrNotMember) {
		t.Fatal("wrapped error must match its sentinel by code")
	}
	if errors.Is(wrapped, ErrViewOnly) {
		t.Fatal("different codes must not match")
	}
	if got := CodeOf(NotUploaderError("delete")); got != CodePermissionDenied {
		t.Fatalf("CodeOf(uploader error) = %q", got)
	}
	if got := CodeOf(errors.New("plain")); got != "" {
		t.Fatalf("CodeOf(plain error) = %q, want empty", got)
	}
}
