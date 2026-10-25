//go:build (ios || android) && !cgo

package ai

import "errors"

type OSSecretStore struct{}

func NewOSSecretStore() SecretStore { return OSSecretStore{} }
func (OSSecretStore) Set(string, string) error {
	return errors.New("ai: native credential store unavailable")
}
func (OSSecretStore) Get(string) (string, error) {
	return "", errors.New("ai: native credential store unavailable")
}
func (OSSecretStore) Delete(string) error {
	return errors.New("ai: native credential store unavailable")
}
