// Package datadir centralizes where TDrive keeps its own files. On desktop,
// daemon and CLI this stays os.UserConfigDir()/TDrive for data and
// os.UserCacheDir()/TDrive for cache, exactly as before. On mobile those
// defaults can be wrong -- Go's home dir is /sdcard on Android when HOME is
// unset, so the OS-convention dirs land on shared external storage -- and the
// host overrides the bases with the app-private sandbox at startup.
package datadir

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// appDir is the per-app subdirectory every TDrive file lives under, kept the
// same across platforms so the on-disk layout matches everywhere.
const appDir = "TDrive"

// dirMode is the private mode TDrive's data directories are created with,
// matching every caller's historical 0700.
const dirMode os.FileMode = 0o700

var cacheTempPrefixes = [...]string{
	"tdrive-rendition-source-",
	"tdrive-upload-",
	".tdrive-upload-part-",
	"tdrive-enc-",
	"tdrive-mountdav-put-",
}

var (
	mu            sync.RWMutex
	dataOverride  string
	cacheOverride string
	// allow_default_in_tests is set only by this package's own tests, which
	// exercise the default resolution against a sandboxed HOME/APPDATA. Every
	// other test must call Set/SetCache with a temp dir.
	allow_default_in_tests bool
)

// is_test_binary reports whether this process is a Go test binary. It is a
// variable so the guard's own test can check the production branch.
var is_test_binary = testing.Testing

// refuse_real_dir panics when a test resolves a default data or cache dir
// without an override. Without this, a test that forgot to isolate itself
// writes into the developer's real %APPDATA%\TDrive or %LOCALAPPDATA%\TDrive.
// The panic fires before any directory is created.
func refuse_real_dir(op, override string) {
	if override != "" || !is_test_binary() {
		return
	}
	panic(fmt.Sprintf("datadir: %s resolved the real data dir from a test: call datadir.Set(t.TempDir()) and datadir.SetCache(t.TempDir()) first", op))
}

// Set overrides the base Dir builds on, replacing the os.UserConfigDir default.
// Mobile calls it once at startup, before any path is read, with the
// app-private storage path. An empty dir clears the override.
func Set(dir string) {
	mu.Lock()
	dataOverride = dir
	mu.Unlock()
}

// SetCache overrides the base CacheDir builds on, replacing the
// os.UserCacheDir default. Android needs it (no usable home dir); iOS keeps the
// default, which already resolves to the app's Library/Caches and stays out of
// backups. An empty dir clears the override.
func SetCache(dir string) {
	mu.Lock()
	cacheOverride = dir
	mu.Unlock()
}

// Dir returns TDrive's data/config directory, creating it 0700. Defaults to
// os.UserConfigDir()/TDrive; Set replaces the base. Under a test binary with no
// Set override it panics instead of touching the real config dir.
func Dir() (string, error) {
	mu.RLock()
	root := dataOverride
	allow := allow_default_in_tests
	mu.RUnlock()
	if !allow {
		refuse_real_dir("Dir", root)
	}
	return resolve(root, os.UserConfigDir)
}

// CacheDir returns TDrive's cache directory, creating it 0700. Defaults to
// os.UserCacheDir()/TDrive; SetCache replaces the base. Under a test binary
// with no SetCache override it panics instead of touching the real cache dir.
func CacheDir() (string, error) {
	mu.RLock()
	root := cacheOverride
	allow := allow_default_in_tests
	mu.RUnlock()
	if !allow {
		refuse_real_dir("CacheDir", root)
	}
	return resolve(root, os.UserCacheDir)
}

// UserCacheBase returns the native user cache root for callers that keep a
// folder beside TDrive's own cache, such as the Photos-library staging and the
// mount staging root. Production returns os.UserCacheDir unchanged. Under a test
// binary it returns a folder inside the SetCache override, so nothing reaches
// the real cache, and it panics when no override is set.
func UserCacheBase() (string, error) {
	mu.RLock()
	root := cacheOverride
	mu.RUnlock()
	if !is_test_binary() {
		return os.UserCacheDir()
	}
	refuse_real_dir("UserCacheBase", root)
	return filepath.Join(root, "native-cache"), nil
}

// CreateCacheTemp creates a private, app-owned temporary file. It is the
// single temporary-file boundary for data that may contain file bytes or
// decrypted metadata, so mobile builds never fall back to shared storage.
func CreateCacheTemp(pattern string) (*os.File, error) {
	dir, err := CacheDir()
	if err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, pattern)
}

// CleanupCacheTemps removes app-owned scratch files left by an interrupted
// upload, encryption, rendition, or mount write. It only examines the cache
// root and only removes names produced by CreateCacheTemp callers.
//
// A file it cannot delete is left where it is. Windows refuses to unlink a
// file another process still has open, and on that platform a second copy of
// the app -- or a test running beside this one -- is enough to make one of
// these undeletable. Tidying up is not a reason to refuse to start: the file
// is scratch, the next launch will try again, and the caller that owns it is
// the one that will finish with it.
func CleanupCacheTemps() error {
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("datadir: read cache temp directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !isOwnedCacheTemp(entry.Name()) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			slog.Debug("datadir: cache temp left in place", "name", entry.Name(), "error", err)
		}
	}
	return nil
}

func isOwnedCacheTemp(name string) bool {
	for _, prefix := range cacheTempPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// resolve derives the default base at call time, not at init, so tests that
// redirect HOME/XDG keep working through the defaults.
func resolve(root string, base func() (string, error)) (string, error) {
	if root == "" {
		var err error
		if root, err = base(); err != nil {
			return "", err
		}
	}

	dir := filepath.Join(root, appDir)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", err
	}
	_ = os.Chmod(dir, dirMode)
	return dir, nil
}
