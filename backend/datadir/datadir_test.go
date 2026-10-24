package datadir

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// clearOverride resets the package-global override around a test so cases do
// not leak into each other.
func clearOverride(t *testing.T) {
	t.Helper()
	Set("")
	SetCache("")
	t.Cleanup(func() {
		Set("")
		SetCache("")
	})
}

// allow_real_defaults lets a test exercise the default resolution. The caller
// must sandbox HOME, XDG_* and the Windows profile env first.
func allow_real_defaults(t *testing.T) {
	t.Helper()
	allow_default_in_tests = true
	t.Cleanup(func() { allow_default_in_tests = false })
}

func TestDirDefaultsToUserConfigDir(t *testing.T) {
	clearOverride(t)
	allow_real_defaults(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	want := filepath.Join(base, "TDrive")

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if got != want {
		t.Fatalf("Dir = %q, want %q", got, want)
	}
	assertPrivateDir(t, got)
}

func TestCacheDirDefaultsToUserCacheDir(t *testing.T) {
	clearOverride(t)
	allow_real_defaults(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "LocalAppData"))

	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	want := filepath.Join(base, "TDrive")

	got, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if got != want {
		t.Fatalf("CacheDir = %q, want %q", got, want)
	}
	assertPrivateDir(t, got)
}

func TestOverridesAreIndependent(t *testing.T) {
	clearOverride(t)
	allow_real_defaults(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "LocalAppData"))

	data := t.TempDir()
	Set(data)
	if got, _ := Dir(); got != filepath.Join(data, "TDrive") {
		t.Fatalf("Dir = %q, want under %q", got, data)
	}
	// Set must leave the cache on its default, which is what iOS relies on.
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	if got, _ := CacheDir(); got != filepath.Join(base, "TDrive") {
		t.Fatalf("CacheDir after Set = %q, want default under %q", got, base)
	}

	cache := t.TempDir()
	SetCache(cache)
	got, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if got != filepath.Join(cache, "TDrive") {
		t.Fatalf("CacheDir = %q, want under %q", got, cache)
	}
	assertPrivateDir(t, got)
}

func TestOverrideResolvedAtCallTime(t *testing.T) {
	clearOverride(t)
	first := t.TempDir()
	Set(first)
	if got, _ := Dir(); got != filepath.Join(first, "TDrive") {
		t.Fatalf("Dir = %q, want under %q", got, first)
	}

	second := t.TempDir()
	Set(second)
	if got, _ := Dir(); got != filepath.Join(second, "TDrive") {
		t.Fatalf("Dir after re-Set = %q, want under %q", got, second)
	}
}

func TestCreateCacheTempUsesPrivateCacheOverride(t *testing.T) {
	clearOverride(t)
	base := t.TempDir()
	SetCache(base)
	file, err := CreateCacheTemp("test-private-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if got, want := filepath.Dir(file.Name()), filepath.Join(base, "TDrive"); got != want {
		t.Fatalf("temp parent = %q, want %q", got, want)
	}
}

func TestCleanupCacheTempsRemovesOnlyOwnedScratchFiles(t *testing.T) {
	clearOverride(t)
	base := t.TempDir()
	SetCache(base)
	cacheDir, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tdrive-upload-orphan", "tdrive-enc-orphan", ".tdrive-upload-part-orphan", "tdrive-mountdav-put-orphan"} {
		if err := os.WriteFile(filepath.Join(cacheDir, name), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "keep.cache"), []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := CleanupCacheTemps(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tdrive-upload-orphan", "tdrive-enc-orphan", ".tdrive-upload-part-orphan", "tdrive-mountdav-put-orphan"} {
		if _, err := os.Stat(filepath.Join(cacheDir, name)); !os.IsNotExist(err) {
			t.Fatalf("owned temp %q remains: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "keep.cache")); err != nil {
		t.Fatalf("unrelated cache removed: %v", err)
	}
}

// A scratch file that cannot be deleted is not a reason to refuse to start.
// Windows will not unlink a file another process has open, so a second copy
// of the app -- or, in CI, a test running beside this one -- was enough to
// stop the engine being created at all.
func TestCleanupCacheTempsKeepsGoingWhenOneCannotBeRemoved(t *testing.T) {
	clearOverride(t)
	base := t.TempDir()
	SetCache(base)
	cacheDir, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	// A directory under an owned name stands in for the undeletable file:
	// os.Remove refuses a non-empty one on every platform, which is the same
	// shape of failure as Windows refusing an open file.
	stuck := filepath.Join(cacheDir, "tdrive-mountdav-put-stuck")
	if err := os.MkdirAll(filepath.Join(stuck, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "tdrive-upload-orphan"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := CleanupCacheTemps(); err != nil {
		t.Fatalf("a scratch file nobody can delete stopped the cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "tdrive-upload-orphan")); !os.IsNotExist(err) {
		t.Fatalf("the deletable orphan remains: %v", err)
	}
}

// expect_panic runs fn and returns the panic message, failing the test when fn
// returns normally.
func expect_panic(t *testing.T, fn func()) (message string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic, got none")
		}
		message = fmt.Sprint(r)
	}()
	fn()
	return ""
}

func TestGuardPanicsWhenTestResolvesDefaultDataDir(t *testing.T) {
	clearOverride(t)
	msg := expect_panic(t, func() { _, _ = Dir() })
	if !strings.Contains(msg, "datadir.Set(t.TempDir())") {
		t.Fatalf("panic message does not name the fix: %q", msg)
	}
}

func TestGuardPanicsWhenTestResolvesDefaultCacheDir(t *testing.T) {
	clearOverride(t)
	msg := expect_panic(t, func() { _, _ = CacheDir() })
	if !strings.Contains(msg, "datadir.SetCache(t.TempDir())") {
		t.Fatalf("panic message does not name the fix: %q", msg)
	}
}

func TestGuardPanicsWhenTestResolvesUserCacheBase(t *testing.T) {
	clearOverride(t)
	expect_panic(t, func() { _, _ = UserCacheBase() })
}

func TestGuardDoesNotFireWithOverrides(t *testing.T) {
	clearOverride(t)
	Set(t.TempDir())
	SetCache(t.TempDir())
	if _, err := Dir(); err != nil {
		t.Fatalf("Dir with override: %v", err)
	}
	if _, err := CacheDir(); err != nil {
		t.Fatalf("CacheDir with override: %v", err)
	}
	native, err := UserCacheBase()
	if err != nil {
		t.Fatalf("UserCacheBase with override: %v", err)
	}
	if !strings.HasPrefix(native, cacheOverride) {
		t.Fatalf("UserCacheBase = %q, want a folder inside the cache override %q", native, cacheOverride)
	}
}

func TestGuardIsInertOutsideTestBinaries(t *testing.T) {
	clearOverride(t)
	// Production behavior: no panic, the default resolution is used. Sandbox
	// HOME/APPDATA so the default still resolves inside a temp dir.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "LocalAppData"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	prev := is_test_binary
	is_test_binary = func() bool { return false }
	t.Cleanup(func() { is_test_binary = prev })

	if _, err := Dir(); err != nil {
		t.Fatalf("Dir outside tests: %v", err)
	}
	if _, err := CacheDir(); err != nil {
		t.Fatalf("CacheDir outside tests: %v", err)
	}
	base, err := UserCacheBase()
	if err != nil {
		t.Fatalf("UserCacheBase outside tests: %v", err)
	}
	want, _ := os.UserCacheDir()
	if base != want {
		t.Fatalf("UserCacheBase outside tests = %q, want os.UserCacheDir %q", base, want)
	}
}

func assertPrivateDir(t *testing.T, dir string) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %q: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", dir)
	}
	// Windows does not honour unix permission bits.
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Fatalf("dir mode = %04o, want 0700", perm)
		}
	}
}
