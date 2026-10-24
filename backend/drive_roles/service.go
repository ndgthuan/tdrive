package drive_roles

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"TDrive/backend/tgclient"

	"golang.org/x/sync/singleflight"
)

// DriveKind says how a channel is used. Only shared drives have members and
// roles; the personal drive is always owned by the account, and channel
// sources are watched, never written.
type DriveKind int

const (
	DriveKindShared DriveKind = iota
	DriveKindPersonal
	DriveKindReadOnly
)

const (
	default_role_ttl   = 60 * time.Second
	default_member_ttl = 30 * time.Second
	default_page_limit = 50
	max_page_limit     = 200
	// telegram_page is the largest page channels.getParticipants accepts.
	telegram_page = 200
	// max_scan bounds how many participants one snapshot reads per filter.
	max_scan = 10000
)

// KindFunc reports the kind of a channel.
type KindFunc func(channel_id int64) (DriveKind, error)

// PeerFunc resolves the Telegram peer of a shared drive.
type PeerFunc func(ctx context.Context, channel_id int64) (tgclient.InputPeer, error)

// SelfIDFunc returns the calling account's Telegram user id.
type SelfIDFunc func(ctx context.Context) (int64, error)

// Service owns role lookup, member listing and role changes for shared drives.
// Telegram is the source of truth: the service only caches answers for a short
// time and never invents a role locally. It holds no SQLite state.
type Service struct {
	Kind   KindFunc
	Peers  PeerFunc
	SelfID SelfIDFunc
	TG     Telegram
	// OnRoleChanged is called when a refresh observes a different role for a
	// drive than the one cached before. It is called without locks held.
	OnRoleChanged func(channel_id int64, role Role)
	Now           func() time.Time
	RoleTTL       time.Duration
	MemberTTL     time.Duration

	mu      sync.Mutex
	roles   map[int64]role_entry
	members map[int64]member_entry
	flight  singleflight.Group
}

type role_entry struct {
	caps Capabilities
	at   time.Time
}

type member_entry struct {
	list []Participant
	at   time.Time
}

// NewService builds a Service with default cache lifetimes.
func NewService(kind KindFunc, peers PeerFunc, self_id SelfIDFunc, tg Telegram) *Service {
	return &Service{
		Kind:      kind,
		Peers:     peers,
		SelfID:    self_id,
		TG:        tg,
		Now:       time.Now,
		RoleTTL:   default_role_ttl,
		MemberTTL: default_member_ttl,
		roles:     make(map[int64]role_entry),
		members:   make(map[int64]member_entry),
	}
}

// MyRole returns the caller's capabilities in a drive. Results are cached for
// RoleTTL. The personal drive is always owned by the caller; a channel source
// is always read-only.
func (s *Service) MyRole(ctx context.Context, channel_id int64) (Capabilities, error) {
	return s.capabilities(ctx, channel_id, false)
}

// RefreshMyRole asks Telegram for the caller's role now, ignoring the cache,
// and reports a changed role through OnRoleChanged.
func (s *Service) RefreshMyRole(ctx context.Context, channel_id int64) (Capabilities, error) {
	return s.capabilities(ctx, channel_id, true)
}

func (s *Service) capabilities(ctx context.Context, channel_id int64, force bool) (Capabilities, error) {
	kind, err := s.kind_of(channel_id)
	if err != nil {
		return Capabilities{}, err
	}
	switch kind {
	case DriveKindPersonal:
		return CapabilitiesFor(RoleOwner), nil
	case DriveKindReadOnly:
		return CapabilitiesFor(RoleViewer), nil
	}
	if !force {
		s.mu.Lock()
		entry, ok := s.roles[channel_id]
		s.mu.Unlock()
		if ok && s.now().Sub(entry.at) < s.RoleTTL {
			return entry.caps, nil
		}
	}
	key := strconv.FormatInt(channel_id, 10)
	value, err, _ := s.flight.Do(key, func() (any, error) {
		return s.resolve(ctx, channel_id)
	})
	if err != nil {
		return Capabilities{}, err
	}
	return value.(Capabilities), nil
}

// resolve asks Telegram for the role and records it. When Telegram cannot be
// reached but an earlier answer exists, that answer is returned so an offline
// session keeps its last known permissions instead of losing them all.
func (s *Service) resolve(ctx context.Context, channel_id int64) (Capabilities, error) {
	caps, err := s.fetch_role(ctx, channel_id)
	if err != nil {
		if errors.Is(err, ErrNotMember) {
			s.mu.Lock()
			delete(s.roles, channel_id)
			s.mu.Unlock()
			return Capabilities{}, err
		}
		s.mu.Lock()
		prev, ok := s.roles[channel_id]
		s.mu.Unlock()
		if ok {
			return prev.caps, nil
		}
		return Capabilities{}, err
	}
	s.mu.Lock()
	prev, had := s.roles[channel_id]
	s.roles[channel_id] = role_entry{caps: caps, at: s.now()}
	s.mu.Unlock()
	if had && prev.caps.Role != caps.Role && s.OnRoleChanged != nil {
		s.OnRoleChanged(channel_id, caps.Role)
	}
	return caps, nil
}

func (s *Service) fetch_role(ctx context.Context, channel_id int64) (Capabilities, error) {
	peer, err := s.peer_of(ctx, channel_id)
	if err != nil {
		return Capabilities{}, wrap_error(ErrRoleUnavailable, err)
	}
	self, ok, err := s.TG.SelfParticipant(ctx, peer)
	if err != nil {
		return Capabilities{}, wrap_error(ErrRoleUnavailable, err)
	}
	if !ok {
		return Capabilities{}, ErrNotMember
	}
	if self.Kind == KindMember {
		defaults, err := s.TG.DefaultBannedRights(ctx, peer)
		if err != nil {
			return Capabilities{}, wrap_error(ErrRoleUnavailable, err)
		}
		self = with_default_rights(self, defaults)
	}
	role, member := RoleOf(self)
	if !member {
		return Capabilities{}, ErrNotMember
	}
	return CapabilitiesFor(role), nil
}

// ListMembers returns one page of the drive's members, sorted by role, then
// name. An empty query lists everyone; a non-empty query searches names and
// usernames. limit defaults to 50 and is capped at 200.
func (s *Service) ListMembers(ctx context.Context, channel_id int64, query string, offset, limit int) (MemberPage, error) {
	if err := s.require_shared(channel_id); err != nil {
		return MemberPage{}, err
	}
	if _, err := s.MyRole(ctx, channel_id); err != nil {
		return MemberPage{}, err
	}
	self, err := s.self_user(ctx)
	if err != nil {
		return MemberPage{}, err
	}
	list, err := s.snapshot(ctx, channel_id, strings.TrimSpace(query), false)
	if err != nil {
		return MemberPage{}, err
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = default_page_limit
	}
	if limit > max_page_limit {
		limit = max_page_limit
	}
	total := len(list)
	start := min(offset, total)
	end := min(start+limit, total)
	page := MemberPage{Total: total, NextOffset: end, HasMore: end < total, Members: make([]Member, 0, end-start)}
	for _, p := range list[start:end] {
		role, _ := RoleOf(p)
		page.Members = append(page.Members, Member{
			UserID:      p.UserID,
			DisplayName: p.DisplayName,
			Username:    p.Username,
			Role:        role,
			JoinedAt:    p.JoinedAt,
			IsSelf:      p.UserID == self,
		})
	}
	return page, nil
}

// SetRole changes one member's role. The rules are enforced before any RPC:
// the owner is never changed and never assigned; only the owner promotes to
// manager; a manager may set editors and viewers but not other managers; and a
// manager may step down only while another manager remains.
//
// Telegram enforces the same rights server-side, so these checks only make the
// refusal clearer and avoid half-applied changes.
func (s *Service) SetRole(ctx context.Context, channel_id, user_id int64, role Role) error {
	if err := s.require_shared(channel_id); err != nil {
		return err
	}
	if user_id <= 0 {
		return ErrUserNotFound
	}
	if role == RoleOwner {
		return ErrOwnerImmutable
	}
	if !role.Valid() {
		return ErrInvalidRole
	}
	actor, err := s.MyRole(ctx, channel_id)
	if err != nil {
		return err
	}
	if !actor.CanManageMembers {
		return ErrPermissionDenied
	}
	self, err := s.self_user(ctx)
	if err != nil {
		return err
	}
	list, err := s.snapshot(ctx, channel_id, "", false)
	if err != nil {
		return err
	}
	target, managers, err := find_in(list, user_id)
	if err != nil {
		return err
	}
	current, _ := RoleOf(target)
	if current == role {
		return nil
	}
	if err := check_role_change(actor.Role, current, role, target.UserID == self, managers); err != nil {
		return err
	}
	peer, err := s.peer_of(ctx, channel_id)
	if err != nil {
		return wrap_error(ErrRoleUnavailable, err)
	}
	if err := s.apply_role(ctx, peer, target, current, role); err != nil {
		return err
	}
	s.forget_members(channel_id)
	return nil
}

// RemoveMember kicks a member out of the drive. With ban the member stays
// banned from viewing; without it the ban is lifted at once, so an invite
// link works again. The owner cannot be removed, managers cannot remove other
// managers, and nobody removes themselves through this call (that is leaving).
func (s *Service) RemoveMember(ctx context.Context, channel_id, user_id int64, ban bool) error {
	if err := s.require_shared(channel_id); err != nil {
		return err
	}
	if user_id <= 0 {
		return ErrUserNotFound
	}
	actor, err := s.MyRole(ctx, channel_id)
	if err != nil {
		return err
	}
	if !actor.CanManageMembers {
		return ErrPermissionDenied
	}
	self, err := s.self_user(ctx)
	if err != nil {
		return err
	}
	if user_id == self {
		return ErrPermissionDenied
	}
	list, err := s.snapshot(ctx, channel_id, "", false)
	if err != nil {
		return err
	}
	target, _, err := find_in(list, user_id)
	if err != nil {
		return err
	}
	current, _ := RoleOf(target)
	if current == RoleOwner {
		return ErrOwnerImmutable
	}
	if actor.Role != RoleOwner && current == RoleManager {
		return ErrPermissionDenied
	}
	peer, err := s.peer_of(ctx, channel_id)
	if err != nil {
		return wrap_error(ErrRoleUnavailable, err)
	}
	if current == RoleManager {
		if err := s.TG.EditAdmin(ctx, peer, target, AdminRights{}); err != nil {
			return err
		}
	}
	if err := s.TG.EditBanned(ctx, peer, target, BannedRights{ViewMessages: true}); err != nil {
		return err
	}
	if !ban {
		if err := s.TG.EditBanned(ctx, peer, target, BannedRights{}); err != nil {
			return err
		}
	}
	s.forget_members(channel_id)
	return nil
}

// InviteResult is a new invite link and the drive's default role for people
// who join through it.
type InviteResult struct {
	Link        string
	DefaultRole Role
}

// CreateInvite exports an invite link. Telegram cannot attach a role to a
// link, so a non-empty default_role is applied drive-wide through
// SetDefaultRole before the link is made; newcomers then start in that role.
func (s *Service) CreateInvite(ctx context.Context, channel_id int64, default_role Role, require_approval bool) (InviteResult, error) {
	if err := s.require_shared(channel_id); err != nil {
		return InviteResult{}, err
	}
	actor, err := s.MyRole(ctx, channel_id)
	if err != nil {
		return InviteResult{}, err
	}
	if !actor.CanInvite {
		return InviteResult{}, ErrPermissionDenied
	}
	if default_role != "" {
		if err := s.SetDefaultRole(ctx, channel_id, default_role); err != nil {
			return InviteResult{}, err
		}
	}
	peer, err := s.peer_of(ctx, channel_id)
	if err != nil {
		return InviteResult{}, wrap_error(ErrRoleUnavailable, err)
	}
	link, err := s.TG.ExportInviteLink(ctx, peer, require_approval)
	if err != nil {
		return InviteResult{}, err
	}
	def, err := s.GetDefaultRole(ctx, channel_id)
	if err != nil {
		return InviteResult{}, err
	}
	return InviteResult{Link: link, DefaultRole: def}, nil
}

// SetDefaultRole sets the role newcomers get: editor (no restrictions) or
// viewer (send rights banned). Telegram applies default restrictions to every
// member without individual rights, so setting viewer also restricts members
// who have no explicit editor rights. Only the owner and managers may change it.
func (s *Service) SetDefaultRole(ctx context.Context, channel_id int64, role Role) error {
	if err := s.require_shared(channel_id); err != nil {
		return err
	}
	if role != RoleEditor && role != RoleViewer {
		return ErrInvalidRole
	}
	actor, err := s.MyRole(ctx, channel_id)
	if err != nil {
		return err
	}
	if !actor.CanManageMembers {
		return ErrPermissionDenied
	}
	peer, err := s.peer_of(ctx, channel_id)
	if err != nil {
		return wrap_error(ErrRoleUnavailable, err)
	}
	return s.TG.SetDefaultBannedRights(ctx, peer, BannedRightsFor(role))
}

// GetDefaultRole reports the role newcomers get: viewer when the default
// restrictions ban sending, otherwise editor.
func (s *Service) GetDefaultRole(ctx context.Context, channel_id int64) (Role, error) {
	if err := s.require_shared(channel_id); err != nil {
		return "", err
	}
	if _, err := s.MyRole(ctx, channel_id); err != nil {
		return "", err
	}
	peer, err := s.peer_of(ctx, channel_id)
	if err != nil {
		return "", wrap_error(ErrRoleUnavailable, err)
	}
	rights, err := s.TG.DefaultBannedRights(ctx, peer)
	if err != nil {
		return "", err
	}
	if IsViewerRestriction(rights) {
		return RoleViewer, nil
	}
	return RoleEditor, nil
}

// Member is one row of ListMembers.
type Member struct {
	UserID      int64
	DisplayName string
	Username    string
	Role        Role
	JoinedAt    int64
	IsSelf      bool
}

// MemberPage is one page of members. NextOffset is where the following page
// starts; HasMore is false on the last page.
type MemberPage struct {
	Members    []Member
	Total      int
	NextOffset int
	HasMore    bool
}

// check_role_change is the permission matrix for SetRole. It returns nil when
// the actor may move the target from current to requested.
func check_role_change(actor, current, requested Role, is_self bool, managers int) error {
	if current == RoleOwner {
		return ErrOwnerImmutable
	}
	if requested == RoleManager && actor != RoleOwner {
		return ErrPermissionDenied
	}
	switch actor {
	case RoleOwner:
		return nil
	case RoleManager:
		if current == RoleManager && !is_self {
			return ErrPermissionDenied
		}
		if is_self && current == RoleManager && requested != RoleManager && managers <= 1 {
			return ErrLastManager
		}
		return nil
	default:
		return ErrPermissionDenied
	}
}

type role_step int

const (
	step_clear_admin role_step = iota
	step_lift_restrictions
	step_grant_manager
	step_restrict_viewer
)

// plan_role_change lists the Telegram writes that move a member between roles,
// in order. Admin rights are removed before restrictions are set, and
// restrictions are lifted before admin rights are granted, because Telegram
// refuses to restrict an admin and to make a restricted member an admin.
func plan_role_change(current, requested Role) []role_step {
	var steps []role_step
	if current == RoleManager && requested != RoleManager {
		steps = append(steps, step_clear_admin)
	}
	if current == RoleViewer && requested != RoleViewer {
		steps = append(steps, step_lift_restrictions)
	}
	if requested == RoleManager && current != RoleManager {
		steps = append(steps, step_grant_manager)
	}
	if requested == RoleViewer && current != RoleViewer {
		steps = append(steps, step_restrict_viewer)
	}
	return steps
}

func (s *Service) apply_role(ctx context.Context, peer tgclient.InputPeer, target Participant, current, requested Role) error {
	for _, step := range plan_role_change(current, requested) {
		var err error
		switch step {
		case step_clear_admin:
			err = s.TG.EditAdmin(ctx, peer, target, AdminRights{})
		case step_lift_restrictions:
			err = s.TG.EditBanned(ctx, peer, target, BannedRights{})
		case step_grant_manager:
			err = s.TG.EditAdmin(ctx, peer, target, ManagerRights())
		case step_restrict_viewer:
			err = s.TG.EditBanned(ctx, peer, target, ViewerRights())
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// snapshot returns the members of a drive sorted by role, then name. An empty
// query is cached for MemberTTL; a search is never cached.
func (s *Service) snapshot(ctx context.Context, channel_id int64, query string, force bool) ([]Participant, error) {
	if query == "" && !force {
		s.mu.Lock()
		entry, ok := s.members[channel_id]
		s.mu.Unlock()
		if ok && s.now().Sub(entry.at) < s.MemberTTL {
			return entry.list, nil
		}
	}
	peer, err := s.peer_of(ctx, channel_id)
	if err != nil {
		return nil, wrap_error(ErrRoleUnavailable, err)
	}
	filters := []ParticipantFilter{FilterRecent, FilterAdmins, FilterBanned}
	if query != "" {
		filters = []ParticipantFilter{FilterSearch, FilterBanned}
	}
	var all []Participant
	for _, filter := range filters {
		page, err := s.collect(ctx, peer, filter, query)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
	}
	defaults, err := s.TG.DefaultBannedRights(ctx, peer)
	if err != nil {
		return nil, wrap_error(ErrRoleUnavailable, err)
	}
	for i := range all {
		all[i] = with_default_rights(all[i], defaults)
	}
	list := merge_members(all)
	if query == "" {
		s.mu.Lock()
		s.members[channel_id] = member_entry{list: list, at: s.now()}
		s.mu.Unlock()
	}
	return list, nil
}

func (s *Service) collect(ctx context.Context, peer tgclient.InputPeer, filter ParticipantFilter, query string) ([]Participant, error) {
	var out []Participant
	for offset := 0; offset < max_scan; offset += telegram_page {
		page, err := s.TG.ListParticipants(ctx, peer, filter, query, offset, telegram_page)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < telegram_page || filter == FilterAdmins {
			break
		}
	}
	return out, nil
}

// participant_weight ranks the entries Telegram reports for one user. The
// most authoritative entry wins: a creator or admin over a restricted member
// over a plain member. A kicked or left entry is never a member.
func participant_weight(p Participant) int {
	switch p.Kind {
	case KindCreator:
		return 4
	case KindAdmin:
		return 3
	case KindBanned:
		if p.IsMember() {
			return 2
		}
		return -1
	case KindMember:
		return 1
	default:
		return -1
	}
}

// merge_members keeps one entry per user, drops non-members and sorts members
// by role rank, then name, then user id.
func merge_members(all []Participant) []Participant {
	best := make(map[int64]Participant, len(all))
	for _, p := range all {
		if p.UserID == 0 {
			continue
		}
		prev, ok := best[p.UserID]
		if !ok || participant_weight(p) > participant_weight(prev) {
			best[p.UserID] = p
		}
	}
	out := make([]Participant, 0, len(best))
	for _, p := range best {
		if p.IsMember() {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b Participant) int {
		ra, _ := RoleOf(a)
		rb, _ := RoleOf(b)
		if c := cmp.Compare(ra.rank(), rb.rank()); c != 0 {
			return c
		}
		if c := cmp.Compare(strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName)); c != 0 {
			return c
		}
		return cmp.Compare(a.UserID, b.UserID)
	})
	return out
}

// find_in looks up one member in a snapshot and counts the managers in it.
func find_in(list []Participant, user_id int64) (Participant, int, error) {
	var target Participant
	found := false
	managers := 0
	for _, p := range list {
		if role, _ := RoleOf(p); role == RoleManager {
			managers++
		}
		if p.UserID == user_id {
			target = p
			found = true
		}
	}
	if !found {
		return Participant{}, managers, ErrNotMember
	}
	return target, managers, nil
}

func (s *Service) require_shared(channel_id int64) error {
	kind, err := s.kind_of(channel_id)
	if err != nil {
		return err
	}
	if kind != DriveKindShared {
		return ErrNotSharedDrive
	}
	return nil
}

func (s *Service) kind_of(channel_id int64) (DriveKind, error) {
	if s.Kind == nil {
		return DriveKindShared, nil
	}
	return s.Kind(channel_id)
}

func (s *Service) peer_of(ctx context.Context, channel_id int64) (tgclient.InputPeer, error) {
	if s.Peers == nil {
		return tgclient.InputPeer{}, errors.New("drive_roles: peer resolver not wired")
	}
	return s.Peers(ctx, channel_id)
}

func (s *Service) self_user(ctx context.Context) (int64, error) {
	if s.SelfID == nil {
		return 0, wrap_error(ErrRoleUnavailable, errors.New("drive_roles: self id not wired"))
	}
	id, err := s.SelfID(ctx)
	if err != nil {
		return 0, wrap_error(ErrRoleUnavailable, err)
	}
	return id, nil
}

func (s *Service) forget_members(channel_id int64) {
	s.mu.Lock()
	delete(s.members, channel_id)
	s.mu.Unlock()
}

func (s *Service) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}
