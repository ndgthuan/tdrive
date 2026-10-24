package drive_roles

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"TDrive/backend/tgclient"
)

// fake_tg is an in-memory Telegram channel. It keeps participants with the
// same kinds Telegram reports and refuses the writes Telegram refuses, so the
// service's ordering and permission rules are exercised without a connection.
type fake_tg struct {
	self     int64
	people   map[int64]Participant
	defaults BannedRights
	calls    []string

	self_calls    int
	list_calls    int
	fail_self     error
	invite_link   string
	invite_flag   bool
	invite_calls  int
	default_calls int
}

func new_fake_tg(self int64, people ...Participant) *fake_tg {
	f := &fake_tg{self: self, people: make(map[int64]Participant)}
	for _, p := range people {
		f.people[p.UserID] = p
	}
	return f
}

func creator_of(id int64, name string) Participant {
	return Participant{UserID: id, AccessHash: id * 10, DisplayName: name, Kind: KindCreator}
}

func editor_of(id int64, name string) Participant {
	return Participant{UserID: id, AccessHash: id * 10, DisplayName: name, Kind: KindMember}
}

func manager_of(id int64, name string) Participant {
	return Participant{UserID: id, AccessHash: id * 10, DisplayName: name, Kind: KindAdmin, AdminRights: ManagerRights()}
}

func viewer_of(id int64, name string) Participant {
	return Participant{UserID: id, AccessHash: id * 10, DisplayName: name, Kind: KindBanned, BannedRights: ViewerRights()}
}

func (f *fake_tg) SelfParticipant(ctx context.Context, peer tgclient.InputPeer) (Participant, bool, error) {
	f.self_calls++
	if f.fail_self != nil {
		return Participant{}, false, f.fail_self
	}
	p, ok := f.people[f.self]
	return p, ok, nil
}

func (f *fake_tg) ListParticipants(ctx context.Context, peer tgclient.InputPeer, filter ParticipantFilter, query string, offset, limit int) ([]Participant, error) {
	f.list_calls++
	ids := make([]int64, 0, len(f.people))
	for id := range f.people {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var matched []Participant
	for _, id := range ids {
		p := f.people[id]
		if !name_matches(p, query, filter) {
			continue
		}
		if !in_filter(p, filter) {
			continue
		}
		matched = append(matched, p)
	}
	if offset >= len(matched) {
		return nil, nil
	}
	end := min(offset+limit, len(matched))
	return matched[offset:end], nil
}

// name_matches applies the search text. Only search and banned lists honour it.
func name_matches(p Participant, query string, filter ParticipantFilter) bool {
	if query == "" || (filter != FilterSearch && filter != FilterBanned) {
		return true
	}
	return strings.Contains(strings.ToLower(p.DisplayName), strings.ToLower(query))
}

// in_filter mirrors Telegram: recent and admin lists never include restricted
// or kicked members; the banned list holds them and nothing else.
func in_filter(p Participant, filter ParticipantFilter) bool {
	switch filter {
	case FilterAdmins:
		return p.Kind == KindCreator || p.Kind == KindAdmin
	case FilterBanned:
		return p.Kind == KindBanned || p.Kind == KindLeft
	default:
		return p.Kind == KindCreator || p.Kind == KindAdmin || p.Kind == KindMember
	}
}

func (f *fake_tg) EditAdmin(ctx context.Context, peer tgclient.InputPeer, target Participant, rights AdminRights) error {
	f.calls = append(f.calls, fmt.Sprintf("editAdmin %d %s", target.UserID, describe_admin(rights)))
	p, ok := f.people[target.UserID]
	if !ok {
		return errors.New("USER_ID_INVALID")
	}
	if p.Kind == KindCreator {
		return errors.New("CHAT_ADMIN_REQUIRED")
	}
	if p.Kind == KindBanned && rights != (AdminRights{}) {
		return errors.New("USER_RESTRICTED")
	}
	if rights == (AdminRights{}) {
		if p.Kind == KindAdmin {
			p.Kind = KindMember
		}
	} else if p.Kind == KindMember || p.Kind == KindAdmin {
		p.Kind = KindAdmin
	}
	p.AdminRights = rights
	f.people[target.UserID] = p
	return nil
}

func (f *fake_tg) EditBanned(ctx context.Context, peer tgclient.InputPeer, target Participant, rights BannedRights) error {
	f.calls = append(f.calls, fmt.Sprintf("editBanned %d %s", target.UserID, describe_banned(rights)))
	p, ok := f.people[target.UserID]
	if !ok {
		return errors.New("USER_ID_INVALID")
	}
	if p.Kind == KindAdmin || p.Kind == KindCreator {
		return errors.New("CHAT_ADMIN_REQUIRED")
	}
	p.AdminRights = AdminRights{}
	p.BannedRights = rights
	switch {
	case rights == (BannedRights{}):
		p.Kind = KindMember
	default:
		p.Kind = KindBanned
	}
	f.people[target.UserID] = p
	return nil
}

func (f *fake_tg) DefaultBannedRights(ctx context.Context, peer tgclient.InputPeer) (BannedRights, error) {
	return f.defaults, nil
}

func (f *fake_tg) SetDefaultBannedRights(ctx context.Context, peer tgclient.InputPeer, rights BannedRights) error {
	f.default_calls++
	f.defaults = rights
	return nil
}

func (f *fake_tg) ExportInviteLink(ctx context.Context, peer tgclient.InputPeer, require_approval bool) (string, error) {
	f.invite_calls++
	f.invite_flag = require_approval
	if f.invite_link == "" {
		f.invite_link = "https://t.me/+fake"
	}
	return f.invite_link, nil
}

// describe_admin and describe_banned name the rights a write carried, so tests
// can assert the order of writes without comparing whole structs.
func describe_admin(r AdminRights) string {
	switch {
	case r == (AdminRights{}):
		return "clear"
	case r == ManagerRights():
		return "manager"
	default:
		return "other"
	}
}

func describe_banned(r BannedRights) string {
	switch {
	case r == (BannedRights{}):
		return "lift"
	case r == ViewerRights():
		return "viewer"
	case r == (BannedRights{ViewMessages: true}):
		return "kick"
	default:
		return "other"
	}
}
