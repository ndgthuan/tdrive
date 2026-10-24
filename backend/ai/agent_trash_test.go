package ai

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// trashed_drive is agent_drive with the given ids already in the trash.
func trashed_drive(ids ...string) *fake_drive {
	drive := agent_drive()
	for _, id := range ids {
		drive.trashed[id] = true
	}
	return drive
}

func TestAgentSpecsDeclareTrashTools(t *testing.T) {
	want := map[string][]string{
		"list_trash":    nil,
		"restore_items": {"item_ids"},
		"empty_trash":   nil,
	}
	found := map[string]ToolSpec{}
	for _, spec := range agent_tool_specs() {
		found[spec.Name] = spec
	}
	for name, required := range want {
		spec, ok := found[name]
		if !ok {
			t.Fatalf("tool %s is not declared", name)
		}
		got, _ := spec.Parameters["required"].([]string)
		if !slices.Equal(got, required) {
			t.Fatalf("%s required = %v, want %v", name, got, required)
		}
	}
}

func TestListTrashShowsTrashedItemsWithoutChangingThem(t *testing.T) {
	drive := trashed_drive("1", "d:beach")
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "list_trash", map[string]any{}),
		text_reply("Two items are in the trash."),
	)
	reply := h.send(t, "what is in the trash")
	body := last_tool_result(t, h.script.body(1))
	for _, want := range []string{`"total":2`, `"name":"goku.png"`, `"name":"Beach"`, `"kind":"folder"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("list_trash result missing %s: %s", want, body)
		}
	}
	if len(reply.Actions) != 0 || len(h.events.names(EventActionRequest)) != 0 {
		t.Fatal("listing the trash is read only: no action, no approval")
	}
}

func TestEmptyTrashNeedsApprovalAndPurgesEverything(t *testing.T) {
	drive := trashed_drive("1", "d:beach")
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "empty_trash", map[string]any{}),
		text_reply("Permanently deleted 2 items from \"temp\"."),
	)
	go func() {
		req := <-h.requests
		if req.Tool != "empty_trash" || len(req.Items) != 2 {
			t.Errorf("approval request wrong: %+v", req)
		}
		if !strings.Contains(req.Summary, "PERMANENTLY") || !strings.Contains(req.Summary, "cannot be undone") {
			t.Errorf("summary must say the delete is permanent: %q", req.Summary)
		}
		if got := drive.purged_ids(); len(got) != 0 {
			t.Errorf("nothing may be purged before approval, got %v", got)
		}
		if err := h.svc.ResolveAction(req.RequestID, true); err != nil {
			t.Errorf("resolve: %v", err)
		}
	}()
	reply := h.send(t, "empty everything in the trash")
	if got := drive.purged_ids(); !slices.Equal(got, []string{"1", "d:beach"}) {
		t.Fatalf("purged = %v, want both trashed items", got)
	}
	if len(reply.Actions) != 1 || !reply.Actions[0].OK || reply.Actions[0].UndoToken != "" {
		t.Fatalf("empty action wrong, want OK with no undo token: %+v", reply.Actions)
	}
	if !strings.Contains(last_tool_result(t, h.script.body(1)), `"purged":2`) {
		t.Fatalf("model should receive the purge count: %s", h.script.body(1))
	}
}

func TestEmptyTrashDeniedPurgesNothing(t *testing.T) {
	drive := trashed_drive("1")
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "empty_trash", map[string]any{}),
		text_reply("Okay, the trash was not emptied."),
	)
	go func() {
		req := <-h.requests
		_ = h.svc.ResolveAction(req.RequestID, false)
	}()
	reply := h.send(t, "empty the trash")
	if got := drive.purged_ids(); len(got) != 0 || !drive.is_trashed("1") {
		t.Fatalf("denied empty must purge nothing: purged=%v", got)
	}
	if !strings.Contains(h.script.body(1), "user denied") {
		t.Fatalf("model should receive 'user denied': %s", h.script.body(1))
	}
	if len(reply.Actions) != 1 || reply.Actions[0].OK || reply.Actions[0].Error != "denied by user" {
		t.Fatalf("denied action should be recorded as not done: %+v", reply.Actions)
	}
}

func TestEmptyTrashTimeoutPurgesNothing(t *testing.T) {
	drive := trashed_drive("1")
	h := new_agent_harness(t, drive, true, 30*time.Millisecond,
		call_reply("c1", "empty_trash", map[string]any{}),
		text_reply("No answer, so nothing was deleted."),
	)
	reply := h.send(t, "empty the trash")
	if got := drive.purged_ids(); len(got) != 0 {
		t.Fatalf("a timed out empty must not run: purged=%v", got)
	}
	if len(reply.Actions) != 1 || reply.Actions[0].Error != "approval timed out" {
		t.Fatalf("timeout should count as denied: %+v", reply.Actions)
	}
}

func TestEmptyTrashWithSelectedIdsPurgesOnlyThose(t *testing.T) {
	drive := trashed_drive("1", "2")
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "empty_trash", map[string]any{"item_ids": []string{"2"}}),
		text_reply("Deleted dup.jpg permanently."),
	)
	go func() {
		req := <-h.requests
		if len(req.Items) != 1 || req.Items[0].Name != "dup.jpg" {
			t.Errorf("approval should name only the selected item: %+v", req.Items)
		}
		_ = h.svc.ResolveAction(req.RequestID, true)
	}()
	h.send(t, "delete dup.jpg from the trash for good")
	if got := drive.purged_ids(); !slices.Equal(got, []string{"2"}) {
		t.Fatalf("purged = %v, want only 2", got)
	}
	if !drive.is_trashed("1") {
		t.Fatal("an unselected trashed item must stay in the trash")
	}
}

func TestEmptyTrashIdNotInTrashIsReportedNotPurged(t *testing.T) {
	drive := trashed_drive("1")
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "empty_trash", map[string]any{"item_ids": []string{"4"}}),
		text_reply("That file is not in the trash."),
	)
	reply := h.send(t, "delete notes.txt for good")
	if len(h.events.names(EventActionRequest)) != 0 {
		t.Fatal("no approval should be asked when nothing selected is in the trash")
	}
	if got := drive.purged_ids(); len(got) != 0 {
		t.Fatalf("purged = %v, want none", got)
	}
	if len(reply.Actions) != 0 || !strings.Contains(h.script.body(1), "not in the trash") {
		t.Fatalf("model should learn the id is not in the trash: %s", h.script.body(1))
	}
}

func TestEmptyTrashWhenAlreadyEmptyDoesNotAsk(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "empty_trash", map[string]any{}),
		text_reply("The trash is already empty."),
	)
	h.send(t, "empty the trash")
	if len(h.events.names(EventActionRequest)) != 0 {
		t.Fatal("an empty trash needs no approval")
	}
	if !strings.Contains(h.script.body(1), "already empty") {
		t.Fatalf("model should be told the trash is empty: %s", h.script.body(1))
	}
}

func TestEmptyTrashStopsAtFirstFailureAndReportsRest(t *testing.T) {
	drive := trashed_drive("1", "2")
	drive.write_err = map[string]error{"2": errors.New("telegram flood wait")}
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "empty_trash", map[string]any{}),
		text_reply("Some items could not be deleted."),
	)
	go func() {
		req := <-h.requests
		_ = h.svc.ResolveAction(req.RequestID, true)
	}()
	reply := h.send(t, "empty the trash")
	if got := drive.purged_ids(); !slices.Equal(got, []string{"1"}) {
		t.Fatalf("purged = %v, want only 1 before the failure", got)
	}
	if !strings.Contains(last_tool_result(t, h.script.body(1)), `"remaining":1`) {
		t.Fatalf("model should see one item remaining: %s", h.script.body(1))
	}
	if len(reply.Actions) != 1 || reply.Actions[0].OK || !strings.Contains(reply.Actions[0].Error, "flood wait") {
		t.Fatalf("partial empty must not be reported as OK: %+v", reply.Actions)
	}
}

func TestRestoreItemsRunsWithoutApprovalAndUndoTrashesAgain(t *testing.T) {
	drive := trashed_drive("1")
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "restore_items", map[string]any{"item_ids": []string{"1"}}),
		text_reply("Restored goku.png."),
	)
	reply := h.send(t, "restore goku.png from the trash")
	if drive.is_trashed("1") {
		t.Fatal("restore should take the file out of the trash")
	}
	if len(h.events.names(EventActionRequest)) != 0 {
		t.Fatal("restore runs at once and must not ask for approval")
	}
	if len(reply.Actions) != 1 || !reply.Actions[0].OK || reply.Actions[0].UndoToken == "" {
		t.Fatalf("restore action wrong: %+v", reply.Actions)
	}
	if undo, err := h.svc.UndoAction(context.Background(), reply.Actions[0].UndoToken); err != nil || !undo.OK {
		t.Fatalf("undo: %+v %v", undo, err)
	}
	if !drive.is_trashed("1") {
		t.Fatal("undo of restore should move the item back to the trash")
	}
}

func TestRestoreRejectsItemsNotInTrash(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "restore_items", map[string]any{"item_ids": []string{"1"}}),
		text_reply("goku.png is not in the trash."),
	)
	reply := h.send(t, "restore goku.png")
	if !strings.Contains(h.script.body(1), "not in the trash") {
		t.Fatalf("model should see the refusal: %s", h.script.body(1))
	}
	if len(reply.Actions) != 0 {
		t.Fatalf("no action should be recorded when nothing was restored: %+v", reply.Actions)
	}
}

func TestRestoreItemsRejectsBadIds(t *testing.T) {
	drive := trashed_drive("1")
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "restore_items", map[string]any{"item_ids": []string{"../x"}}),
		text_reply("That id is not valid."),
	)
	h.send(t, "restore it")
	if !strings.Contains(h.script.body(1), "not a valid file or folder id") {
		t.Fatalf("bad id should be reported to the model: %s", h.script.body(1))
	}
	if !drive.is_trashed("1") {
		t.Fatal("a bad id call must not touch the trash")
	}
}

// last_tool_result returns the content of the last tool result the model was
// sent, decoded from the request body.
func last_tool_result(t *testing.T, body string) string {
	t.Helper()
	msgs := tool_messages(t, body)
	if len(msgs) == 0 {
		t.Fatalf("request has no tool result: %s", body)
	}
	return msgs[len(msgs)-1]
}
