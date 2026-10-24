package drive_roles

import (
	"context"

	"TDrive/backend/tgclient"
)

// ParticipantFilter selects which Telegram participant list a page comes from.
// A full member list is the union of the filters: recent members, admins and
// banned or restricted members.
type ParticipantFilter int

const (
	FilterRecent ParticipantFilter = iota
	FilterSearch
	FilterAdmins
	FilterBanned
)

// Telegram is the narrow Telegram surface role management needs. The gotd
// adapter in gotd.go implements it; tests use an in-memory fake. Nothing here
// exposes *tg types, so the role logic stays testable without a connection.
type Telegram interface {
	// SelfParticipant returns the calling account's participation in the
	// drive. The bool is false when the account is not a participant at all.
	SelfParticipant(ctx context.Context, peer tgclient.InputPeer) (Participant, bool, error)
	// ListParticipants returns one page of participants from the filter.
	// query is used by FilterSearch and FilterBanned.
	ListParticipants(ctx context.Context, peer tgclient.InputPeer, filter ParticipantFilter, query string, offset, limit int) ([]Participant, error)
	// EditAdmin sets the admin rights of target. Empty rights remove admin.
	EditAdmin(ctx context.Context, peer tgclient.InputPeer, target Participant, rights AdminRights) error
	// EditBanned sets the restrictions of target. Empty rights lift all
	// restrictions and, with ViewMessages, kick or re-allow.
	EditBanned(ctx context.Context, peer tgclient.InputPeer, target Participant, rights BannedRights) error
	// DefaultBannedRights returns the restrictions applied to every member
	// that has no individual rights.
	DefaultBannedRights(ctx context.Context, peer tgclient.InputPeer) (BannedRights, error)
	// SetDefaultBannedRights replaces the default restrictions.
	SetDefaultBannedRights(ctx context.Context, peer tgclient.InputPeer, rights BannedRights) error
	// ExportInviteLink creates an invite link, optionally one that needs
	// approval. Telegram cannot attach a role to a link.
	ExportInviteLink(ctx context.Context, peer tgclient.InputPeer, require_approval bool) (string, error)
}
