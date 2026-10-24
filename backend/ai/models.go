package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Provider names the wire format a model profile speaks.
type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderAnthropic  Provider = "anthropic"
	ProviderGemini     Provider = "gemini"
	ProviderOpenRouter Provider = "openrouter"
	// ProviderCustom is any OpenAI-compatible endpoint; base_url is required.
	ProviderCustom Provider = "custom"
)

// models_file_name is the non-secret model profile store inside the data dir.
const models_file_name = "ai_models.json"

const (
	max_display_name_runes = 80
	max_model_name_runes   = 200
	max_api_key_bytes      = 4096
)

// ErrNoModel reports that no usable model profile exists or was named.
var ErrNoModel = errors.New("ai: no model configured; add a model in AI settings")

// ModelProfile is the persisted, non-secret part of a model configuration.
// The API key lives in the SecretStore under the profile id.
type ModelProfile struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name"`
	Provider      Provider `json:"provider"`
	BaseURL       string   `json:"base_url,omitempty"`
	Model         string   `json:"model"`
	VisionEnabled bool     `json:"vision_enabled"`
	IsDefault     bool     `json:"is_default"`
}

// ModelView is what the frontend sees: the profile plus whether a key exists.
type ModelView struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name"`
	Provider      Provider `json:"provider"`
	BaseURL       string   `json:"base_url"`
	Model         string   `json:"model"`
	VisionEnabled bool     `json:"vision_enabled"`
	IsDefault     bool     `json:"is_default"`
	HasAPIKey     bool     `json:"has_api_key"`
}

// ModelInput is the write payload for add and update. APIKey is write-only:
// on update an empty APIKey keeps the stored key, and ClearAPIKey removes it.
type ModelInput struct {
	DisplayName   string   `json:"display_name"`
	Provider      Provider `json:"provider"`
	BaseURL       string   `json:"base_url"`
	Model         string   `json:"model"`
	VisionEnabled bool     `json:"vision_enabled"`
	APIKey        string   `json:"api_key"`
	ClearAPIKey   bool     `json:"clear_api_key"`
}

// ModelRuntime is a resolved profile with its key. It is used only inside the
// backend to build a provider client and is never serialized.
type ModelRuntime struct {
	Profile ModelProfile
	APIKey  string
}

// model_file is the on-disk shape of ai_models.json.
type model_file struct {
	Version int            `json:"version"`
	Models  []ModelProfile `json:"models"`
}

// Manager owns the model profile list and its keys.
type Manager struct {
	mu      sync.Mutex
	path    string
	secrets SecretStore
}

// NewManager returns a manager that stores profiles at path and keys in secrets.
func NewManager(path string, secrets SecretStore) *Manager {
	return &Manager{path: path, secrets: secrets}
}

// ListModels returns every profile. IsDefault marks the default profile.
func (m *Manager) ListModels() ([]ModelView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	file, err := load_models(m.path)
	if err != nil {
		return nil, err
	}
	views := make([]ModelView, 0, len(file.Models))
	for _, profile := range file.Models {
		has_key, err := m.has_key(profile.ID)
		if err != nil {
			return nil, err
		}
		views = append(views, view_of(profile, has_key))
	}
	return views, nil
}

// AddModel validates and stores a new profile. The first profile added becomes
// the default. Non-custom providers require an API key.
func (m *Manager) AddModel(input ModelInput) (ModelView, error) {
	if err := validate_input(input, true); err != nil {
		return ModelView{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	file, err := load_models(m.path)
	if err != nil {
		return ModelView{}, err
	}
	profile := profile_from_input(uuid.NewString(), input)
	profile.IsDefault = true
	for _, existing := range file.Models {
		if existing.IsDefault {
			profile.IsDefault = false
			break
		}
	}
	if input.APIKey != "" {
		if err := m.secrets.Set(profile.ID, input.APIKey); err != nil {
			return ModelView{}, err
		}
	}
	file.Models = append(file.Models, profile)
	if err := save_models(m.path, file); err != nil {
		if input.APIKey != "" {
			_ = m.secrets.Delete(profile.ID)
		}
		return ModelView{}, err
	}
	return view_of(profile, input.APIKey != ""), nil
}

// UpdateModel replaces the profile fields of id and optionally rotates or
// clears its key.
func (m *Manager) UpdateModel(id string, input ModelInput) (ModelView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	file, err := load_models(m.path)
	if err != nil {
		return ModelView{}, err
	}
	index := find_model(file.Models, id)
	if index < 0 {
		return ModelView{}, ErrNoModel
	}
	if err := validate_input(input, false); err != nil {
		return ModelView{}, err
	}
	profile := file.Models[index]
	updated := profile_from_input(profile.ID, input)
	updated.IsDefault = profile.IsDefault

	stored, err := m.has_key(profile.ID)
	if err != nil {
		return ModelView{}, err
	}
	// Decide the key outcome before touching the store, so a refused update
	// never removes a key that is still needed.
	has_key := stored
	if input.ClearAPIKey {
		has_key = false
	} else if input.APIKey != "" {
		has_key = true
	}
	if updated.Provider != ProviderCustom && !has_key {
		return ModelView{}, errors.New("ai: this provider needs an API key")
	}
	switch {
	case input.ClearAPIKey:
		if err := m.secrets.Delete(profile.ID); err != nil {
			return ModelView{}, err
		}
	case input.APIKey != "":
		if err := m.secrets.Set(profile.ID, input.APIKey); err != nil {
			return ModelView{}, err
		}
	}
	file.Models[index] = updated
	if err := save_models(m.path, file); err != nil {
		return ModelView{}, err
	}
	return view_of(updated, has_key), nil
}

// DeleteModel removes a profile and its key. If the default is removed, the
// first remaining profile becomes the default.
func (m *Manager) DeleteModel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	file, err := load_models(m.path)
	if err != nil {
		return err
	}
	index := find_model(file.Models, id)
	if index < 0 {
		return ErrNoModel
	}
	was_default := file.Models[index].IsDefault
	file.Models = append(file.Models[:index], file.Models[index+1:]...)
	if was_default && len(file.Models) > 0 {
		file.Models[0].IsDefault = true
	}
	if err := save_models(m.path, file); err != nil {
		return err
	}
	return m.secrets.Delete(id)
}

// SetDefaultModel marks id as the single default profile.
func (m *Manager) SetDefaultModel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	file, err := load_models(m.path)
	if err != nil {
		return err
	}
	if find_model(file.Models, id) < 0 {
		return ErrNoModel
	}
	for i := range file.Models {
		file.Models[i].IsDefault = file.Models[i].ID == id
	}
	return save_models(m.path, file)
}

// Resolve returns the profile and key for id, or the default profile when id is
// empty. A non-custom provider without a stored key is an error.
func (m *Manager) Resolve(id string) (ModelRuntime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	file, err := load_models(m.path)
	if err != nil {
		return ModelRuntime{}, err
	}
	index := -1
	if id == "" {
		for i := range file.Models {
			if file.Models[i].IsDefault {
				index = i
				break
			}
		}
	} else {
		index = find_model(file.Models, id)
	}
	if index < 0 {
		return ModelRuntime{}, ErrNoModel
	}
	profile := file.Models[index]
	key, err := m.secrets.Get(profile.ID)
	if err != nil && !errors.Is(err, ErrNoAPIKey) {
		return ModelRuntime{}, err
	}
	if err != nil && profile.Provider != ProviderCustom {
		return ModelRuntime{}, fmt.Errorf("ai: model %q has no API key; add one in AI settings", profile.DisplayName)
	}
	return ModelRuntime{Profile: profile, APIKey: key}, nil
}

// Profile returns the stored profile for id without its key.
func (m *Manager) Profile(id string) (ModelProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	file, err := load_models(m.path)
	if err != nil {
		return ModelProfile{}, err
	}
	index := find_model(file.Models, id)
	if index < 0 {
		return ModelProfile{}, ErrNoModel
	}
	return file.Models[index], nil
}

func (m *Manager) has_key(id string) (bool, error) {
	_, err := m.secrets.Get(id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNoAPIKey) {
		return false, nil
	}
	return false, err
}

func find_model(models []ModelProfile, id string) int {
	for i := range models {
		if models[i].ID == id {
			return i
		}
	}
	return -1
}

func view_of(profile ModelProfile, has_key bool) ModelView {
	return ModelView{
		ID:            profile.ID,
		DisplayName:   profile.DisplayName,
		Provider:      profile.Provider,
		BaseURL:       profile.BaseURL,
		Model:         profile.Model,
		VisionEnabled: profile.VisionEnabled,
		IsDefault:     profile.IsDefault,
		HasAPIKey:     has_key,
	}
}

func profile_from_input(id string, input ModelInput) ModelProfile {
	return ModelProfile{
		ID:            id,
		DisplayName:   strings.TrimSpace(input.DisplayName),
		Provider:      input.Provider,
		BaseURL:       normalize_base_url(input.BaseURL),
		Model:         strings.TrimSpace(input.Model),
		VisionEnabled: input.VisionEnabled,
	}
}

// validate_input checks a write payload. require_key is true on add, where a
// non-custom provider must come with a key.
func validate_input(input ModelInput, require_key bool) error {
	name := strings.TrimSpace(input.DisplayName)
	if name == "" {
		return errors.New("ai: display name is required")
	}
	if utf8.RuneCountInString(name) > max_display_name_runes {
		return fmt.Errorf("ai: display name must be at most %d characters", max_display_name_runes)
	}
	switch input.Provider {
	case ProviderOpenAI, ProviderAnthropic, ProviderGemini, ProviderOpenRouter, ProviderCustom:
	default:
		return fmt.Errorf("ai: unknown provider %q", input.Provider)
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return errors.New("ai: model name is required")
	}
	if utf8.RuneCountInString(model) > max_model_name_runes {
		return fmt.Errorf("ai: model name must be at most %d characters", max_model_name_runes)
	}
	if err := validate_base_url(input.BaseURL, input.Provider == ProviderCustom); err != nil {
		return err
	}
	if len(input.APIKey) > max_api_key_bytes {
		return errors.New("ai: API key is too long")
	}
	if require_key && input.Provider != ProviderCustom && input.APIKey == "" {
		return errors.New("ai: an API key is required for this provider")
	}
	return nil
}

func validate_base_url(raw string, required bool) error {
	raw = normalize_base_url(raw)
	if raw == "" {
		if required {
			return errors.New("ai: base URL is required for a custom provider")
		}
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return errors.New("ai: base URL must be an absolute http or https URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("ai: base URL must not contain a query or fragment")
	}
	return nil
}

func normalize_base_url(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// load_models reads the profile file. A missing file is an empty list.
func load_models(path string) (model_file, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return model_file{Version: 1, Models: []ModelProfile{}}, nil
	}
	if err != nil {
		return model_file{}, fmt.Errorf("ai: read model configs: %w", err)
	}
	var file model_file
	if err := json.Unmarshal(data, &file); err != nil {
		return model_file{}, fmt.Errorf("ai: parse model configs: %w", err)
	}
	if file.Models == nil {
		file.Models = []ModelProfile{}
	}
	return file, nil
}

// save_models writes the profile file atomically with 0600 permissions.
func save_models(path string, file model_file) error {
	file.Version = 1
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ai: create data dir: %w", err)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("ai: encode model configs: %w", err)
	}
	return write_private_file(path, data)
}

// write_private_file replaces path with data via a temp file and rename.
func write_private_file(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("ai: write %s: %w", filepath.Base(path), err)
	}
	tmp_name := tmp.Name()
	defer func() { _ = os.Remove(tmp_name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ai: write %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("ai: write %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(tmp_name, 0o600); err != nil {
		return fmt.Errorf("ai: restrict %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp_name, path); err != nil {
		return fmt.Errorf("ai: replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
