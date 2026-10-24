//go:build windows

package ai

import (
	"errors"

	"github.com/danieljoos/wincred"
)

// OSSecretStore keeps keys in the Windows Credential Manager as generic
// credentials named TDrive/ai/<model id>.
type OSSecretStore struct{}

// NewOSSecretStore returns the credential-manager backed store for this OS.
func NewOSSecretStore() SecretStore {
	return OSSecretStore{}
}

// Set implements SecretStore.
func (OSSecretStore) Set(model_id string, api_key string) error {
	cred := wincred.NewGenericCredential(credential_target_prefix + model_id)
	cred.CredentialBlob = []byte(api_key)
	cred.Persist = wincred.PersistLocalMachine
	if err := cred.Write(); err != nil {
		return errors.New("ai: store API key in Windows Credential Manager: " + err.Error())
	}
	return nil
}

// Get implements SecretStore.
func (OSSecretStore) Get(model_id string) (string, error) {
	cred, err := wincred.GetGenericCredential(credential_target_prefix + model_id)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return "", ErrNoAPIKey
		}
		return "", errors.New("ai: read API key from Windows Credential Manager: " + err.Error())
	}
	return string(cred.CredentialBlob), nil
}

// Delete implements SecretStore.
func (OSSecretStore) Delete(model_id string) error {
	cred, err := wincred.GetGenericCredential(credential_target_prefix + model_id)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil
		}
		return errors.New("ai: read API key from Windows Credential Manager: " + err.Error())
	}
	if err := cred.Delete(); err != nil {
		return errors.New("ai: delete API key from Windows Credential Manager: " + err.Error())
	}
	return nil
}
