package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A viewer may not change anything in the drive, and the assistant says so with
// the one message the user sees. The gate runs before any drive call.
func TestOrganizeGuardRefusesViewer(t *testing.T) {
	drive := &fake_drive{role: "viewer"}
	err := organize_guard(context.Background(), drive, 7)
	if !errors.Is(err, ErrViewOnlyDrive) {
		t.Fatalf("viewer organize_guard = %v, want ErrViewOnlyDrive", err)
	}
	if err.Error() != "You only have view access to this drive" {
		t.Fatalf("viewer message = %q", err.Error())
	}
}

func TestOrganizeGuardAllowsEditorManagerAndOwner(t *testing.T) {
	for _, role := range []string{"owner", "manager", "editor"} {
		drive := &fake_drive{role: role}
		if err := organize_guard(context.Background(), drive, 7); err != nil {
			t.Fatalf("%s organize_guard = %v, want nil", role, err)
		}
	}
}

// An editor passes the assistant gate; the file service refuses the files they
// did not upload with its own uploader error, so this is not a second copy of
// that rule.
func TestOrganizeGuardLetsEditorThroughToFileRule(t *testing.T) {
	drive := &fake_drive{role: "editor"}
	if err := organize_guard(context.Background(), drive, 7); err != nil {
		t.Fatalf("editor organize_guard = %v, want nil", err)
	}
}

func TestOrganizeGuardKeepsChannelSourcesReadOnly(t *testing.T) {
	drive := &fake_drive{role: "owner", read_only: true}
	err := organize_guard(context.Background(), drive, 7)
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("channel source organize_guard = %v, want read-only refusal", err)
	}
}

// An unreadable role is a refusal, never a silent pass.
func TestOrganizeGuardRefusesWhenRoleUnknown(t *testing.T) {
	drive := &fake_drive{role: "not-a-role"}
	if err := organize_guard(context.Background(), drive, 7); err == nil {
		t.Fatal("unknown role must refuse the change")
	}
}

func TestCreateFolderToolRefusesViewerBeforeAnyWrite(t *testing.T) {
	drive := &fake_drive{role: "viewer"}
	env := agent_env{scope: Scope{ChannelID: 7}, drive: drive}
	out := (&Service{}).tool_create_folder(context.Background(), env, `{"name":"Taxes","parent_id":"root"}`)
	if !out.is_error || out.content != "You only have view access to this drive" {
		t.Fatalf("viewer create_folder output = %+v", out)
	}
	if len(drive.created) != 0 {
		t.Fatalf("viewer created folders: %v", drive.created)
	}
}

func TestListFilesReportsTheDriveRole(t *testing.T) {
	drive := &fake_drive{role: "viewer"}
	env := agent_env{scope: Scope{ChannelID: 7}, drive: drive}
	out := (&Service{}).tool_list_files(context.Background(), env, `{}`)
	if out.is_error {
		t.Fatalf("list_files failed: %s", out.content)
	}
	if !strings.Contains(out.content, `"role":"viewer"`) {
		t.Fatalf("list_files output lacks role: %s", out.content)
	}
}

func TestSystemPromptNamesTheRole(t *testing.T) {
	prompt := agent_system_prompt(Scope{ChannelID: 7}, "Team", "viewer", false)
	if !strings.Contains(prompt, "Your role in") || !strings.Contains(prompt, "is viewer") {
		t.Fatalf("system prompt does not name the role")
	}
}

func TestRoleLabelIsUnknownWhenLookupFails(t *testing.T) {
	drive := &fake_drive{role: "not-a-role"}
	if got := role_label(context.Background(), drive, 7); got != "unknown" {
		t.Fatalf("role_label = %q, want unknown", got)
	}
}
