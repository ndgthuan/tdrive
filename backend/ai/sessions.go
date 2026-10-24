package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// sessions_dir_name is the folder under the data dir holding one JSON file per
// chat session.
const sessions_dir_name = "ai_sessions"

const (
	max_message_runes = 8000
	max_title_runes   = 120
	// history_turns is how many prior messages are sent back to the model.
	history_turns = 20
)

// ErrSessionNotFound reports an unknown or malformed session id.
var ErrSessionNotFound = errors.New("ai: chat session not found")

// session_id_pattern restricts ids to UUID-like names so an id can never escape
// the sessions folder.
var session_id_pattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// Scope is the TDrive location a chat or organize action works in. ChannelID is
// the drive id. FolderID is the folder id, or empty for the drive root.
type Scope struct {
	ChannelID int64  `json:"channel_id"`
	FolderID  string `json:"folder_id"`
}

// ChatMessage is one stored message. Role is "user" or "assistant".
type ChatMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	ModelID   string `json:"model_id,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// ChatSession is a persisted conversation.
type ChatSession struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	Scope     Scope         `json:"scope"`
	CreatedAt int64         `json:"created_at"`
	UpdatedAt int64         `json:"updated_at"`
	Messages  []ChatMessage `json:"messages"`
}

// SessionSummary is the list view of a session without its messages.
type SessionSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Scope        Scope  `json:"scope"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	MessageCount int    `json:"message_count"`
}

// ChatStore persists sessions as <dir>/<id>.json. A mutex serializes access
// within the process; each write is atomic.
type ChatStore struct {
	mu  sync.Mutex
	dir string
	now func() time.Time
}

// NewChatStore returns a store rooted at dir, which is created on first write.
func NewChatStore(dir string) *ChatStore {
	return &ChatStore{dir: dir, now: time.Now}
}

// Create starts an empty session. A blank title gets a default.
func (s *ChatStore) Create(title string, scope Scope) (ChatSession, error) {
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) > max_title_runes {
		return ChatSession{}, fmt.Errorf("ai: session title must be at most %d characters", max_title_runes)
	}
	if title == "" {
		title = "New chat"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UnixMilli()
	session := ChatSession{
		ID:        uuid.NewString(),
		Title:     title,
		Scope:     scope,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []ChatMessage{},
	}
	if err := s.save(session); err != nil {
		return ChatSession{}, err
	}
	return session, nil
}

// List returns summaries, most recently updated first. Unreadable files are
// skipped and logged so one damaged session cannot hide the rest.
func (s *ChatStore) List() ([]SessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []SessionSummary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ai: list sessions: %w", err)
	}
	out := make([]SessionSummary, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		session, err := s.load(strings.TrimSuffix(name, ".json"))
		if err != nil {
			slog.Warn("ai: skipping unreadable chat session", "file", name, "error", err)
			continue
		}
		out = append(out, SessionSummary{
			ID:           session.ID,
			Title:        session.Title,
			Scope:        session.Scope,
			CreatedAt:    session.CreatedAt,
			UpdatedAt:    session.UpdatedAt,
			MessageCount: len(session.Messages),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// Get returns one full session.
func (s *ChatStore) Get(id string) (ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(id)
}

// Delete removes a session file. Chat history is not drive data, so deleting it
// is a plain file removal.
func (s *ChatStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("ai: delete session: %w", err)
	}
	return nil
}

// Update applies fn to a session under the store lock and persists the result.
// fn may return an error to abort without writing.
func (s *ChatStore) Update(id string, fn func(*ChatSession) error) (ChatSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.load(id)
	if err != nil {
		return ChatSession{}, err
	}
	if err := fn(&session); err != nil {
		return ChatSession{}, err
	}
	session.UpdatedAt = s.now().UnixMilli()
	if err := s.save(session); err != nil {
		return ChatSession{}, err
	}
	return session, nil
}

// NewMessage builds a stored message stamped with the store clock.
func (s *ChatStore) NewMessage(role, text, model_id string) ChatMessage {
	return ChatMessage{
		ID:        uuid.NewString(),
		Role:      role,
		Text:      text,
		ModelID:   model_id,
		CreatedAt: s.now().UnixMilli(),
	}
}

func (s *ChatStore) path(id string) (string, error) {
	if !session_id_pattern.MatchString(id) {
		return "", ErrSessionNotFound
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *ChatStore) load(id string) (ChatSession, error) {
	path, err := s.path(id)
	if err != nil {
		return ChatSession{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ChatSession{}, ErrSessionNotFound
	}
	if err != nil {
		return ChatSession{}, fmt.Errorf("ai: read session: %w", err)
	}
	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return ChatSession{}, fmt.Errorf("ai: parse session: %w", err)
	}
	if session.Messages == nil {
		session.Messages = []ChatMessage{}
	}
	return session, nil
}

func (s *ChatStore) save(session ChatSession) error {
	path, err := s.path(session.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("ai: create sessions dir: %w", err)
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("ai: encode session: %w", err)
	}
	return write_private_file(path, data)
}

// history_for_model returns the last history_turns messages as provider turns.
func history_for_model(messages []ChatMessage) []Turn {
	start := 0
	if len(messages) > history_turns {
		start = len(messages) - history_turns
	}
	turns := make([]Turn, 0, len(messages)-start)
	for _, msg := range messages[start:] {
		turns = append(turns, Turn{Role: msg.Role, Text: msg.Text})
	}
	return turns
}

// chat_system_prompt describes TDrive and the one assistant capability this
// backend offers. The model cannot move files itself. drive_name is the name
// the user sees for the drive; the model must use it rather than "My Drive",
// which is only the personal drive's name.
func chat_system_prompt(scope Scope, drive_name string) string {
	drive := "drive " + fmt.Sprint(scope.ChannelID)
	if drive_name != "" {
		drive = fmt.Sprintf("the drive named %q", drive_name)
	}
	location := "the root of " + drive
	if scope.FolderID != "" {
		location = "a folder inside " + drive
	}
	return strings.Join([]string{
		"You are the assistant inside TDrive, a desktop file manager that stores files in Telegram channels, called drives.",
		"Files live in folders on a drive. Some drives are personal or shared; channel sources are read-only and cannot be organized.",
		"The user is currently working in " + location + ".",
		"Name the drive exactly as given above. Only the personal drive is called My Drive; do not call a shared drive My Drive.",
		"Capability you can point the user to: \"Organize photos\". It scans the image files in the current folder (recursively), looks at small previews with a vision model, and proposes new folders for them.",
		"The user reviews the proposal and can exclude files before anything moves. Moves never delete files, and an undo is available afterwards.",
		"You cannot move, rename, or delete files yourself, and you must never claim that files were changed unless the user confirmed the action in the app.",
		"Be concise. If you are unsure which file or folder the user means, ask.",
	}, "\n")
}

// validate_message_text checks user input for a chat turn.
func validate_message_text(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("ai: message is empty")
	}
	if utf8.RuneCountInString(text) > max_message_runes {
		return "", fmt.Errorf("ai: message must be at most %d characters", max_message_runes)
	}
	return text, nil
}
