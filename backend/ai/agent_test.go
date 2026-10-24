package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// script serves scripted chat replies in order and records each request body.
type script struct {
	server  *httptest.Server
	mu      sync.Mutex
	replies []string
	bodies  []string
}

func new_script(t *testing.T, replies ...string) *script {
	t.Helper()
	sc := &script{replies: replies}
	sc.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		sc.mu.Lock()
		sc.bodies = append(sc.bodies, string(raw))
		index := len(sc.bodies) - 1
		sc.mu.Unlock()
		if index >= len(sc.replies) {
			http.Error(w, `{"error":{"message":"script exhausted"}}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":` + sc.replies[index] + `}]}`))
	}))
	t.Cleanup(sc.server.Close)
	return sc
}

func (sc *script) requests() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return len(sc.bodies)
}

func (sc *script) body(i int) string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if i >= len(sc.bodies) {
		return ""
	}
	return sc.bodies[i]
}

// call_reply is one assistant message asking for a single tool call.
func call_reply(id, name string, args any) string {
	raw, _ := json.Marshal(args)
	msg := map[string]any{
		"role":    "assistant",
		"content": nil,
		"tool_calls": []any{map[string]any{
			"id":   id,
			"type": "function",
			"function": map[string]any{
				"name":      name,
				"arguments": string(raw),
			},
		}},
	}
	data, _ := json.Marshal(msg)
	return string(data)
}

// text_reply is one plain assistant message.
func text_reply(text string) string {
	data, _ := json.Marshal(map[string]any{"role": "assistant", "content": text})
	return string(data)
}

// agent_harness wires a Service to a drive, a scripted model and an event log.
type agent_harness struct {
	svc      *Service
	drive    *fake_drive
	script   *script
	session  ChatSession
	events   *event_log
	requests chan ActionRequest
}

func new_agent_harness(t *testing.T, drive *fake_drive, vision bool, timeout time.Duration, replies ...string) *agent_harness {
	t.Helper()
	isolate_data_dir(t)
	h := &agent_harness{
		drive:    drive,
		script:   new_script(t, replies...),
		events:   &event_log{},
		requests: make(chan ActionRequest, 16),
	}
	h.svc = NewService(Options{
		Secrets:         new_fake_secrets(),
		Drive:           func() (Drive, error) { return drive, nil },
		ApprovalTimeout: timeout,
		Emit: func(name string, payload any) {
			h.events.emit(name, payload)
			if req, ok := payload.(ActionRequest); ok {
				h.requests <- req
			}
		},
	})
	if _, err := h.svc.AddModel(ModelInput{
		DisplayName:   "Agent",
		Provider:      ProviderCustom,
		BaseURL:       h.script.server.URL + "/v1",
		Model:         "agent-model",
		VisionEnabled: vision,
	}); err != nil {
		t.Fatalf("add model: %v", err)
	}
	session, err := h.svc.CreateSession("", Scope{ChannelID: 42})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	h.session = session
	return h
}

// send runs one agent-mode turn.
func (h *agent_harness) send(t *testing.T, text string) ChatMessage {
	t.Helper()
	reply, err := h.svc.SendMessage(context.Background(), SendMessageRequest{
		SessionID: h.session.ID,
		Text:      text,
		Mode:      ModeAgent,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	return reply
}

// agent_drive is the fixture for tool tests. Folders use d: ids as the
// production drive does.
func agent_drive() *fake_drive {
	return &fake_drive{
		drive_name: "temp",
		folders: []FolderNode{
			{ID: "d:beach", Name: "Beach", ParentID: ""},
			{ID: "d:trips", Name: "Trips", ParentID: ""},
		},
		files: []FileNode{
			{ID: 1, Name: "goku.png", ParentID: "", Size: 100},
			{ID: 2, Name: "dup.jpg", ParentID: ""},
			{ID: 3, Name: "dup.jpg", ParentID: "d:beach"},
			{ID: 4, Name: "notes.txt", ParentID: ""},
			{ID: 5, Name: "clip.mp4", ParentID: ""},
		},
		thumbs:    map[int64][]byte{},
		fail_move: map[int64]bool{},
		trashed:   map[string]bool{},
		write_err: map[string]error{},
	}
}

func TestAgentRenameFileThenUndo(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "rename_item", map[string]any{"item_id": "1", "new_name": "1.png"}),
		text_reply("Renamed goku.png to 1.png in \"temp\"."),
	)
	reply := h.send(t, "rename goku.png to 1.png")
	if drive.item_name("1") != "1.png" {
		t.Fatalf("file was not renamed, name is %q", drive.item_name("1"))
	}
	if len(reply.Actions) != 1 || !reply.Actions[0].OK || reply.Actions[0].Tool != "rename_item" || reply.Actions[0].UndoToken == "" {
		t.Fatalf("action not recorded: %+v", reply.Actions)
	}
	if !strings.Contains(strings.Join(tool_messages(t, h.script.body(1)), "\n"), "Renamed") {
		t.Fatalf("tool result not sent back to the model: %s", h.script.body(1))
	}
	if done := h.events.names(EventActionDone); len(done) != 1 {
		t.Fatalf("want one ai:action_done, got %d", len(done))
	}
	stored, err := h.svc.GetSession(h.session.ID)
	if err != nil || len(stored.Messages) != 2 || len(stored.Messages[1].Actions) != 1 {
		t.Fatalf("actions must persist on the assistant message: %+v %v", stored.Messages, err)
	}
	undo, err := h.svc.UndoAction(context.Background(), reply.Actions[0].UndoToken)
	if err != nil || !undo.OK {
		t.Fatalf("undo failed: %+v %v", undo, err)
	}
	if drive.item_name("1") != "goku.png" {
		t.Fatalf("undo did not restore the name, got %q", drive.item_name("1"))
	}
	again, _ := h.svc.UndoAction(context.Background(), reply.Actions[0].UndoToken)
	if again.OK {
		t.Fatal("an undo token must work once")
	}
}

func TestAgentMoveItemsIntoFolder(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "move_items", map[string]any{"item_ids": []string{"1", "4"}, "destination_folder_id": "d:beach"}),
		text_reply("Moved 2 items to Beach."),
	)
	reply := h.send(t, "move goku.png and notes.txt to Beach")
	if drive.file_parent(1) != "d:beach" || drive.file_parent(4) != "d:beach" {
		t.Fatalf("files not moved: %q %q", drive.file_parent(1), drive.file_parent(4))
	}
	if len(reply.Actions) != 1 || !reply.Actions[0].OK || len(reply.Actions[0].Items) != 2 {
		t.Fatalf("move action wrong: %+v", reply.Actions)
	}
	if _, err := h.svc.UndoAction(context.Background(), reply.Actions[0].UndoToken); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if drive.file_parent(1) != "" {
		t.Fatalf("undo should return the file to the root, got %q", drive.file_parent(1))
	}
}

func TestAgentCreateFolderAndUndoTrashesIt(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "create_folder", map[string]any{"name": "Photos 2026"}),
		text_reply("Created Photos 2026 in \"temp\"."),
	)
	reply := h.send(t, "make a folder called Photos 2026")
	if len(reply.Actions) != 1 || !reply.Actions[0].OK || len(reply.Actions[0].Items) != 1 {
		t.Fatalf("create action wrong: %+v", reply.Actions)
	}
	folder_id := reply.Actions[0].Items[0].ID
	if drive.is_trashed(folder_id) {
		t.Fatal("new folder should be live")
	}
	if undo, err := h.svc.UndoAction(context.Background(), reply.Actions[0].UndoToken); err != nil || !undo.OK {
		t.Fatalf("undo: %+v %v", undo, err)
	}
	if !drive.is_trashed(folder_id) {
		t.Fatal("undo of create should move the folder to the trash")
	}
}

func TestAgentDeleteRequiresApprovalAndCanBeUndone(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "delete_items", map[string]any{"item_ids": []string{"1"}}),
		text_reply("Moved goku.png to the trash."),
	)
	go func() {
		req := <-h.requests
		if req.Tool != "delete_items" || len(req.Items) != 1 || req.Items[0].Name != "goku.png" {
			t.Errorf("approval request wrong: %+v", req)
		}
		if err := h.svc.ResolveAction(req.RequestID, true); err != nil {
			t.Errorf("resolve: %v", err)
		}
	}()
	reply := h.send(t, "delete goku.png")
	if !drive.is_trashed("1") {
		t.Fatal("approved delete should trash the file")
	}
	if len(reply.Actions) != 1 || !reply.Actions[0].OK || reply.Actions[0].UndoToken == "" {
		t.Fatalf("delete action wrong: %+v", reply.Actions)
	}
	if undo, err := h.svc.UndoAction(context.Background(), reply.Actions[0].UndoToken); err != nil || !undo.OK {
		t.Fatalf("undo restore: %+v %v", undo, err)
	}
	if drive.is_trashed("1") {
		t.Fatal("undo should restore the file from the trash")
	}
}

func TestAgentDeleteDeniedLeavesFileAndTellsModel(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "delete_items", map[string]any{"item_ids": []string{"1"}}),
		text_reply("Okay, I will leave it."),
	)
	go func() {
		req := <-h.requests
		_ = h.svc.ResolveAction(req.RequestID, false)
	}()
	reply := h.send(t, "delete goku.png")
	if drive.is_trashed("1") {
		t.Fatal("denied delete must not touch the file")
	}
	if !strings.Contains(h.script.body(1), "user denied") {
		t.Fatalf("model should receive 'user denied': %s", h.script.body(1))
	}
	if len(reply.Actions) != 1 || reply.Actions[0].OK {
		t.Fatalf("denied action should be recorded as not done: %+v", reply.Actions)
	}
}

func TestAgentDeleteTimesOutAsDenied(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 30*time.Millisecond,
		call_reply("c1", "delete_items", map[string]any{"item_ids": []string{"1"}}),
		text_reply("No answer, so nothing was deleted."),
	)
	reply := h.send(t, "delete goku.png")
	if drive.is_trashed("1") {
		t.Fatal("a timed out delete must not run")
	}
	if !strings.Contains(h.script.body(1), "user denied") {
		t.Fatalf("timeout should reach the model as denied: %s", h.script.body(1))
	}
	if len(reply.Actions) != 1 || reply.Actions[0].OK || reply.Actions[0].Error != "approval timed out" {
		t.Fatalf("timeout action wrong: %+v", reply.Actions)
	}
	if err := h.svc.ResolveAction(h.events.names(EventActionRequest)[0].(ActionRequest).RequestID, true); err == nil {
		t.Fatal("an answer after the timeout must be rejected")
	}
}

func TestAgentPermissionErrorPassesThroughToModel(t *testing.T) {
	drive := agent_drive()
	drive.write_err = map[string]error{"1": errors.New("Only the uploader can rename this file in a shared drive")}
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "rename_item", map[string]any{"item_id": "1", "new_name": "x.png"}),
		text_reply("You cannot rename it; only its uploader can."),
	)
	reply := h.send(t, "rename goku.png to x.png")
	if drive.item_name("1") != "goku.png" {
		t.Fatal("a refused rename must not change the file")
	}
	if !strings.Contains(h.script.body(1), "Only the uploader can rename this file") {
		t.Fatalf("permission error should reach the model: %s", h.script.body(1))
	}
	if len(reply.Actions) != 1 || reply.Actions[0].OK || !strings.Contains(reply.Actions[0].Error, "uploader") {
		t.Fatalf("failed action should keep the error: %+v", reply.Actions)
	}
}

func TestAskModeSendsNoTools(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0, text_reply("I can chat."))
	if _, err := h.svc.SendMessage(context.Background(), SendMessageRequest{SessionID: h.session.ID, Text: "hi"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if strings.Contains(h.script.body(0), `"tools"`) {
		t.Fatalf("ask mode must not send tools: %s", h.script.body(0))
	}
}

func TestAgentModeSendsToolsAndRejectsUnknownMode(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0, text_reply("ok"))
	h.send(t, "hello")
	if !strings.Contains(h.script.body(0), `"rename_item"`) || !strings.Contains(h.script.body(0), `"view_media"`) {
		t.Fatalf("agent mode must declare the tools: %s", h.script.body(0))
	}
	if _, err := h.svc.SendMessage(context.Background(), SendMessageRequest{SessionID: h.session.ID, Text: "x", Mode: "bogus"}); err == nil {
		t.Fatal("unknown mode must be rejected")
	}
}

func TestAgentLoopIsCappedAtEightToolRounds(t *testing.T) {
	drive := agent_drive()
	var replies []string
	for i := 0; i < 12; i++ {
		replies = append(replies, call_reply(fmt.Sprintf("c%d", i), "list_files", map[string]any{}))
	}
	h := new_agent_harness(t, drive, true, 0, replies...)
	reply := h.send(t, "keep listing")
	if got := h.script.requests(); got != max_tool_rounds {
		t.Fatalf("want %d model calls, got %d", max_tool_rounds, got)
	}
	if !strings.Contains(reply.Text, "stopped after 8 tool steps") {
		t.Fatalf("cap should be explained, got %q", reply.Text)
	}
}

func TestAgentAmbiguousNameAsksInsteadOfGuessing(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "search_files", map[string]any{"query": "dup"}),
		text_reply("There are two dup.jpg files: one at the root and one in Beach. Which one do you mean?"),
	)
	reply := h.send(t, "delete dup.jpg")
	if len(drive.calls) != 0 {
		t.Fatalf("no change may happen while the name is ambiguous: %v", drive.calls)
	}
	if !strings.Contains(reply.Text, "Which one") {
		t.Fatalf("model should ask: %q", reply.Text)
	}
	results := strings.Join(tool_messages(t, h.script.body(1)), "\n")
	if !strings.Contains(results, `"path":"Beach"`) || !strings.Contains(results, `"id":"2"`) || !strings.Contains(results, `"id":"3"`) {
		t.Fatalf("search result should list both matches with paths: %s", results)
	}
}

func TestSearchFilesFindsNestedAndFiltersByKind(t *testing.T) {
	drive := agent_drive()
	env := agent_env{scope: Scope{ChannelID: 42}, drive: drive, drive_name: "temp"}
	svc := NewService(Options{Secrets: new_fake_secrets()})
	out := svc.tool_search_files(context.Background(), env, `{"query":"DUP","kind":"image"}`)
	if out.is_error || !strings.Contains(out.content, `"path":"Beach"`) || strings.Contains(out.content, "notes") {
		t.Fatalf("search wrong: %+v", out)
	}
	bad := svc.tool_search_files(context.Background(), env, `{"query":""}`)
	if !bad.is_error {
		t.Fatal("empty query must be an error")
	}
}

func TestMoveRejectsSourceChannelAndBadDestination(t *testing.T) {
	drive := agent_drive()
	drive.read_only = true
	env := agent_env{scope: Scope{ChannelID: 42}, drive: drive}
	svc := NewService(Options{Secrets: new_fake_secrets()})
	out := svc.tool_move_items(context.Background(), env, `{"item_ids":["1"],"destination_folder_id":"root"}`)
	if !out.is_error || !strings.Contains(out.content, "read-only") {
		t.Fatalf("channel source must be refused: %+v", out)
	}
	drive.read_only = false
	out = svc.tool_move_items(context.Background(), env, `{"item_ids":["1"],"destination_folder_id":"goku"}`)
	if !out.is_error || !strings.Contains(out.content, "destination") {
		t.Fatalf("non-folder destination must be refused: %+v", out)
	}
}

func TestUndoUnknownTokenReportsFailure(t *testing.T) {
	svc := NewService(Options{Secrets: new_fake_secrets(), Drive: func() (Drive, error) { return agent_drive(), nil }})
	result, err := svc.UndoAction(context.Background(), "missing")
	if err != nil || result.OK {
		t.Fatalf("unknown token should fail softly: %+v %v", result, err)
	}
}

func TestViewMediaNeedsVisionModel(t *testing.T) {
	drive := agent_drive()
	h := new_agent_harness(t, drive, false, 0,
		call_reply("c1", "view_media", map[string]any{"item_ids": []string{"1"}}),
		text_reply("This model cannot see pictures. Pick a vision model."),
	)
	h.send(t, "what is in goku.png")
	if !strings.Contains(h.script.body(1), "cannot see images") {
		t.Fatalf("model should be told it cannot see media: %s", h.script.body(1))
	}
	if drive.thumb_calls != 0 {
		t.Fatal("no preview should be fetched when vision is off")
	}
}

func TestViewMediaSendsImagesAndNotesMissingPreviews(t *testing.T) {
	drive := agent_drive()
	preview := png_bytes(t, 80, 60)
	drive.thumbs = map[int64][]byte{1: preview, 5: preview} // file 2 has no preview
	h := new_agent_harness(t, drive, true, 0,
		call_reply("c1", "view_media", map[string]any{"item_ids": []string{"1", "2", "5", "4", "d:beach"}}),
		text_reply("Two pictures viewed."),
	)
	h.send(t, "look at these")
	body := h.script.body(1)
	if got := strings.Count(body, "data:image/jpeg;base64,"); got != 2 {
		t.Fatalf("want 2 images (goku.png and the video poster), got %d", got)
	}
	for _, want := range []string{"1: goku.png (image", "clip.mp4 (video", "no preview available", "not a picture or video", "a folder, not a picture or video"} {
		if !strings.Contains(body, want) {
			t.Fatalf("view_media text should contain %q", want)
		}
	}
	if drive.thumb_calls != 3 {
		t.Fatalf("want one preview fetch per media item (3), got %d", drive.thumb_calls)
	}
}

func TestViewMediaRejectsMoreThanTwelve(t *testing.T) {
	drive := agent_drive()
	ids := make([]string, 0, 13)
	for i := 0; i < 13; i++ {
		ids = append(ids, fmt.Sprint(100+i))
	}
	env := agent_env{scope: Scope{ChannelID: 42}, drive: drive, vision: true}
	svc := NewService(Options{Secrets: new_fake_secrets()})
	args, _ := json.Marshal(map[string]any{"item_ids": ids})
	out := svc.tool_view_media(context.Background(), env, string(args))
	if !out.is_error || !strings.Contains(out.content, "at most 12") {
		t.Fatalf("13 items must be refused: %+v", out)
	}
}

func TestOrganizeWithVideosAndKindFilter(t *testing.T) {
	drive := photo_drive(t)
	drive.files = append(drive.files, FileNode{ID: 20, Name: "clip_beach.mp4", ParentID: ""})
	drive.thumbs[20] = png_bytes(t, 64, 48)
	fake := new_fake_vision_server(t, photo_rule)
	svc, _ := new_organize_service(t, drive, fake.server.URL+"/v1", true)

	videos := plan_request("")
	videos.Kind = "videos"
	plan, err := svc.PlanOrganize(context.Background(), videos)
	if err != nil {
		t.Fatalf("videos plan: %v", err)
	}
	if plan.Total != 1 || plan.Folders[0].Files[0].FileID != 20 {
		t.Fatalf("kind=videos should plan only the video: %+v", plan.Folders)
	}

	both, err := svc.PlanOrganize(context.Background(), plan_request(""))
	if err != nil {
		t.Fatalf("both plan: %v", err)
	}
	found := false
	for _, folder := range both.Folders {
		for _, file := range folder.Files {
			if file.FileID == 20 {
				found = true
			}
		}
	}
	if !found || both.Total < 2 {
		t.Fatalf("default kind should include videos with photos: total %d", both.Total)
	}

	bad := plan_request("")
	bad.Kind = "documents"
	if _, err := svc.PlanOrganize(context.Background(), bad); err == nil {
		t.Fatal("unknown kind must be rejected")
	}
}

func TestOrganizePhotosToolSummarisesPlan(t *testing.T) {
	drive := photo_drive(t)
	fake := new_fake_vision_server(t, photo_rule)
	svc, _ := new_organize_service(t, drive, fake.server.URL+"/v1", true)
	env := agent_env{scope: Scope{ChannelID: 42}, drive: drive, model_id: "", vision: true, drive_name: "temp"}
	out := svc.tool_organize_photos(context.Background(), env, `{"instruction":"group by place","kind":"both"}`)
	if out.is_error || out.action == nil || out.action.PlanID == "" {
		t.Fatalf("organize tool should return a plan: %+v", out)
	}
	if !strings.Contains(out.content, "nothing has moved") && !strings.Contains(out.content, "Nothing has moved") {
		t.Fatalf("result must say nothing moved: %s", out.content)
	}
	if drive.file_parent(1) != "" {
		t.Fatal("planning must not move files")
	}
}

// tool_messages returns the text of every tool-role message in an OpenAI-format
// request body, decoded so quotes are not escaped.
func tool_messages(t *testing.T, body string) []string {
	t.Helper()
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	var out []string
	for _, msg := range req.Messages {
		if text, ok := msg.Content.(string); ok && msg.Role == "tool" {
			out = append(out, text)
		}
	}
	return out
}
