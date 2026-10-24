//go:build windows

package mountos

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// The round trip writes real HKCU MountPoints2 state. The remote embeds a
// unique nanosecond suffix so it can never collide with a live tdrive mount,
// and the cleanup deletes the subkeys even when an assertion fails.
func TestWindowsMountLabelRegistryRoundTrip(t *testing.T) {
	remote := unc(fmt.Sprintf(`||127.0.0.1@59999|tdrive-test-%d`, time.Now().UnixNano()))
	t.Cleanup(func() { _ = windows_clear_mount_label(remote) })

	if err := windows_set_mount_label(remote, windows_mount_label); err != nil {
		t.Fatalf("windows_set_mount_label() error = %v", err)
	}
	names, err := build_label_key_variants(remote)
	if err != nil {
		t.Fatalf("build_label_key_variants() error = %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("build_label_key_variants() = %q; want two variants", names)
	}
	for _, name := range names {
		key, err := registry.OpenKey(registry.CURRENT_USER, windows_mount_points_path+`\`+name, registry.QUERY_VALUE)
		if err != nil {
			t.Fatalf("OpenKey(%q) error = %v", name, err)
		}
		value, _, err := key.GetStringValue(windows_label_value_name)
		key.Close()
		if err != nil || value != "Telegram Drive" {
			t.Fatalf("label value under %q = %q, %v; want \"Telegram Drive\"", name, value, err)
		}
	}

	if err := windows_clear_mount_label(remote); err != nil {
		t.Fatalf("windows_clear_mount_label() error = %v", err)
	}
	for _, name := range names {
		if _, err := registry.OpenKey(registry.CURRENT_USER, windows_mount_points_path+`\`+name, registry.QUERY_VALUE); !errors.Is(err, registry.ErrNotExist) {
			t.Fatalf("label key %q still present after clear: %v", name, err)
		}
	}
	if err := windows_clear_mount_label(remote); err != nil {
		t.Fatalf("second windows_clear_mount_label() error = %v; want nil for missing keys", err)
	}
}

func TestWindowsMountLabelRejectsMalformedRemoteWithoutRegistryAccess(t *testing.T) {
	malformed := unc(`||bad#host|share`)
	if err := windows_set_mount_label(malformed, windows_mount_label); err == nil {
		t.Fatal("windows_set_mount_label accepted a malformed remote")
	}
	if err := windows_clear_mount_label(malformed); err == nil {
		t.Fatal("windows_clear_mount_label accepted a malformed remote")
	}
}
