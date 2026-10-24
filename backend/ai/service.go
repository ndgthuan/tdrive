// Package ai holds TDrive's AI assistant backend: model profiles and their keys,
// provider clients for OpenAI-compatible, Anthropic and Gemini endpoints, chat
// sessions, and the photo organize planner with apply and undo.
//
// The package has no Wails dependency. internal/app wires it to the running
// engine through the Drive interface and an event emitter.
package ai

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"TDrive/backend/datadir"
)

// Event names emitted on the webview bus. Each payload reaches the frontend as
// a one-element array, like every other TDrive event.
const (
	EventMessageDelta     = "ai:message_delta"
	EventMessageDone      = "ai:message_done"
	EventOrganizeProgress = "ai:organize_progress"
	EventApplyProgress    = "ai:apply_progress"
)

// Options wires the service to its environment. Every field is optional except
// Drive when organize is wanted.
type Options struct {
	// DataDir returns the TDrive data directory. Default datadir.Dir.
	DataDir func() (string, error)
	// Secrets stores API keys. Default NewOSSecretStore.
	Secrets SecretStore
	// Drive resolves the drive access used by organize. Nil disables organize.
	Drive func() (Drive, error)
	// Emit publishes an event to the webview. Nil discards events.
	Emit func(name string, payload any)
	// HTTPClient is used for provider calls. Nil uses a client with a timeout.
	HTTPClient *http.Client
	// Now is the clock. Default time.Now.
	Now func() time.Time
	// ApprovalTimeout is how long an action waits for the user's answer before
	// it counts as denied. Default 5 minutes.
	ApprovalTimeout time.Duration
}

// Service is the AI backend entry point bound to the frontend.
type Service struct {
	opts Options

	mu           sync.Mutex
	manager      *Manager
	chats        *ChatStore
	plans        map[string]*Plan
	undos        map[string]*undo_record
	action_undos map[string]*action_undo
	approvals    map[string]chan bool
}

// NewService returns a service. Stores are opened lazily on first use so the
// data directory can be overridden at startup.
func NewService(opts Options) *Service {
	if opts.DataDir == nil {
		opts.DataDir = datadir.Dir
	}
	if opts.Secrets == nil {
		opts.Secrets = NewOSSecretStore()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Service{
		opts:         opts,
		plans:        map[string]*Plan{},
		undos:        map[string]*undo_record{},
		action_undos: map[string]*action_undo{},
		approvals:    map[string]chan bool{},
	}
}

// TestResult reports whether a model answered a tiny request.
type TestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// SendMessageRequest is one chat turn. Scope, when set, replaces the session's
// stored scope. ModelID empty uses the default model.
type SendMessageRequest struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
	Scope     *Scope `json:"scope,omitempty"`
	ModelID   string `json:"model_id"`
	// Mode is "agent" (the model may call drive tools) or "ask" (chat only).
	// Empty means "ask".
	Mode string `json:"mode"`
}

// MessageDelta is the ai:message_delta payload. The reply is not streamed:
// one delta carries the whole reply before ai:message_done follows.
type MessageDelta struct {
	SessionID string `json:"session_id"`
	Delta     string `json:"delta"`
}

// MessageDone is the ai:message_done payload.
type MessageDone struct {
	SessionID string `json:"session_id"`
	MessageID string `json:"message_id"`
}

// ListModels returns every model profile without keys.
func (s *Service) ListModels() ([]ModelView, error) {
	manager, err := s.models()
	if err != nil {
		return nil, err
	}
	return manager.ListModels()
}

// AddModel stores a new model profile. The API key is written to the OS
// credential store and never returned.
func (s *Service) AddModel(input ModelInput) (ModelView, error) {
	manager, err := s.models()
	if err != nil {
		return ModelView{}, err
	}
	return manager.AddModel(input)
}

// UpdateModel changes a profile. An empty APIKey keeps the stored key.
func (s *Service) UpdateModel(id string, input ModelInput) (ModelView, error) {
	manager, err := s.models()
	if err != nil {
		return ModelView{}, err
	}
	return manager.UpdateModel(id, input)
}

// DeleteModel removes a profile and its key.
func (s *Service) DeleteModel(id string) error {
	manager, err := s.models()
	if err != nil {
		return err
	}
	return manager.DeleteModel(id)
}

// SetDefaultModel makes id the default profile.
func (s *Service) SetDefaultModel(id string) error {
	manager, err := s.models()
	if err != nil {
		return err
	}
	return manager.SetDefaultModel(id)
}

// TestConnection sends a tiny request to a model. With model_id set, the stored
// profile is the base and non-empty draft fields override it; a draft APIKey is
// used in place of the stored one. Provider failures are reported in the result,
// not as an error.
func (s *Service) TestConnection(ctx context.Context, model_id string, draft ModelInput) (TestResult, error) {
	manager, err := s.models()
	if err != nil {
		return TestResult{}, err
	}
	var runtime ModelRuntime
	if model_id != "" {
		profile, err := manager.Profile(model_id)
		if err != nil {
			return TestResult{}, err
		}
		runtime.Profile = profile
		if draft.APIKey == "" {
			stored, err := manager.Resolve(model_id)
			if err != nil {
				return TestResult{}, err
			}
			runtime.APIKey = stored.APIKey
		}
		if strings.TrimSpace(draft.Model) != "" {
			runtime.Profile.Model = strings.TrimSpace(draft.Model)
		}
		if draft.Provider != "" {
			runtime.Profile.Provider = draft.Provider
		}
		if strings.TrimSpace(draft.BaseURL) != "" {
			runtime.Profile.BaseURL = normalize_base_url(draft.BaseURL)
		}
		if draft.APIKey != "" {
			runtime.APIKey = draft.APIKey
		}
	} else {
		if err := validate_input(draft, true); err != nil {
			return TestResult{}, err
		}
		runtime = ModelRuntime{Profile: profile_from_input("", draft), APIKey: draft.APIKey}
	}
	if err := validate_base_url(runtime.Profile.BaseURL, runtime.Profile.Provider == ProviderCustom); err != nil {
		return TestResult{}, err
	}
	completer, err := NewCompleter(runtime, s.opts.HTTPClient)
	if err != nil {
		return TestResult{}, err
	}
	probe_ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = completer.Complete(probe_ctx, Request{
		Turns:     []Turn{{Role: "user", Text: "Reply with the single word OK."}},
		MaxTokens: 16,
	})
	if err != nil {
		return TestResult{OK: false, Message: err.Error()}, nil
	}
	return TestResult{OK: true, Message: "Connected to " + runtime.Profile.Model}, nil
}

// CreateSession starts an empty chat in scope.
func (s *Service) CreateSession(title string, scope Scope) (ChatSession, error) {
	chats, err := s.chat_store()
	if err != nil {
		return ChatSession{}, err
	}
	return chats.Create(title, scope)
}

// ListSessions returns chat summaries, most recent first.
func (s *Service) ListSessions() ([]SessionSummary, error) {
	chats, err := s.chat_store()
	if err != nil {
		return nil, err
	}
	return chats.List()
}

// GetSession returns one chat with its messages.
func (s *Service) GetSession(id string) (ChatSession, error) {
	chats, err := s.chat_store()
	if err != nil {
		return ChatSession{}, err
	}
	return chats.Get(id)
}

// DeleteSession removes a chat. It does not touch any drive files.
func (s *Service) DeleteSession(id string) error {
	chats, err := s.chat_store()
	if err != nil {
		return err
	}
	return chats.Delete(id)
}

// SendMessage sends one user turn and returns the assistant reply. The call
// blocks until the provider answers, then persists both messages and emits
// ai:message_delta followed by ai:message_done. A failed call stores nothing.
func (s *Service) SendMessage(ctx context.Context, req SendMessageRequest) (ChatMessage, error) {
	text, err := validate_message_text(req.Text)
	if err != nil {
		return ChatMessage{}, err
	}
	mode, err := parse_chat_mode(req.Mode)
	if err != nil {
		return ChatMessage{}, err
	}
	chats, err := s.chat_store()
	if err != nil {
		return ChatMessage{}, err
	}
	session, err := chats.Get(req.SessionID)
	if err != nil {
		return ChatMessage{}, err
	}
	scope := session.Scope
	if req.Scope != nil {
		scope = *req.Scope
	}
	manager, err := s.models()
	if err != nil {
		return ChatMessage{}, err
	}
	runtime, err := manager.Resolve(req.ModelID)
	if err != nil {
		return ChatMessage{}, err
	}
	completer, err := NewCompleter(runtime, s.opts.HTTPClient)
	if err != nil {
		return ChatMessage{}, err
	}
	turns := history_for_model(session.Messages)
	turns = append(turns, Turn{Role: "user", Text: text})
	drive_name := s.drive_name(scope.ChannelID)

	var reply_text string
	var actions []ChatAction
	if mode == ModeAgent {
		drive, err := s.drive()
		if err != nil {
			return ChatMessage{}, err
		}
		env := agent_env{
			session_id: req.SessionID,
			scope:      scope,
			model_id:   req.ModelID,
			vision:     runtime.Profile.VisionEnabled,
			drive:      drive,
			drive_name: drive_name,
		}
		system := agent_system_prompt(scope, drive_name, env.vision)
		reply_text, actions, err = s.run_agent(ctx, env, system, turns, completer)
		if err != nil {
			if len(actions) == 0 {
				return ChatMessage{}, err
			}
			// Some actions already changed the drive. Keep them on record.
			reply_text = "Stopped early: " + err.Error() + ". The actions listed were already applied."
		}
	} else {
		reply, err := completer.Complete(ctx, Request{
			System:    chat_system_prompt(scope, drive_name),
			Turns:     turns,
			MaxTokens: 2048,
		})
		if err != nil {
			return ChatMessage{}, err
		}
		reply_text = reply.Text
	}
	user_message := chats.NewMessage("user", text, "")
	assistant := chats.NewMessage("assistant", reply_text, runtime.Profile.ID)
	assistant.Actions = actions
	_, err = chats.Update(req.SessionID, func(sess *ChatSession) error {
		sess.Scope = scope
		if len(sess.Messages) == 0 && sess.Title == "New chat" {
			sess.Title = title_from(text)
		}
		sess.Messages = append(sess.Messages, user_message, assistant)
		return nil
	})
	if err != nil {
		return ChatMessage{}, err
	}
	s.emit(EventMessageDelta, MessageDelta{SessionID: req.SessionID, Delta: assistant.Text})
	s.emit(EventMessageDone, MessageDone{SessionID: req.SessionID, MessageID: assistant.ID})
	return assistant, nil
}

// title_from makes a short session title from the first message.
func title_from(text string) string {
	const max_runes = 40
	line := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if utf8.RuneCountInString(line) > max_runes {
		return string([]rune(line)[:max_runes]) + "..."
	}
	return line
}

// models returns the lazily opened model manager.
func (s *Service) models() (*Manager, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.manager != nil {
		return s.manager, nil
	}
	dir, err := s.opts.DataDir()
	if err != nil {
		return nil, err
	}
	s.manager = NewManager(filepath.Join(dir, models_file_name), s.opts.Secrets)
	return s.manager, nil
}

// chat_store returns the lazily opened session store.
func (s *Service) chat_store() (*ChatStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chats != nil {
		return s.chats, nil
	}
	dir, err := s.opts.DataDir()
	if err != nil {
		return nil, err
	}
	s.chats = NewChatStore(filepath.Join(dir, sessions_dir_name))
	return s.chats, nil
}

// drive resolves the drive access for organize.
func (s *Service) drive() (Drive, error) {
	if s.opts.Drive == nil {
		return nil, errors.New("ai: drive access is not available")
	}
	return s.opts.Drive()
}

// drive_name is the root name of a drive for the model prompt. It is empty when
// the drive cannot be read, so the prompt falls back to the drive id.
func (s *Service) drive_name(channel_id int64) string {
	drive, err := s.drive()
	if err != nil || drive == nil {
		return ""
	}
	return drive.DriveName(channel_id)
}

func (s *Service) now() time.Time {
	return s.opts.Now()
}

func (s *Service) emit(name string, payload any) {
	if s.opts.Emit != nil {
		s.opts.Emit(name, payload)
	}
}
