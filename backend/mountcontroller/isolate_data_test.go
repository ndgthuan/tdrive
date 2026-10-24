package mountcontroller

import (
	"os"
	"path/filepath"
	"testing"

	"TDrive/backend/datadir"
)

// isolate_data_dir points TDrive's config and cache dirs at fresh temp dirs for
// the rest of t. The datadir overrides are what the guard in backend/datadir
// checks, so a test that skips this call panics instead of writing to the
// developer's real %APPDATA%\TDrive or %LOCALAPPDATA%\TDrive. The environment
// variables are belt and braces for code that reads the OS dirs directly. They
// use os.Setenv, not t.Setenv, so the helper also works in parallel tests.
func isolate_data_dir(t *testing.T) {
	t.Helper()
	datadir.Set(t.TempDir())
	datadir.SetCache(t.TempDir())
	home := t.TempDir()
	env := map[string]string{
		"HOME":            home,
		"USERPROFILE":     home,
		"APPDATA":         filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA":    filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_CACHE_HOME":  filepath.Join(home, ".cache"),
	}
	for key, value := range env {
		prev, had := os.LookupEnv(key)
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("isolate %s: %v", key, err)
		}
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(key, prev)
				return
			}
			_ = os.Unsetenv(key)
		})
	}
	t.Cleanup(func() {
		datadir.Set("")
		datadir.SetCache("")
	})
}
