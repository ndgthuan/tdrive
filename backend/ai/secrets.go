package ai

import "errors"

// ErrNoAPIKey reports that no API key is stored for a model profile.
var ErrNoAPIKey = errors.New("ai: no API key stored for this model")

// credential_target_prefix namespaces AI keys in the OS credential store. The
// full target for a model is credential_target_prefix + model id.
const credential_target_prefix = "TDrive/ai/"

// SecretStore keeps provider API keys outside the data directory. Keys are
// written and read only by the backend; nothing returns them to the webview.
type SecretStore interface {
	// Set stores or replaces the key for a model id.
	Set(model_id string, api_key string) error
	// Get returns the key for a model id, or ErrNoAPIKey when none is stored.
	Get(model_id string) (string, error)
	// Delete removes the key for a model id. A missing key is not an error.
	Delete(model_id string) error
}
