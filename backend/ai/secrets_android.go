//go:build android && cgo

package ai

/*
#include <stdlib.h>
int tdrive_ai_android_secret(int operation, const char *id, const void *input, size_t size, void **output, size_t *length);
*/
import "C"

import (
	"errors"
	"unsafe"
)

type OSSecretStore struct{}

func NewOSSecretStore() SecretStore                 { return OSSecretStore{} }
func (OSSecretStore) Set(id, key string) error      { _, err := androidSecret(1, id, key); return err }
func (OSSecretStore) Get(id string) (string, error) { return androidSecret(2, id, "") }
func (OSSecretStore) Delete(id string) error        { _, err := androidSecret(3, id, ""); return err }

func androidSecret(operation int, id, key string) (string, error) {
	account := C.CString(id)
	defer C.free(unsafe.Pointer(account))
	data := []byte(key)
	defer clear(data)
	var input unsafe.Pointer
	if len(data) > 0 {
		input = unsafe.Pointer(&data[0])
	}
	var output unsafe.Pointer
	var length C.size_t
	status := C.tdrive_ai_android_secret(C.int(operation), account, input, C.size_t(len(data)), &output, &length)
	if status != 0 || output == nil {
		return "", errors.New("ai: Android Keystore unavailable")
	}
	defer C.free(output)
	bytes := unsafe.Slice((*byte)(output), int(length))
	defer clear(bytes)
	if len(bytes) == 0 {
		return "", errors.New("ai: invalid Keystore result")
	}
	if bytes[0] == 1 {
		return "", ErrNoAPIKey
	}
	if bytes[0] != 0 {
		return "", errors.New("ai: Android Keystore operation failed")
	}
	return string(bytes[1:]), nil
}
