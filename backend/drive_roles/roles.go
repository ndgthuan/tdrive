// Package drive_roles maps TDrive's four shared-drive roles onto Telegram's
// own channel permissions, so the server enforces them. A shared drive is a
// Telegram megagroup; its owner is the creator, managers are admins with the
// moderation rights below, editors are plain members and viewers are members
// whose send rights are banned while viewing stays allowed.
package drive_roles

// Role is a member's standing in one shared drive.
type Role string

const (
	RoleOwner   Role = "owner"
	RoleManager Role = "manager"
	RoleEditor  Role = "editor"
	RoleViewer  Role = "viewer"
)

// rank orders roles from most to least privileged. It drives member sorting.
func (r Role) rank() int {
	switch r {
	case RoleOwner:
		return 0
	case RoleManager:
		return 1
	case RoleEditor:
		return 2
	case RoleViewer:
		return 3
	default:
		return 4
	}
}

// Valid reports whether r is one of the four roles.
func (r Role) Valid() bool {
	return r.rank() < 4
}

// Capabilities is what a role may do in its drive. The same struct is handed
// to the UI and used by the backend write gates.
type Capabilities struct {
	Role             Role `json:"role"`
	CanUpload        bool `json:"can_upload"`
	CanDeleteOthers  bool `json:"can_delete_others"`
	CanManageMembers bool `json:"can_manage_members"`
	CanInvite        bool `json:"can_invite"`
}

// CapabilitiesFor returns the fixed capability set of a role.
func CapabilitiesFor(role Role) Capabilities {
	switch role {
	case RoleOwner:
		return Capabilities{Role: role, CanUpload: true, CanDeleteOthers: true, CanManageMembers: true, CanInvite: true}
	case RoleManager:
		return Capabilities{Role: role, CanUpload: true, CanDeleteOthers: true, CanManageMembers: true, CanInvite: true}
	case RoleEditor:
		return Capabilities{Role: role, CanUpload: true}
	default:
		return Capabilities{Role: RoleViewer}
	}
}

// AdminRights are the Telegram admin rights TDrive reads and writes. Only the
// fields listed here are ever set; the rest stay false.
type AdminRights struct {
	ChangeInfo     bool
	DeleteMessages bool
	BanUsers       bool
	InviteUsers    bool
	PinMessages    bool
	AddAdmins      bool
}

// BannedRights are per-member restrictions. A true field means the member is
// banned from that action. ViewMessages true means the member is kicked, so it
// is never part of a role and is only used by RemoveMember.
type BannedRights struct {
	ViewMessages bool
	SendMessages bool
	SendMedia    bool
	SendStickers bool
	SendGifs     bool
	SendGames    bool
	SendInline   bool
	EmbedLinks   bool
	SendPolls    bool
	ChangeInfo   bool
	InviteUsers  bool
	PinMessages  bool
}

// ManagerRights are the admin rights a manager holds: delete_messages,
// pin_messages, invite_users and ban_users, but never add_admins or change_info.
func ManagerRights() AdminRights {
	return AdminRights{DeleteMessages: true, PinMessages: true, InviteUsers: true, BanUsers: true}
}

// ViewerRights restrict a member to reading: every send-type action is banned
// and viewing stays allowed.
func ViewerRights() BannedRights {
	return BannedRights{
		SendMessages: true,
		SendMedia:    true,
		SendStickers: true,
		SendGifs:     true,
		SendGames:    true,
		SendInline:   true,
		EmbedLinks:   true,
		SendPolls:    true,
		ChangeInfo:   true,
		InviteUsers:  true,
		PinMessages:  true,
	}
}

// AdminRightsFor returns the admin rights that go with a role. Only managers
// are admins; the owner is the creator and is never written.
func AdminRightsFor(role Role) AdminRights {
	if role == RoleManager {
		return ManagerRights()
	}
	return AdminRights{}
}

// BannedRightsFor returns the per-member restrictions that go with a role.
// Owners, managers and editors have none; viewers get ViewerRights.
func BannedRightsFor(role Role) BannedRights {
	if role == RoleViewer {
		return ViewerRights()
	}
	return BannedRights{}
}

// IsModerator reports whether an admin right set makes its holder a manager.
// Any moderation right is enough; a plain admin with none of them is an editor.
func IsModerator(rights AdminRights) bool {
	return rights.DeleteMessages || rights.BanUsers || rights.InviteUsers || rights.PinMessages
}

// IsViewerRestriction reports whether banned rights make a member a viewer:
// sending messages and media are both banned.
func IsViewerRestriction(rights BannedRights) bool {
	return rights.SendMessages && rights.SendMedia
}

// ParticipantKind is the Telegram participant type, reduced to what role
// mapping needs.
type ParticipantKind string

const (
	KindCreator ParticipantKind = "creator"
	KindAdmin   ParticipantKind = "admin"
	KindMember  ParticipantKind = "member"
	KindBanned  ParticipantKind = "banned"
	KindLeft    ParticipantKind = "left"
)

// Participant is one channel participant as the Telegram adapter reports it.
// AccessHash is needed to address the user in later edits.
type Participant struct {
	UserID       int64
	AccessHash   int64
	DisplayName  string
	Username     string
	Kind         ParticipantKind
	AdminRights  AdminRights
	BannedRights BannedRights
	JoinedAt     int64
}

// IsMember reports whether the participant still belongs to the drive. Left
// and kicked (view-banned) participants do not.
func (p Participant) IsMember() bool {
	switch p.Kind {
	case KindCreator, KindAdmin, KindMember:
		return true
	case KindBanned:
		return !p.BannedRights.ViewMessages
	default:
		return false
	}
}

// RoleOf maps a participant to its drive role. The second result is false
// when the participant is not a member.
func RoleOf(p Participant) (Role, bool) {
	if !p.IsMember() {
		return "", false
	}
	switch p.Kind {
	case KindCreator:
		return RoleOwner, true
	case KindAdmin:
		if IsModerator(p.AdminRights) {
			return RoleManager, true
		}
		return RoleEditor, true
	case KindBanned:
		if IsViewerRestriction(p.BannedRights) {
			return RoleViewer, true
		}
		return RoleEditor, true
	case KindMember:
		// A plain member has no record of its own: Telegram applies the drive's
		// default restrictions, which the service copies into BannedRights.
		if IsViewerRestriction(p.BannedRights) {
			return RoleViewer, true
		}
		return RoleEditor, true
	default:
		return RoleEditor, true
	}
}

// with_default_rights gives a plain member the drive's default restrictions.
// Telegram reports no banned rights for a member with no individual record, so
// without this a viewer-by-default drive would show its new members as editors.
func with_default_rights(p Participant, defaults BannedRights) Participant {
	if p.Kind == KindMember {
		p.BannedRights = defaults
	}
	return p
}
