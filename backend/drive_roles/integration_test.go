//go:build integration

// Integration test: runs the shared-drive role service against Telegram's
// official test servers (dcs.Test) with five fake test accounts. It is excluded
// from normal builds and CI by the build tag above.
//
// Required environment (read by the test, never printed):
//
//	TDRIVE_TEST_API_ID   the Telegram API id as a decimal number
//	TDRIVE_TEST_API_HASH the Telegram API hash
//
// Without both variables the test is skipped. The test never touches TDrive's
// data directory and never logs into a real account: every client uses an
// in-memory session and a phone number of the form 99966XYYYY on the test DC.
package drive_roles

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"TDrive/backend/auth"
	"TDrive/backend/tgclient"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	tgauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const itg_test_dc = 2

// itg_drive is the megagroup under test. Its peer is read by the services
// through the PeerFunc, so it is filled in before any service is used.
type itg_drive struct {
	channel_id  int64
	access_hash int64
}

func (d *itg_drive) peer(ctx context.Context, channel_id int64) (tgclient.InputPeer, error) {
	return tgclient.InputPeer{ChannelID: d.channel_id, AccessHash: d.access_hash}, nil
}

func (d *itg_drive) input_peer() *tg.InputPeerChannel {
	return &tg.InputPeerChannel{ChannelID: d.channel_id, AccessHash: d.access_hash}
}

func (d *itg_drive) input_channel() *tg.InputChannel {
	return &tg.InputChannel{ChannelID: d.channel_id, AccessHash: d.access_hash}
}

// itg_account is one fake Telegram user with its own gotd client and its own
// role service, so each user's cache and point of view stay separate.
type itg_account struct {
	t      *testing.T
	name   string
	client *telegram.Client
	api    *tg.Client
	id     int64
	access int64
	svc    *Service
	cancel context.CancelFunc
	done   chan error
}

func (a *itg_account) self(ctx context.Context) (int64, error) { return a.id, nil }

// itg_fixture carries the shared state of the scenario.
type itg_fixture struct {
	t        *testing.T
	ctx      context.Context
	drive    *itg_drive
	owner    *itg_account
	manager  *itg_account
	editor   *itg_account
	viewer   *itg_account
	stranger *itg_account
}

func TestRolesAgainstTelegramTestServer(t *testing.T) {
	id_text := os.Getenv("TDRIVE_TEST_API_ID")
	hash := os.Getenv("TDRIVE_TEST_API_HASH")
	if id_text == "" || hash == "" {
		t.Skip("TDRIVE_TEST_API_ID and TDRIVE_TEST_API_HASH are not set; skipping Telegram test-server integration test")
	}
	api_id, err := strconv.Atoi(id_text)
	if err != nil || api_id <= 0 {
		t.Fatalf("TDRIVE_TEST_API_ID is not a positive integer")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()

	names := []string{"owner", "manager", "editor", "viewer", "stranger"}
	accounts := make([]*itg_account, len(names))
	for i, name := range names {
		accounts[i] = &itg_account{t: t, name: name}
	}
	phones := itg_unique_phones(t, len(names), itg_test_dc)

	// Start all five clients and log them in on the test DC.
	var wg sync.WaitGroup
	login_errs := make([]error, len(names))
	for i, acc := range accounts {
		wg.Add(1)
		go func(i int, acc *itg_account) {
			defer wg.Done()
			login_errs[i] = itg_login(ctx, acc, api_id, hash, phones[i], itg_test_dc)
		}(i, acc)
	}
	wg.Wait()
	defer itg_stop_all(t, accounts)
	for i, err := range login_errs {
		if err != nil {
			t.Fatalf("login %s on test DC: %v", names[i], err)
		}
	}

	f := &itg_fixture{
		t:        t,
		ctx:      ctx,
		drive:    &itg_drive{},
		owner:    accounts[0],
		manager:  accounts[1],
		editor:   accounts[2],
		viewer:   accounts[3],
		stranger: accounts[4],
	}
	kind := func(int64) (DriveKind, error) { return DriveKindShared, nil }
	for _, acc := range accounts {
		acc.svc = NewGotdService(kind, f.drive.peer, acc.self, tgclient.StaticAPIRunner{API: acc.api})
	}

	// The owner creates the megagroup through the app's own helper.
	chID, chHash, err := auth.CreateMegagroup(ctx, f.owner.client, "TDrive roles test "+phones[0], "Automated role test, safe to delete")
	f.must(err, "create megagroup")
	f.drive.channel_id, f.drive.access_hash = chID, chHash
	defer f.delete_drive()

	f.phase_setup()
	f.phase_role_matrix()
	f.phase_default_viewer_and_override()
}

// phase_setup joins the three members, promotes and restricts them through the
// service, and checks the computed roles from both sides.
func (f *itg_fixture) phase_setup() {
	t := f.t
	link := f.invite_link()
	for _, acc := range []*itg_account{f.manager, f.editor, f.viewer} {
		f.join(acc, link)
		f.expect_role(acc, RoleEditor, "joined as default (no default restriction)")
	}

	f.must(f.owner.svc.SetRole(f.ctx, f.drive.channel_id, f.manager.id, RoleManager), "owner promotes manager")
	f.must(f.owner.svc.SetRole(f.ctx, f.drive.channel_id, f.viewer.id, RoleViewer), "owner restricts viewer")
	f.expect_role(f.manager, RoleManager, "manager after promotion")
	f.expect_role(f.viewer, RoleViewer, "viewer after restriction")

	// Transitions that exercise the lift/grant and clear/restrict plans.
	f.must(f.owner.svc.SetRole(f.ctx, f.drive.channel_id, f.viewer.id, RoleManager), "viewer -> manager")
	f.expect_role(f.viewer, RoleManager, "viewer promoted to manager")
	f.must(f.owner.svc.SetRole(f.ctx, f.drive.channel_id, f.viewer.id, RoleViewer), "manager -> viewer")
	f.expect_role(f.viewer, RoleViewer, "manager demoted back to viewer")
	f.must(f.owner.svc.SetRole(f.ctx, f.drive.channel_id, f.manager.id, RoleEditor), "manager -> editor")
	f.expect_role(f.manager, RoleEditor, "manager demoted to editor")
	f.must(f.owner.svc.SetRole(f.ctx, f.drive.channel_id, f.manager.id, RoleManager), "editor -> manager")
	f.expect_role(f.manager, RoleManager, "editor promoted back to manager")

	page, err := f.owner.svc.ListMembers(f.ctx, f.drive.channel_id, "", 0, 50)
	f.must(err, "owner lists members")
	want := map[int64]Role{
		f.owner.id:   RoleOwner,
		f.manager.id: RoleManager,
		f.editor.id:  RoleEditor,
		f.viewer.id:  RoleViewer,
	}
	if len(page.Members) != len(want) {
		t.Errorf("ListMembers returned %d members, want %d", len(page.Members), len(want))
	}
	for _, m := range page.Members {
		if want[m.UserID] != m.Role {
			t.Errorf("ListMembers user %d role = %s, want %s", m.UserID, m.Role, want[m.UserID])
		}
	}
	t.Logf("ListMembers: %d members with roles %v", len(page.Members), itg_role_summary(page.Members))
}

// phase_role_matrix runs every role against the same actions through the raw
// Telegram API and compares the outcome with the app-layer guard.
func (f *itg_fixture) phase_role_matrix() {
	t := f.t
	actors := []*itg_account{f.owner, f.manager, f.editor, f.viewer}
	f.t.Logf("%-10s %-16s %-6s %-9s %s", "actor", "action", "app", "telegram", "telegram error code")
	for _, actor := range actors {
		caps, err := actor.svc.RefreshMyRole(f.ctx, f.drive.channel_id)
		f.must(err, "caps for "+actor.name)
		author := f.owner
		if actor == f.owner {
			author = f.editor
		}

		// send_message and send_media: viewers must be refused by Telegram.
		f.matrix_row(actor, "send_message", caps.CanUpload, func() (bool, string) {
			_, err := f.raw_send_text(actor, "role matrix text")
			return err == nil, itg_code(err)
		})
		f.matrix_row(actor, "send_media", caps.CanUpload, func() (bool, string) {
			_, err := f.raw_send_file(actor, "matrix.txt")
			return err == nil, itg_code(err)
		})
		if actor.svc != nil && actor == f.viewer {
			f.check_viewer_send_codes(actor)
		}

		// delete_others: a fresh message by someone else, deleted by the actor.
		f.matrix_row_app(actor, "delete_others", RequireFileChange(caps, author.id, actor.id, "delete") == nil, func() (bool, string) {
			id, err := f.raw_send_text(author, "message by "+author.name)
			f.must(err, "author "+author.name+" sends message for delete_others")
			err = f.raw_delete(actor, id)
			return err == nil, itg_code(err)
		})

		// delete_own: the actor sends a message and deletes it. A viewer cannot send.
		f.matrix_row_app(actor, "delete_own", caps.CanUpload && RequireFileChange(caps, actor.id, actor.id, "delete") == nil, func() (bool, string) {
			id, err := f.raw_send_text(actor, "own message")
			if err != nil {
				return false, itg_code(err)
			}
			err = f.raw_delete(actor, id)
			return err == nil, itg_code(err)
		})

		// restrict_member: a raw editBanned on the viewer account.
		f.matrix_row_app(actor, "restrict_member", caps.CanManageMembers, func() (bool, string) {
			err := f.raw_restrict(actor, f.viewer, ViewerRights())
			return err == nil, itg_code(err)
		})

		// set_default: re-apply the current default (no restrictions).
		f.matrix_row_app(actor, "set_default", caps.CanManageMembers, func() (bool, string) {
			err := f.raw_set_default(actor, BannedRights{})
			return err == nil, itg_code(err)
		})

		// create_invite: exporting an invite link.
		f.matrix_row_app(actor, "create_invite", caps.CanInvite, func() (bool, string) {
			err := f.raw_export_invite(actor)
			return err == nil, itg_code(err)
		})

		// read_history: any member can read.
		f.matrix_row_app(actor, "read_history", caps.Role != "", func() (bool, string) {
			err := f.raw_history(actor)
			return err == nil, itg_code(err)
		})
	}
	_ = t
	// The restrict and default-rights calls above were idempotent. Make sure
	// the final roles are unchanged.
	f.expect_role(f.viewer, RoleViewer, "viewer after the role matrix")
	f.expect_role(f.editor, RoleEditor, "editor after the role matrix")
	f.expect_role(f.manager, RoleManager, "manager after the role matrix")
}

// check_viewer_send_codes asserts the refusal codes Telegram gives a viewer.
func (f *itg_fixture) check_viewer_send_codes(viewer *itg_account) {
	_, err := f.raw_send_text(viewer, "viewer text")
	if err == nil {
		f.t.Errorf("viewer sendMessage succeeded; Telegram did not enforce viewer")
		return
	}
	code := itg_code(err)
	if code != "CHAT_WRITE_FORBIDDEN" && code != "USER_BANNED_IN_CHANNEL" {
		f.t.Errorf("viewer sendMessage refused with %q, expected CHAT_WRITE_FORBIDDEN or USER_BANNED_IN_CHANNEL", code)
	}
	_, err = f.raw_send_file(viewer, "viewer.txt")
	if err == nil {
		f.t.Errorf("viewer sendMedia succeeded; Telegram did not enforce viewer")
		return
	}
	code = itg_code(err)
	if code != "CHAT_WRITE_FORBIDDEN" && code != "USER_BANNED_IN_CHANNEL" {
		f.t.Errorf("viewer sendMedia refused with %q, expected CHAT_WRITE_FORBIDDEN or USER_BANNED_IN_CHANNEL", code)
	}
}

// matrix_row compares the app decision with what Telegram did for one action.
// The app decision is the caller's bool. Telegram's answer comes from run.
func (f *itg_fixture) matrix_row(actor *itg_account, action string, app bool, run func() (bool, string)) {
	f.matrix_row_app(actor, action, app, run)
}

func (f *itg_fixture) matrix_row_app(actor *itg_account, action string, app bool, run func() (bool, string)) {
	tg_ok, code := run()
	f.t.Logf("%-10s %-16s %-6v %-9v %s", actor.name, action, app, tg_ok, code)
	if tg_ok != app {
		f.t.Errorf("%s %s: app guard = %v but Telegram = %v (code %q)", actor.name, action, app, tg_ok, code)
	}
}

// phase_default_viewer_and_override checks the viewer default, the stranger who
// joins through it, the explicit editor override, kick and ban.
func (f *itg_fixture) phase_default_viewer_and_override() {
	t := f.t
	ctx := f.ctx
	chID := f.drive.channel_id

	// The stranger is not a member yet.
	if _, err := f.stranger.svc.RefreshMyRole(ctx, chID); !errors.Is(err, ErrNotMember) {
		t.Errorf("stranger before joining: err = %v, want not_a_member", err)
	}

	f.must(f.owner.svc.SetDefaultRole(ctx, chID, RoleViewer), "owner sets default role viewer")
	got, err := f.owner.svc.GetDefaultRole(ctx, chID)
	f.must(err, "owner reads default role")
	if got != RoleViewer {
		t.Errorf("default role = %s, want viewer", got)
	}
	// Plain members with no individual record take the default restriction.
	f.expect_role(f.editor, RoleViewer, "plain editor becomes viewer under viewer default (design)")
	f.expect_role(f.manager, RoleManager, "manager unaffected by default restriction")

	link := f.invite_link()
	f.join(f.stranger, link)
	f.expect_role(f.stranger, RoleViewer, "stranger joins through viewer default")
	if _, err := f.raw_send_text(f.stranger, "stranger before override"); err == nil {
		t.Errorf("stranger viewer sendMessage succeeded")
	} else {
		t.Logf("stranger viewer sendMessage refused: %s", itg_code(err))
	}

	// Explicit override: the owner promotes the stranger to editor.
	f.must(f.owner.svc.SetRole(ctx, chID, f.stranger.id, RoleEditor), "owner sets stranger to editor over viewer default")
	f.expect_role(f.stranger, RoleEditor, "stranger editor override sticks")
	if id, err := f.raw_send_text(f.stranger, "stranger after override"); err != nil {
		t.Errorf("stranger sendMessage after editor override: %s", itg_code(err))
	} else {
		t.Logf("stranger sendMessage after override ok (message %d)", id)
	}
	if _, err := f.raw_send_file(f.stranger, "override.txt"); err != nil {
		t.Errorf("stranger sendMedia after editor override: %s", itg_code(err))
	} else {
		t.Logf("stranger sendMedia after override ok")
	}
	page, err := f.owner.svc.ListMembers(ctx, chID, "", 0, 50)
	f.must(err, "owner lists members after override")
	if role := role_of_user(page.Members, f.stranger.id); role != RoleEditor {
		t.Errorf("ListMembers shows stranger as %q, want editor", role)
	}

	// Kick without ban: the stranger cannot read; an invite works again.
	f.must(f.owner.svc.RemoveMember(ctx, chID, f.stranger.id, false), "owner kicks stranger without ban")
	if err := f.raw_history(f.stranger); err == nil {
		t.Errorf("kicked stranger can still read history")
	} else {
		t.Logf("kicked stranger read refused: %s", itg_code(err))
	}
	if _, err := f.stranger.svc.RefreshMyRole(ctx, chID); !errors.Is(err, ErrNotMember) {
		t.Errorf("kicked stranger role err = %v, want not_a_member", err)
	}
	f.join(f.stranger, link)
	f.expect_role(f.stranger, RoleViewer, "stranger rejoins after non-ban kick (override cleared, default applies)")

	// Kick with ban: the stranger cannot rejoin.
	f.must(f.owner.svc.RemoveMember(ctx, chID, f.stranger.id, true), "owner kicks stranger with ban")
	if err := f.raw_history(f.stranger); err == nil {
		t.Errorf("banned stranger can still read history")
	}
	_, _, err = f.join_error(f.stranger, link)
	if err == nil {
		t.Errorf("banned stranger rejoined through invite")
	} else {
		code := itg_code(err)
		t.Logf("banned stranger rejoin refused: %s", code)
		if code != "USER_BANNED_IN_CHANNEL" {
			t.Errorf("banned rejoin code = %q, want USER_BANNED_IN_CHANNEL", code)
		}
	}
	page, err = f.owner.svc.ListMembers(ctx, chID, "", 0, 50)
	f.must(err, "owner lists members after ban")
	if role_of_user(page.Members, f.stranger.id) != "" {
		t.Errorf("banned stranger still listed as a member")
	}
}

// ---- drive and account helpers ---------------------------------------------

func (f *itg_fixture) must(err error, label string) {
	f.t.Helper()
	if err != nil {
		f.t.Fatalf("%s: %v", label, err)
	}
}

func (f *itg_fixture) invite_link() string {
	f.t.Helper()
	res, err := f.owner.svc.CreateInvite(f.ctx, f.drive.channel_id, "", false)
	f.must(err, "owner creates invite link")
	return res.Link
}

func (f *itg_fixture) join(acc *itg_account, link string) {
	f.t.Helper()
	if _, _, err := f.join_error(acc, link); err != nil {
		f.t.Fatalf("%s joins drive: %v", acc.name, err)
	}
}

func (f *itg_fixture) join_error(acc *itg_account, link string) (int64, int64, error) {
	hash, err := auth.ParseInviteHash(link)
	if err != nil {
		return 0, 0, err
	}
	var ch, ah int64
	err = itg_do(f.ctx, func() error {
		var e error
		ch, ah, e = auth.JoinByInvite(f.ctx, acc.api, hash)
		return e
	})
	return ch, ah, err
}

func (f *itg_fixture) expect_role(acc *itg_account, want Role, label string) {
	f.t.Helper()
	caps, err := acc.svc.RefreshMyRole(f.ctx, f.drive.channel_id)
	if err != nil {
		f.t.Errorf("%s (%s): MyRole error: %v", label, acc.name, err)
		return
	}
	if caps.Role != want {
		f.t.Errorf("%s (%s): role = %s, want %s", label, acc.name, caps.Role, want)
		return
	}
	f.t.Logf("%s (%s): role %s", label, acc.name, caps.Role)
}

func (f *itg_fixture) raw_send_text(acc *itg_account, text string) (int, error) {
	rid := itg_random_id()
	var upd tg.UpdatesClass
	err := itg_do(f.ctx, func() error {
		var e error
		upd, e = acc.api.MessagesSendMessage(f.ctx, &tg.MessagesSendMessageRequest{
			Peer:     f.drive.input_peer(),
			Message:  text,
			RandomID: rid,
		})
		return e
	})
	if err != nil {
		return 0, err
	}
	id, ok := itg_message_id(upd, rid)
	if !ok {
		return 0, fmt.Errorf("sendMessage: no message id in %T", upd)
	}
	return id, nil
}

func (f *itg_fixture) raw_send_file(acc *itg_account, name string) (int, error) {
	var file tg.InputFileClass
	err := itg_do(f.ctx, func() error {
		var e error
		file, e = uploader.NewUploader(acc.api).FromBytes(f.ctx, name, []byte("tdrive role test\n"))
		return e
	})
	if err != nil {
		return 0, err
	}
	rid := itg_random_id()
	var upd tg.UpdatesClass
	err = itg_do(f.ctx, func() error {
		var e error
		upd, e = acc.api.MessagesSendMedia(f.ctx, &tg.MessagesSendMediaRequest{
			Peer: f.drive.input_peer(),
			Media: &tg.InputMediaUploadedDocument{
				File:     file,
				MimeType: "text/plain",
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeFilename{FileName: name},
				},
			},
			Message:  "",
			RandomID: rid,
		})
		return e
	})
	if err != nil {
		return 0, err
	}
	id, ok := itg_message_id(upd, rid)
	if !ok {
		return 0, fmt.Errorf("sendMedia: no message id in %T", upd)
	}
	return id, nil
}

func (f *itg_fixture) raw_delete(acc *itg_account, msg_id int) error {
	return itg_do(f.ctx, func() error {
		_, err := acc.api.ChannelsDeleteMessages(f.ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: f.drive.input_channel(),
			ID:      []int{msg_id},
		})
		return err
	})
}

func (f *itg_fixture) raw_restrict(actor, target *itg_account, rights BannedRights) error {
	return itg_do(f.ctx, func() error {
		_, err := actor.api.ChannelsEditBanned(f.ctx, &tg.ChannelsEditBannedRequest{
			Channel:      f.drive.input_channel(),
			Participant:  &tg.InputPeerUser{UserID: target.id, AccessHash: target.access},
			BannedRights: to_chat_banned_rights(rights),
		})
		return err
	})
}

func (f *itg_fixture) raw_set_default(actor *itg_account, rights BannedRights) error {
	return itg_do(f.ctx, func() error {
		_, err := actor.api.MessagesEditChatDefaultBannedRights(f.ctx, &tg.MessagesEditChatDefaultBannedRightsRequest{
			Peer:         f.drive.input_peer(),
			BannedRights: to_chat_banned_rights(rights),
		})
		return err
	})
}

func (f *itg_fixture) raw_export_invite(actor *itg_account) error {
	return itg_do(f.ctx, func() error {
		_, err := actor.api.MessagesExportChatInvite(f.ctx, &tg.MessagesExportChatInviteRequest{
			Peer: f.drive.input_peer(),
		})
		return err
	})
}

func (f *itg_fixture) raw_history(acc *itg_account) error {
	return itg_do(f.ctx, func() error {
		_, err := acc.api.MessagesGetHistory(f.ctx, &tg.MessagesGetHistoryRequest{
			Peer:  f.drive.input_peer(),
			Limit: 1,
		})
		return err
	})
}

// delete_drive removes the test megagroup. Errors are ignored by design.
func (f *itg_fixture) delete_drive() {
	if f.drive.channel_id == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, _ = f.owner.api.ChannelsDeleteChannel(ctx, f.drive.input_channel())
}

// itg_login starts one fake account's client and logs it in on the test DC.
// The client keeps running until itg_stop_all cancels it.
func itg_login(ctx context.Context, acc *itg_account, api_id int, api_hash, phone string, dc int) error {
	client := telegram.NewClient(api_id, api_hash, telegram.Options{
		DCList:         dcs.Test(),
		DC:             dc,
		SessionStorage: &session.StorageMemory{},
		NoUpdates:      true,
	})
	run_ctx, cancel := context.WithCancel(ctx)
	acc.client = client
	acc.cancel = cancel
	acc.done = make(chan error, 1)
	ready := make(chan error, 1)
	go func() {
		acc.done <- client.Run(run_ctx, func(run context.Context) error {
			err := itg_do(run, func() error {
				return client.Auth().IfNecessary(run, tgauth.NewFlow(itg_auth{phone: phone, dc: dc, t: acc.t}, tgauth.SendCodeOptions{}))
			})
			if err != nil {
				ready <- fmt.Errorf("auth flow: %w", err)
				return err
			}
			self, err := client.Self(run)
			if err != nil {
				ready <- fmt.Errorf("self: %w", err)
				return err
			}
			acc.id = self.ID
			acc.access = self.AccessHash
			acc.api = client.API()
			ready <- nil
			<-run.Done()
			return nil
		})
	}()
	select {
	case err := <-ready:
		return err
	case err := <-acc.done:
		if err == nil {
			err = errors.New("client stopped before login")
		}
		return err
	case <-time.After(3 * time.Minute):
		return errors.New("login timed out")
	}
}

func itg_stop_all(t *testing.T, accounts []*itg_account) {
	for _, acc := range accounts {
		if acc.cancel != nil {
			acc.cancel()
		}
	}
	for _, acc := range accounts {
		if acc.done == nil {
			continue
		}
		select {
		case <-acc.done:
		case <-time.After(30 * time.Second):
			t.Logf("client %s did not stop in time", acc.name)
		}
	}
}

// itg_do retries a call on FLOOD_WAIT, sleeping for the server's wait when it
// is at most 30 seconds. Any other error is returned as is.
func itg_do(ctx context.Context, fn func() error) error {
	for attempt := 0; ; attempt++ {
		err := fn()
		wait, ok := tgerr.AsFloodWait(err)
		if !ok || attempt >= 3 || wait > 30*time.Second {
			return err
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// itg_code is Telegram's error type (for example CHAT_WRITE_FORBIDDEN), or the
// error text when the error is not an RPC error.
func itg_code(err error) string {
	if err == nil {
		return ""
	}
	if rpc, ok := tgerr.As(err); ok {
		return rpc.Type
	}
	return err.Error()
}

func itg_random_id() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return int64(binary.LittleEndian.Uint64(b[:]))
}

// itg_message_id finds the id of a message sent with random id rid.
func itg_message_id(upd tg.UpdatesClass, rid int64) (int, bool) {
	switch u := upd.(type) {
	case *tg.UpdateShortSentMessage:
		return u.ID, true
	case *tg.Updates:
		return itg_id_in(u.Updates, rid)
	case *tg.UpdatesCombined:
		return itg_id_in(u.Updates, rid)
	}
	return 0, false
}

func itg_id_in(list []tg.UpdateClass, rid int64) (int, bool) {
	for _, raw := range list {
		if u, ok := raw.(*tg.UpdateMessageID); ok && u.RandomID == rid {
			return u.ID, true
		}
	}
	for _, raw := range list {
		switch u := raw.(type) {
		case *tg.UpdateNewChannelMessage:
			if m, ok := u.Message.(*tg.Message); ok {
				return m.ID, true
			}
		case *tg.UpdateNewMessage:
			if m, ok := u.Message.(*tg.Message); ok {
				return m.ID, true
			}
		}
	}
	return 0, false
}

// itg_unique_phones returns n distinct test-server numbers 99966XYYYY.
func itg_unique_phones(t *testing.T, n, dc int) []string {
	seen := map[int]bool{}
	out := make([]string, 0, n)
	for len(out) < n {
		v, err := rand.Int(rand.Reader, big.NewInt(10000))
		if err != nil {
			t.Fatalf("random phone: %v", err)
		}
		k := int(v.Int64())
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, fmt.Sprintf("99966%d%04d", dc, k))
	}
	return out
}

func role_of_user(list []Member, user_id int64) Role {
	for _, m := range list {
		if m.UserID == user_id {
			return m.Role
		}
	}
	return ""
}

func itg_role_summary(list []Member) map[int64]Role {
	out := make(map[int64]Role, len(list))
	for _, m := range list {
		out[m.UserID] = m.Role
	}
	return out
}

// itg_auth is the test-server login: 99966XYYYY numbers with the code made of
// the DC digit repeated for the length the server asks for.
type itg_auth struct {
	phone string
	dc    int
	t     *testing.T
}

func (a itg_auth) Phone(context.Context) (string, error) { return a.phone, nil }

func (a itg_auth) Password(context.Context) (string, error) {
	return "", tgauth.ErrPasswordNotProvided
}

func (a itg_auth) Code(_ context.Context, sent *tg.AuthSentCode) (string, error) {
	length := 5
	if sent != nil {
		if l, ok := sent.Type.(interface{ GetLength() int }); ok {
			length = l.GetLength()
		}
		if a.t != nil {
			a.t.Logf("sent code type %T length %d", sent.Type, length)
		}
	}
	return strings.Repeat(strconv.Itoa(a.dc), length), nil
}

func (a itg_auth) AcceptTermsOfService(context.Context, tg.HelpTermsOfService) error { return nil }

func (a itg_auth) SignUp(context.Context) (tgauth.UserInfo, error) {
	return tgauth.UserInfo{FirstName: "TDrive", LastName: "Test"}, nil
}
