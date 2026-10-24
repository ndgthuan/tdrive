package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Event names for the agent loop. Payloads arrive as one-element arrays like
// every other TDrive event.
const (
	EventActionRequest = "ai:action_request"
	EventActionDone    = "ai:action_done"
)

const (
	// ModeAsk is plain chat with no tools. It is also the mode when none is given.
	ModeAsk = "ask"
	// ModeAgent lets the model call the drive tools.
	ModeAgent = "agent"
	// max_tool_rounds caps how many tool-call batches one user message may run.
	max_tool_rounds = 8
	// default_approval_timeout is how long an approval waits before it counts
	// as denied.
	default_approval_timeout = 5 * time.Minute
	// bulk_move_approval is the number of moved items above which a move needs
	// the user's approval.
	bulk_move_approval = 25
	// max_tool_items bounds the ids one tool call may name.
	max_tool_items = 200
	// max_view_items bounds the pictures one view_media call may return.
	max_view_items = 12
	// max_view_payload bounds the image bytes one view_media call sends.
	max_view_payload = 6 << 20
	// search_walk_limit bounds the folders visited by search_files.
	search_walk_limit = 2000
)

// ChatAction records one mutating tool call (or organize plan) in a chat
// message, so the UI can show what changed after a reload.
type ChatAction struct {
	Tool      string       `json:"tool"`
	Summary   string       `json:"summary"`
	OK        bool         `json:"ok"`
	Error     string       `json:"error,omitempty"`
	UndoToken string       `json:"undo_token,omitempty"`
	Items     []ActionItem `json:"items,omitempty"`
	PlanID    string       `json:"plan_id,omitempty"`
}

// ActionItem names one file or folder an action touched.
type ActionItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ActionRequest is the ai:action_request payload: an action waiting for the
// user's approval. Answer it with AIService.ResolveAction(request_id, approved).
type ActionRequest struct {
	SessionID string       `json:"session_id"`
	RequestID string       `json:"request_id"`
	Tool      string       `json:"tool"`
	Summary   string       `json:"summary"`
	Items     []ActionItem `json:"items"`
}

// ActionDone is the ai:action_done payload, sent for every action as it
// finishes, including approved-and-run, denied and failed ones.
type ActionDone struct {
	SessionID string       `json:"session_id"`
	Tool      string       `json:"tool"`
	Summary   string       `json:"summary"`
	OK        bool         `json:"ok"`
	Error     string       `json:"error,omitempty"`
	UndoToken string       `json:"undo_token,omitempty"`
	Items     []ActionItem `json:"items,omitempty"`
	PlanID    string       `json:"plan_id,omitempty"`
}

// UndoActionResult reports an undo. Failures come back with OK false and a
// message rather than as an error.
type UndoActionResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// action_undo is one entry in the generic undo registry. run performs the
// reverse operation against the drive current at undo time.
type action_undo struct {
	channel_id int64
	created_ms int64
	run        func(ctx context.Context, drive Drive) (string, error)
}

// agent_env is the per-message context the tools run in.
type agent_env struct {
	session_id string
	scope      Scope
	model_id   string
	vision     bool
	drive      Drive
	drive_name string
}

// tool_output is what one tool call returns to the model, plus the action it
// performed (nil for read-only tools).
type tool_output struct {
	content  string
	is_error bool
	images   []Image
	action   *ChatAction
}

// parse_chat_mode validates the mode of a chat turn. Empty means ask.
func parse_chat_mode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeAsk:
		return ModeAsk, nil
	case ModeAgent:
		return ModeAgent, nil
	default:
		return "", fmt.Errorf("ai: mode must be %q or %q", ModeAgent, ModeAsk)
	}
}

// ResolveAction answers a pending ai:action_request. Each request can be
// answered once; an unknown or already answered id is an error.
func (s *Service) ResolveAction(request_id string, approved bool) error {
	s.mu.Lock()
	ch, ok := s.approvals[request_id]
	if ok {
		delete(s.approvals, request_id)
	}
	s.mu.Unlock()
	if !ok {
		return errors.New("ai: approval request not found or already answered")
	}
	ch <- approved
	return nil
}

// UndoAction reverses an action by its undo_token. Tokens from organize applies
// are accepted too. Each token works once.
func (s *Service) UndoAction(ctx context.Context, token string) (UndoActionResult, error) {
	s.mu.Lock()
	record, ok := s.action_undos[token]
	if ok {
		delete(s.action_undos, token)
	}
	s.mu.Unlock()
	if !ok {
		result, err := s.UndoApply(ctx, token)
		if err != nil {
			return UndoActionResult{OK: false, Message: err.Error()}, nil
		}
		return UndoActionResult{
			OK:      len(result.Failed) == 0,
			Message: fmt.Sprintf("Restored %d file(s).", result.Restored),
		}, nil
	}
	drive, err := s.drive()
	if err != nil {
		return UndoActionResult{OK: false, Message: err.Error()}, nil
	}
	if err := drive.CanOrganize(record.channel_id); err != nil {
		return UndoActionResult{OK: false, Message: err.Error()}, nil
	}
	message, err := record.run(ctx, drive)
	if err != nil {
		return UndoActionResult{OK: false, Message: err.Error()}, nil
	}
	return UndoActionResult{OK: true, Message: message}, nil
}

// store_action_undo registers a generic undo and returns its token.
func (s *Service) store_action_undo(channel_id int64, run func(ctx context.Context, drive Drive) (string, error)) string {
	token := uuid.NewString()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.action_undos[token] = &action_undo{channel_id: channel_id, created_ms: s.now().UnixMilli(), run: run}
	return token
}

// prune_action_undos drops generic undo entries past undo_ttl. The caller holds s.mu.
func (s *Service) prune_action_undos_locked(now_ms int64) {
	cutoff := now_ms - undo_ttl.Milliseconds()
	for token, record := range s.action_undos {
		if record.created_ms < cutoff {
			delete(s.action_undos, token)
		}
	}
}

func (s *Service) approval_timeout() time.Duration {
	if s.opts.ApprovalTimeout > 0 {
		return s.opts.ApprovalTimeout
	}
	return default_approval_timeout
}

// await_approval emits ai:action_request and blocks until the user answers,
// the timeout passes or ctx ends. Only an explicit yes returns true.
func (s *Service) await_approval(ctx context.Context, env agent_env, tool, summary string, items []ActionItem) (bool, string) {
	request_id := uuid.NewString()
	answer := make(chan bool, 1)
	s.mu.Lock()
	s.approvals[request_id] = answer
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.approvals, request_id)
		s.mu.Unlock()
	}()
	if items == nil {
		items = []ActionItem{}
	}
	s.emit(EventActionRequest, ActionRequest{
		SessionID: env.session_id,
		RequestID: request_id,
		Tool:      tool,
		Summary:   summary,
		Items:     items,
	})
	timer := time.NewTimer(s.approval_timeout())
	defer timer.Stop()
	select {
	case approved := <-answer:
		if approved {
			return true, ""
		}
		return false, "denied by user"
	case <-timer.C:
		return false, "approval timed out"
	case <-ctx.Done():
		return false, "approval cancelled"
	}
}

// emit_action publishes ai:action_done for a finished action and returns it as a
// pointer so tools can hand it back to the loop.
func (s *Service) emit_action(env agent_env, action ChatAction) *ChatAction {
	s.emit(EventActionDone, ActionDone{
		SessionID: env.session_id,
		Tool:      action.Tool,
		Summary:   action.Summary,
		OK:        action.OK,
		Error:     action.Error,
		UndoToken: action.UndoToken,
		Items:     action.Items,
		PlanID:    action.PlanID,
	})
	return &action
}

// run_agent drives the tool loop: ask the model, run the tool calls it asks for,
// feed the results back, and repeat until it answers in plain text or the round
// cap is reached. Actions completed so far are returned even on error, so the
// caller can keep a record of what already changed.
func (s *Service) run_agent(ctx context.Context, env agent_env, system string, turns []Turn, completer Completer) (string, []ChatAction, error) {
	var actions []ChatAction
	for round := 0; round < max_tool_rounds; round++ {
		reply, err := completer.Complete(ctx, Request{
			System:    system,
			Turns:     turns,
			Tools:     agent_tool_specs(),
			MaxTokens: 2048,
		})
		if err != nil {
			return "", actions, err
		}
		if len(reply.ToolCalls) == 0 {
			text := strings.TrimSpace(reply.Text)
			if text == "" {
				text = "Done."
			}
			return text, actions, nil
		}
		turns = append(turns, Turn{Role: "assistant", Text: reply.Text, ToolCalls: reply.ToolCalls})
		results := make([]ToolResult, 0, len(reply.ToolCalls))
		for _, call := range reply.ToolCalls {
			if err := ctx.Err(); err != nil {
				return "", actions, err
			}
			out := s.run_tool(ctx, env, call)
			if out.action != nil {
				actions = append(actions, *out.action)
			}
			results = append(results, ToolResult{
				CallID:  call.ID,
				Name:    call.Name,
				Content: out.content,
				IsError: out.is_error,
				Images:  out.images,
			})
		}
		turns = append(turns, Turn{Role: "user", ToolResults: results})
	}
	return fmt.Sprintf("I stopped after %d tool steps for this message. The actions listed were applied; ask me to continue if more is needed.", max_tool_rounds), actions, nil
}

// run_tool dispatches one tool call. Unknown names and bad arguments are
// reported to the model as error results, never as Go errors.
func (s *Service) run_tool(ctx context.Context, env agent_env, call ToolCall) tool_output {
	switch call.Name {
	case "list_files":
		return s.tool_list_files(ctx, env, call.Arguments)
	case "search_files":
		return s.tool_search_files(ctx, env, call.Arguments)
	case "create_folder":
		return s.tool_create_folder(ctx, env, call.Arguments)
	case "rename_item":
		return s.tool_rename_item(ctx, env, call.Arguments)
	case "move_items":
		return s.tool_move_items(ctx, env, call.Arguments)
	case "delete_items":
		return s.tool_delete_items(ctx, env, call.Arguments)
	case "list_trash":
		return s.tool_list_trash(ctx, env, call.Arguments)
	case "restore_items":
		return s.tool_restore_items(ctx, env, call.Arguments)
	case "empty_trash":
		return s.tool_empty_trash(ctx, env, call.Arguments)
	case "organize_photos":
		return s.tool_organize_photos(ctx, env, call.Arguments)
	case "view_media":
		return s.tool_view_media(ctx, env, call.Arguments)
	default:
		return tool_output{content: "unknown tool " + call.Name, is_error: true}
	}
}

// tool_error is a failed tool result the model can read and explain.
func tool_error(err error) tool_output {
	return tool_output{content: err.Error(), is_error: true}
}

// denied_output is the result for an action the user did not approve.
func (s *Service) denied_output(env agent_env, tool, summary string, items []ActionItem, reason string) tool_output {
	action := s.emit_action(env, ChatAction{Tool: tool, Summary: summary, OK: false, Error: reason, Items: items})
	return tool_output{content: "user denied", is_error: true, action: action}
}

// drive_label is the name the user sees for the scope drive.
func drive_label(env agent_env) string {
	if env.drive_name != "" {
		return fmt.Sprintf("%q", env.drive_name)
	}
	return fmt.Sprintf("drive %d", env.scope.ChannelID)
}

// flex_string accepts a JSON string or number, since models vary on id types.
type flex_string string

func (f *flex_string) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*f = flex_string(strings.TrimSpace(text))
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("id must be a string or number")
	}
	*f = flex_string(number.String())
	return nil
}

// decode_args parses a tool's JSON arguments into dst.
func decode_args(raw string, dst any) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = "{}"
	}
	if err := json.Unmarshal([]byte(trimmed), dst); err != nil {
		return fmt.Errorf("invalid arguments: %v", err)
	}
	return nil
}

// valid_item_id reports whether id has the shape of a file or folder id.
func valid_item_id(id string) bool {
	if strings.HasPrefix(id, "d:") {
		return len(id) > len("d:")
	}
	n, err := parse_file_id(id)
	return err == nil && n > 0
}

// is_folder_id reports whether id names a folder.
func is_folder_id(id string) bool {
	return strings.HasPrefix(id, "d:")
}

// parent_arg turns a destination argument into a parent id. "", "root" and
// "/" mean the drive root.
func parent_arg(value string) (string, error) {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "root", "/":
		return "", nil
	}
	if !is_folder_id(value) {
		return "", fmt.Errorf("destination must be a folder id (d:...) or \"root\"")
	}
	return value, nil
}

// check_item_ids validates a list of ids: non-empty, bounded, well formed and
// without duplicates.
func check_item_ids(raw []flex_string) ([]string, error) {
	if len(raw) == 0 {
		return nil, errors.New("item_ids must name at least one file or folder id")
	}
	if len(raw) > max_tool_items {
		return nil, fmt.Errorf("at most %d items per call", max_tool_items)
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		id := string(value)
		if !valid_item_id(id) {
			return nil, fmt.Errorf("%q is not a valid file or folder id", id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// agent_system_prompt is the system prompt for agent mode.
func agent_system_prompt(scope Scope, drive_name string, vision bool) string {
	drive := fmt.Sprintf("drive %d", scope.ChannelID)
	if drive_name != "" {
		drive = fmt.Sprintf("the drive named %q", drive_name)
	}
	location := "the root of " + drive
	if scope.FolderID != "" {
		location = "a folder inside " + drive
	}
	lines := []string{
		"You are the assistant inside TDrive, a desktop file manager that stores files in Telegram channels, called drives.",
		"You ACT on files through tools. You can list and search files, look at pictures and videos, create folders, rename, move, and delete (delete only moves items to the trash, and the user must approve it). You can also look at the trash, restore items from it, and permanently delete items from it.",
		"Never claim you cannot rename, move, delete, restore, create folders or empty the trash. Use the tools.",
		"The trash: call list_trash to see what is in it. restore_items takes trashed items back to where they were; it runs at once. When the user asks to empty, clear or purge the trash (all of it, or some items), call empty_trash: omit item_ids to empty all of it, or pass item_ids for just those items. Call list_trash first when that helps you name what will go. Emptying is permanent and cannot be undone. The user must approve it, and it does not run until they do. Never refuse these requests and never say you cannot empty the trash.",
		"After empty_trash or restore_items, confirm in one short sentence what the tool reports: how many items were permanently deleted or restored, and the drive name. If the user denied it, say nothing was deleted.",
		"The user is working in " + location + ". Name the drive exactly as given: " + drive + ". Only the personal drive is called My Drive.",
		"Resolve names to ids first: use list_files for one folder and search_files across the whole drive. Never guess an id.",
		"If a name matches more than one item, ask the user a brief question which one they mean instead of guessing.",
		"Use rename_item, move_items (destination_folder_id is a folder id or \"root\"), create_folder, delete_items, restore_items and empty_trash for changes. Report what you did in one or two short sentences, with the drive name.",
		"Nothing has changed until a tool reports success. Do not claim a change that a tool did not confirm.",
		"Be concise.",
	}
	if vision {
		lines = append(lines,
			"You can look at pictures and videos with view_media (up to 12 items per call; video shows its poster frame). Use it to decide where something belongs.",
			"For large jobs (many photos or videos) do not call view_media again and again. Use organize_photos instead: it scans the folder in batches, proposes a plan, and the user applies it from the preview. Tell the user the plan summary and that nothing has moved yet. Report progress in plain words.",
		)
	} else {
		lines = append(lines,
			"The active model cannot see images or video, so view_media and organize_photos will fail. Tell the user to choose a vision-enabled model in the model settings if they need pictures to be looked at or sorted by content. You can still list, search, rename, move, delete, restore and empty the trash by name.",
		)
	}
	return strings.Join(lines, "\n")
}
