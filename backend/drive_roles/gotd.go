package drive_roles

import (
	"context"
	"errors"
	"strings"

	"TDrive/backend/auth"
	"TDrive/backend/tgclient"

	"github.com/gotd/td/tg"
)

// gotd_telegram implements Telegram on the shared gotd connection. It is the
// only file in this package that touches *tg types.
type gotd_telegram struct {
	runner tgclient.APIRunner
}

// NewGotdTelegram returns the production Telegram adapter.
func NewGotdTelegram(runner tgclient.APIRunner) Telegram {
	return gotd_telegram{runner: runner}
}

func channel_of(peer tgclient.InputPeer) *tg.InputChannel {
	return &tg.InputChannel{ChannelID: peer.ChannelID, AccessHash: peer.AccessHash}
}

func (t gotd_telegram) SelfParticipant(ctx context.Context, peer tgclient.InputPeer) (Participant, bool, error) {
	var out Participant
	found := false
	err := t.runner.RunAPI(ctx, func(ctx context.Context, api *tg.Client) error {
		res, err := api.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{
			Channel:     channel_of(peer),
			Participant: &tg.InputPeerSelf{},
		})
		if err != nil {
			if is_not_participant(err) {
				return nil
			}
			return err
		}
		out, found = participant_from(res.Participant, users_by_id(res.Users))
		return nil
	})
	if err != nil {
		return Participant{}, false, map_telegram_error(err)
	}
	return out, found, nil
}

func (t gotd_telegram) ListParticipants(ctx context.Context, peer tgclient.InputPeer, filter ParticipantFilter, query string, offset, limit int) ([]Participant, error) {
	var out []Participant
	err := t.runner.RunAPI(ctx, func(ctx context.Context, api *tg.Client) error {
		res, err := api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
			Channel: channel_of(peer),
			Filter:  telegram_filter(filter, query),
			Offset:  offset,
			Limit:   limit,
		})
		if err != nil {
			return err
		}
		list, ok := res.(*tg.ChannelsChannelParticipants)
		if !ok {
			return errors.New("drive_roles: unexpected participants response")
		}
		users := users_by_id(list.Users)
		for _, raw := range list.Participants {
			if p, ok := participant_from(raw, users); ok {
				out = append(out, p)
			}
		}
		return nil
	})
	return out, map_telegram_error(err)
}

func telegram_filter(filter ParticipantFilter, query string) tg.ChannelParticipantsFilterClass {
	switch filter {
	case FilterSearch:
		return &tg.ChannelParticipantsSearch{Q: query}
	case FilterAdmins:
		return &tg.ChannelParticipantsAdmins{}
	case FilterBanned:
		return &tg.ChannelParticipantsBanned{Q: query}
	default:
		return &tg.ChannelParticipantsRecent{}
	}
}

func (t gotd_telegram) EditAdmin(ctx context.Context, peer tgclient.InputPeer, target Participant, rights AdminRights) error {
	return map_telegram_error(t.runner.RunAPI(ctx, func(ctx context.Context, api *tg.Client) error {
		_, err := api.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{
			Channel:     channel_of(peer),
			UserID:      &tg.InputUser{UserID: target.UserID, AccessHash: target.AccessHash},
			AdminRights: to_chat_admin_rights(rights),
		})
		return err
	}))
}

func (t gotd_telegram) EditBanned(ctx context.Context, peer tgclient.InputPeer, target Participant, rights BannedRights) error {
	return map_telegram_error(t.runner.RunAPI(ctx, func(ctx context.Context, api *tg.Client) error {
		_, err := api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
			Channel:      channel_of(peer),
			Participant:  &tg.InputPeerUser{UserID: target.UserID, AccessHash: target.AccessHash},
			BannedRights: to_chat_banned_rights(rights),
		})
		return err
	}))
}

func (t gotd_telegram) DefaultBannedRights(ctx context.Context, peer tgclient.InputPeer) (BannedRights, error) {
	var out BannedRights
	err := t.runner.RunAPI(ctx, func(ctx context.Context, api *tg.Client) error {
		full, err := api.ChannelsGetFullChannel(ctx, channel_of(peer))
		if err != nil {
			return err
		}
		for _, chat := range full.Chats {
			if ch, ok := chat.(*tg.Channel); ok && ch.ID == peer.ChannelID {
				out = from_chat_banned_rights(ch.DefaultBannedRights)
				return nil
			}
		}
		return errors.New("drive_roles: drive not found in full channel response")
	})
	return out, map_telegram_error(err)
}

func (t gotd_telegram) SetDefaultBannedRights(ctx context.Context, peer tgclient.InputPeer, rights BannedRights) error {
	return map_telegram_error(t.runner.RunAPI(ctx, func(ctx context.Context, api *tg.Client) error {
		_, err := api.MessagesEditChatDefaultBannedRights(ctx, &tg.MessagesEditChatDefaultBannedRightsRequest{
			Peer:         &tg.InputPeerChannel{ChannelID: peer.ChannelID, AccessHash: peer.AccessHash},
			BannedRights: to_chat_banned_rights(rights),
		})
		return err
	}))
}

func (t gotd_telegram) ExportInviteLink(ctx context.Context, peer tgclient.InputPeer, require_approval bool) (string, error) {
	var link string
	err := t.runner.RunAPI(ctx, func(ctx context.Context, api *tg.Client) error {
		value, err := auth.ExportInviteLink(ctx, api, &tg.InputPeerChannel{ChannelID: peer.ChannelID, AccessHash: peer.AccessHash}, require_approval)
		link = value
		return err
	})
	return link, map_telegram_error(err)
}

// participant_from converts one Telegram participant. The second result is
// false for entries that carry no user.
func participant_from(raw tg.ChannelParticipantClass, users map[int64]*tg.User) (Participant, bool) {
	var p Participant
	var user_id int64
	switch v := raw.(type) {
	case *tg.ChannelParticipant:
		user_id, p.Kind, p.JoinedAt = v.UserID, KindMember, int64(v.Date)
	case *tg.ChannelParticipantSelf:
		user_id, p.Kind, p.JoinedAt = v.UserID, KindMember, int64(v.Date)
	case *tg.ChannelParticipantCreator:
		user_id, p.Kind = v.UserID, KindCreator
		p.AdminRights = from_chat_admin_rights(v.AdminRights)
	case *tg.ChannelParticipantAdmin:
		user_id, p.Kind, p.JoinedAt = v.UserID, KindAdmin, int64(v.Date)
		p.AdminRights = from_chat_admin_rights(v.AdminRights)
	case *tg.ChannelParticipantBanned:
		uid, ok := peer_user_id(v.Peer)
		if !ok {
			return Participant{}, false
		}
		user_id, p.JoinedAt = uid, int64(v.Date)
		p.Kind = KindBanned
		if v.Left {
			p.Kind = KindLeft
		}
		p.BannedRights = from_chat_banned_rights(v.BannedRights)
	case *tg.ChannelParticipantLeft:
		uid, ok := peer_user_id(v.Peer)
		if !ok {
			return Participant{}, false
		}
		user_id, p.Kind = uid, KindLeft
	default:
		return Participant{}, false
	}
	if user_id == 0 {
		return Participant{}, false
	}
	p.UserID = user_id
	if user := users[user_id]; user != nil {
		p.AccessHash = user.AccessHash
		p.Username = user.Username
		p.DisplayName = display_name(user)
	}
	return p, true
}

func peer_user_id(peer tg.PeerClass) (int64, bool) {
	if p, ok := peer.(*tg.PeerUser); ok {
		return p.UserID, true
	}
	return 0, false
}

func users_by_id(list []tg.UserClass) map[int64]*tg.User {
	out := make(map[int64]*tg.User, len(list))
	for _, raw := range list {
		if user, ok := raw.(*tg.User); ok {
			out[user.ID] = user
		}
	}
	return out
}

func display_name(user *tg.User) string {
	name := strings.TrimSpace(strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName))
	if name != "" {
		return name
	}
	if strings.TrimSpace(user.Username) != "" {
		return "@" + strings.TrimSpace(user.Username)
	}
	return ""
}

func from_chat_admin_rights(r tg.ChatAdminRights) AdminRights {
	return AdminRights{
		ChangeInfo:     r.ChangeInfo,
		DeleteMessages: r.DeleteMessages,
		BanUsers:       r.BanUsers,
		InviteUsers:    r.InviteUsers,
		PinMessages:    r.PinMessages,
		AddAdmins:      r.AddAdmins,
	}
}

func to_chat_admin_rights(r AdminRights) tg.ChatAdminRights {
	return tg.ChatAdminRights{
		ChangeInfo:     r.ChangeInfo,
		DeleteMessages: r.DeleteMessages,
		BanUsers:       r.BanUsers,
		InviteUsers:    r.InviteUsers,
		PinMessages:    r.PinMessages,
		AddAdmins:      r.AddAdmins,
	}
}

func from_chat_banned_rights(r tg.ChatBannedRights) BannedRights {
	return BannedRights{
		ViewMessages: r.ViewMessages,
		SendMessages: r.SendMessages,
		SendMedia:    r.SendMedia,
		SendStickers: r.SendStickers,
		SendGifs:     r.SendGifs,
		SendGames:    r.SendGames,
		SendInline:   r.SendInline,
		EmbedLinks:   r.EmbedLinks,
		SendPolls:    r.SendPolls,
		ChangeInfo:   r.ChangeInfo,
		InviteUsers:  r.InviteUsers,
		PinMessages:  r.PinMessages,
	}
}

// to_chat_banned_rights sets the finer media and text flags together with
// their coarse ones, so a viewer is restricted for every kind of send.
func to_chat_banned_rights(r BannedRights) tg.ChatBannedRights {
	out := tg.ChatBannedRights{
		ViewMessages: r.ViewMessages,
		SendMessages: r.SendMessages,
		SendMedia:    r.SendMedia,
		SendStickers: r.SendStickers,
		SendGifs:     r.SendGifs,
		SendGames:    r.SendGames,
		SendInline:   r.SendInline,
		EmbedLinks:   r.EmbedLinks,
		SendPolls:    r.SendPolls,
		ChangeInfo:   r.ChangeInfo,
		InviteUsers:  r.InviteUsers,
		PinMessages:  r.PinMessages,
	}
	if r.SendMessages {
		out.SendPlain = true
	}
	if r.SendMedia {
		out.SendPhotos = true
		out.SendVideos = true
		out.SendAudios = true
		out.SendDocs = true
		out.SendVoices = true
		out.SendRoundvideos = true
	}
	return out
}

// is_not_participant reports Telegram's "not a member" answer, which is a
// normal result for role lookups rather than a failure.
func is_not_participant(err error) bool {
	return strings.Contains(strings.ToUpper(err.Error()), "USER_NOT_PARTICIPANT")
}

// map_telegram_error turns the Telegram errors role changes can hit into the
// package's stable codes. Other errors pass through unchanged.
func map_telegram_error(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToUpper(err.Error())
	switch {
	case strings.Contains(msg, "USER_NOT_PARTICIPANT"):
		return wrap_error(ErrNotMember, err)
	case strings.Contains(msg, "CHAT_ADMIN_REQUIRED"), strings.Contains(msg, "RIGHT_FORBIDDEN"), strings.Contains(msg, "USER_ADMIN_INVALID"):
		return wrap_error(ErrPermissionDenied, err)
	case strings.Contains(msg, "USER_ID_INVALID"), strings.Contains(msg, "PARTICIPANT_ID_INVALID"):
		return wrap_error(ErrUserNotFound, err)
	default:
		return err
	}
}

// NewGotdService builds a role Service on the shared gotd connection. kind,
// peers and self_id are the drive kind, the peer resolver and the calling
// account's user id; runner is the connection (the engine's client, or
// tgclient.StaticAPIRunner{API: client} around a raw *tg.Client in tests).
func NewGotdService(kind KindFunc, peers PeerFunc, self_id SelfIDFunc, runner tgclient.APIRunner) *Service {
	return NewService(kind, peers, self_id, NewGotdTelegram(runner))
}
