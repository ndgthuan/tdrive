//go:build !windows

package ai

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// keyring_service is the service name keys are filed under in the OS keyring
// (Keychain on macOS, Secret Service on Linux).
const keyring_service = "TDrive/ai"

// OSSecretStore keeps keys in the platform keyring.
type OSSecretStore struct{}

// NewOSSecretStore returns the keyring backed store for this OS.
func NewOSSecretStore() SecretStore {
	return OSSecretStore{}
}

// Set implements SecretStore.
func (OSSecretStore) Set(model_id string, api_key string) error {
	if err := keyring.Set(keyring_service, model_id, api_key); err != nil {
		return errors.New("ai: store API key in OS keyring: " + err.Error())
	}
	return nil
}

// Get implements SecretStore.
func (OSSecretStore) Get(model_id string) (string, error) {
	value, err := keyring.Get(keyring_service, model_id)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNoAPIKey
		}
		return "", errors.New("ai: read API key from OS keyring: " + err.Error())
	}
	return value, nil
}

// Delete implements SecretStore.
func (OSSecretStore) Delete(model_id string) error {
	err := keyring.Delete(keyring_service, model_id)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return errors.New("ai: delete API key from OS keyring: " + err.Error())
	}
	return nil
}
