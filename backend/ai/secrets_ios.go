//go:build ios && cgo

package ai

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework Security
#import <Foundation/Foundation.h>
#import <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static int tdrive_ai_secret(int operation, const char *account, const void *input, size_t size, void **output, size_t *length) {
    @autoreleasepool {
        NSMutableDictionary *query = [@{
            (__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
            (__bridge id)kSecAttrService: @"TDrive/ai",
            (__bridge id)kSecAttrAccount: [NSString stringWithUTF8String:account],
            (__bridge id)kSecAttrSynchronizable: @NO
        } mutableCopy];
        if (operation == 1) {
            NSData *data = [NSData dataWithBytes:input length:size];
            NSDictionary *attributes = @{
                (__bridge id)kSecValueData: data,
                (__bridge id)kSecAttrAccessible: (__bridge id)kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            };
            OSStatus status = SecItemUpdate((__bridge CFDictionaryRef)query, (__bridge CFDictionaryRef)attributes);
            if (status == errSecItemNotFound) {
                [query addEntriesFromDictionary:attributes];
                status = SecItemAdd((__bridge CFDictionaryRef)query, NULL);
            }
            return (int)status;
        }
        if (operation == 3) {
            OSStatus status = SecItemDelete((__bridge CFDictionaryRef)query);
            return status == errSecItemNotFound ? 0 : (int)status;
        }
        query[(__bridge id)kSecReturnData] = @YES;
        query[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
        CFTypeRef result = NULL;
        OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
        if (status != errSecSuccess) return status == errSecItemNotFound ? 1 : (int)status;
        NSData *data = CFBridgingRelease(result);
        if (data.length == 0 || data.length > 65536) return -1;
        *output = malloc(data.length);
        if (!*output) return -1;
        memcpy(*output, data.bytes, data.length);
        *length = data.length;
        return 0;
    }
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type OSSecretStore struct{}

func NewOSSecretStore() SecretStore                 { return OSSecretStore{} }
func (OSSecretStore) Set(id, key string) error      { _, err := iosSecret(1, id, key); return err }
func (OSSecretStore) Get(id string) (string, error) { return iosSecret(2, id, "") }
func (OSSecretStore) Delete(id string) error        { _, err := iosSecret(3, id, ""); return err }

func iosSecret(operation int, id, key string) (string, error) {
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
	status := C.tdrive_ai_secret(C.int(operation), account, input, C.size_t(len(data)), &output, &length)
	if status == 1 {
		return "", ErrNoAPIKey
	}
	if status != 0 {
		return "", fmt.Errorf("ai: Keychain operation failed (%d)", int(status))
	}
	if output == nil {
		return "", nil
	}
	defer C.free(output)
	bytes := unsafe.Slice((*byte)(output), int(length))
	defer clear(bytes)
	return string(bytes), nil
}
