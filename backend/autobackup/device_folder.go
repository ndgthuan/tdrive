// Package autobackup holds the pure rules of automatic photo and video backup:
// which My Drive folder a phone uses, the persisted settings, what phase the
// backup is in, and how that folder is created idempotently. It performs no
// I/O of its own beyond the settings file, so every rule here is testable on
// any host. Discovery and upload stay with the photobackup engine.
package autobackup

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// DefaultFolderName is used when the platform gives no usable device identity.
const DefaultFolderName = "Phone"

// maxFolderNameBytes keeps a name well inside the drive's name limits.
const maxFolderNameBytes = 200

// Platform identifiers as the frontend and the native hosts report them.
const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
)

// androidManufacturers maps the manufacturer string Android reports (compared
// case-insensitively, exact match) to the folder name the user expects. An
// exact match is deliberate: a substring match would turn an unrelated brand
// that merely contains "vivo" or "oppo" into the wrong folder.
var androidManufacturers = map[string]string{
	"oppo":    "OPPO",
	"samsung": "Samsung",
	"xiaomi":  "Xiaomi",
	"huawei":  "HUAWEI",
	"oneplus": "OnePlus",
	"realme":  "Realme",
	"vivo":    "Vivo",
	"google":  "Pixel",
}

// DeviceFolderName returns the default My Drive folder for a device. The
// platform decides the iPhone/iPad split from the model string, so an iOS
// device with an unrecognised model still lands in the iPhone folder rather
// than a generic one.
func DeviceFolderName(platform, manufacturer, model string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformIOS:
		if strings.Contains(strings.ToLower(model), "ipad") {
			return "iPad"
		}
		return "iPhone"
	case PlatformAndroid:
		if name, ok := androidManufacturers[strings.ToLower(strings.TrimSpace(manufacturer))]; ok {
			return name
		}
	}
	return DefaultFolderName
}

// ResolveFolderName returns the folder to use: the user's rename when one is
// set and still valid, otherwise the device default. An invalid override
// falls back to the default instead of failing, so a stored bad value can
// never block backup.
func ResolveFolderName(override, platform, manufacturer, model string) string {
	fallback := DeviceFolderName(platform, manufacturer, model)
	if strings.TrimSpace(override) == "" {
		return fallback
	}
	name, err := SanitizeFolderName(override)
	if err != nil {
		return fallback
	}
	return name
}

// SanitizeFolderName validates a user-entered folder name. It rejects names
// that are empty once trimmed, that contain a character no folder may hold,
// or that are too long. Surrounding spaces and trailing dots are removed.
// Nothing is silently substituted: a name the user typed is either accepted
// as written or refused, so the rename dialog can say why.
func SanitizeFolderName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ErrInvalidFolderName
	}
	if !utf8.ValidString(trimmed) {
		return "", ErrInvalidFolderName
	}
	for _, char := range trimmed {
		if char < 0x20 || unicode.IsControl(char) || strings.ContainsRune(`<>:"/\|?*`, char) {
			return "", ErrInvalidFolderName
		}
	}
	name := strings.TrimRight(trimmed, ". ")
	if name == "" || name == "." || name == ".." {
		return "", ErrInvalidFolderName
	}
	if len([]byte(name)) > maxFolderNameBytes {
		return "", ErrInvalidFolderName
	}
	return name, nil
}
