package app

import (
	"context"

	"TDrive/backend/drive_roles"
)

// DriveMembersService is the Wails surface for shared-drive roles and members.
// Roles are Telegram's own permissions: the owner is the creator, managers are
// admins, editors are plain members, and viewers are members whose send rights
// are banned. Every call answers from Telegram (with a short cache) and never
// from local state, so the screen cannot show a role Telegram would not enforce.
//
// Failures come back in the common OperationResult envelope. Its error code is
// one of the drive_roles codes (permission_denied, user_not_found, not_a_member,
// cannot_change_owner, last_manager, invalid_role, role_unavailable,
// not_shared_drive) or one of the operation codes; the message is display text.
type DriveMembersService struct {
	host serviceHost
}

func newDriveMembersService(host serviceHost) *DriveMembersService {
	return &DriveMembersService{host: host}
}

// DriveRoleResult is the caller's role in one drive and what that role may do.
// A personal drive is always owner; a channel source is always viewer.
type DriveRoleResult struct {
	Result           OperationResult `json:"result"`
	Role             string          `json:"role"`
	CanUpload        bool            `json:"can_upload"`
	CanDeleteOthers  bool            `json:"can_delete_others"`
	CanManageMembers bool            `json:"can_manage_members"`
	CanInvite        bool            `json:"can_invite"`
}

// DriveMemberView is one member row. Role is owner, manager, editor or viewer.
type DriveMemberView struct {
	UserID      int64  `json:"user_id"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	JoinedAt    int64  `json:"joined_at"`
	IsSelf      bool   `json:"is_self"`
}

// DriveMembersResult is one page of members, sorted by role, then name.
type DriveMembersResult struct {
	Result     OperationResult   `json:"result"`
	Members    []DriveMemberView `json:"members"`
	Total      int               `json:"total"`
	NextOffset int               `json:"next_offset"`
	HasMore    bool              `json:"has_more"`
}

// DriveInviteResult is a new invite link and the role newcomers start with.
type DriveInviteResult struct {
	Result      OperationResult `json:"result"`
	Link        string          `json:"link"`
	DefaultRole string          `json:"default_role"`
}

// DriveDefaultRoleResult is the role newcomers get: editor or viewer.
type DriveDefaultRoleResult struct {
	Result OperationResult `json:"result"`
	Role   string          `json:"role"`
}

// roles returns the shared-drive role service, or the standard not-ready error
// while the engine is still starting.
func (m *DriveMembersService) roles(ctx context.Context) (*drive_roles.Service, error) {
	if m == nil || m.host == nil {
		return nil, errBackendUnavailable
	}
	if err := m.host.WaitForBackendReady(ctx); err != nil {
		return nil, err
	}
	engine := m.host.coreEngine()
	if engine == nil {
		return nil, errBackendUnavailable
	}
	svc := engine.DriveRoles()
	if svc == nil {
		return nil, errBackendUnavailable
	}
	return svc, nil
}

func role_result(caps drive_roles.Capabilities, err error) DriveRoleResult {
	if err != nil {
		return DriveRoleResult{Result: operationFailure(err)}
	}
	return DriveRoleResult{
		Result:           operationSuccess(),
		Role:             string(caps.Role),
		CanUpload:        caps.CanUpload,
		CanDeleteOthers:  caps.CanDeleteOthers,
		CanManageMembers: caps.CanManageMembers,
		CanInvite:        caps.CanInvite,
	}
}

// MyRole returns the caller's role in drive driveID. Results are cached for about
// a minute; a role change seen by a fresh answer emits drive:role_changed.
func (m *DriveMembersService) MyRole(ctx context.Context, driveID int64) DriveRoleResult {
	svc, err := m.roles(ctx)
	if err != nil {
		return role_result(drive_roles.Capabilities{}, err)
	}
	caps, err := svc.MyRole(ctx, driveID)
	return role_result(caps, err)
}

// RefreshMyRole asks Telegram for the caller's role now, bypassing the cache.
// The frontend calls it when a shared drive is opened or the window regains focus.
func (m *DriveMembersService) RefreshMyRole(ctx context.Context, driveID int64) DriveRoleResult {
	svc, err := m.roles(ctx)
	if err != nil {
		return role_result(drive_roles.Capabilities{}, err)
	}
	caps, err := svc.RefreshMyRole(ctx, driveID)
	return role_result(caps, err)
}

// ListMembers returns one page of a shared drive's members. An empty query lists
// everyone; otherwise names and usernames are searched. limit defaults to 50 and
// is capped at 200.
func (m *DriveMembersService) ListMembers(ctx context.Context, driveID int64, query string, offset, limit int) DriveMembersResult {
	svc, err := m.roles(ctx)
	if err != nil {
		return DriveMembersResult{Result: operationFailure(err)}
	}
	page, err := svc.ListMembers(ctx, driveID, query, offset, limit)
	if err != nil {
		return DriveMembersResult{Result: operationFailure(err)}
	}
	members := make([]DriveMemberView, 0, len(page.Members))
	for _, member := range page.Members {
		members = append(members, DriveMemberView{
			UserID:      member.UserID,
			DisplayName: member.DisplayName,
			Username:    member.Username,
			Role:        string(member.Role),
			JoinedAt:    member.JoinedAt,
			IsSelf:      member.IsSelf,
		})
	}
	return DriveMembersResult{
		Result:     operationSuccess(),
		Members:    members,
		Total:      page.Total,
		NextOffset: page.NextOffset,
		HasMore:    page.HasMore,
	}
}

// SetRole changes one member's role to "manager", "editor" or "viewer". Only the
// owner makes managers; managers and the owner set editors and viewers. The owner
// cannot be changed, and the last manager cannot step down.
func (m *DriveMembersService) SetRole(ctx context.Context, driveID int64, userID int64, role string) OperationResult {
	svc, err := m.roles(ctx)
	if err != nil {
		return operationFailure(err)
	}
	return operationFailure(svc.SetRole(ctx, driveID, userID, drive_roles.Role(role)))
}

// RemoveMember removes a member from the drive. With ban the member cannot rejoin
// until unbanned; without it the member may rejoin through an invite link.
func (m *DriveMembersService) RemoveMember(ctx context.Context, driveID int64, userID int64, ban bool) OperationResult {
	svc, err := m.roles(ctx)
	if err != nil {
		return operationFailure(err)
	}
	return operationFailure(svc.RemoveMember(ctx, driveID, userID, ban))
}

// CreateInvite exports an invite link for the drive. require_approval makes
// Telegram hold each joiner until a manager accepts them.
func (m *DriveMembersService) CreateInvite(ctx context.Context, driveID int64, require_approval bool) DriveInviteResult {
	svc, err := m.roles(ctx)
	if err != nil {
		return DriveInviteResult{Result: operationFailure(err)}
	}
	invite, err := svc.CreateInvite(ctx, driveID, "", require_approval)
	if err != nil {
		return DriveInviteResult{Result: operationFailure(err)}
	}
	return DriveInviteResult{Result: operationSuccess(), Link: invite.Link, DefaultRole: string(invite.DefaultRole)}
}

// SetDefaultRole sets the role newcomers get: "editor" (may send) or "viewer"
// (read only until promoted). Telegram applies it drive-wide.
func (m *DriveMembersService) SetDefaultRole(ctx context.Context, driveID int64, role string) OperationResult {
	svc, err := m.roles(ctx)
	if err != nil {
		return operationFailure(err)
	}
	return operationFailure(svc.SetDefaultRole(ctx, driveID, drive_roles.Role(role)))
}

// GetDefaultRole reports the role newcomers get in the drive.
func (m *DriveMembersService) GetDefaultRole(ctx context.Context, driveID int64) DriveDefaultRoleResult {
	svc, err := m.roles(ctx)
	if err != nil {
		return DriveDefaultRoleResult{Result: operationFailure(err)}
	}
	role, err := svc.GetDefaultRole(ctx, driveID)
	if err != nil {
		return DriveDefaultRoleResult{Result: operationFailure(err)}
	}
	return DriveDefaultRoleResult{Result: operationSuccess(), Role: string(role)}
}
