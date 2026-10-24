//go:build windows

package mountos

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestWindowsAttachAppliesLabelAndDetachClearsItForOwnedRemote(t *testing.T) {
	remote := testMappedRemote
	system := &windowsFakeSystem{remoteDrive: true, remoteResults: []remoteResult{
		{remote: remote, mapped: true},
		{remote: remote, mapped: true},
		{mapped: false},
	}}
	runner := &windowsRecordingRunner{}
	connector := newTestWindowsConnector(runner, system)
	var labeled_remote, labeled_name string
	var cleared []string
	connector.set_mount_label = func(remote, label string) error {
		labeled_remote, labeled_name = remote, label
		return nil
	}
	connector.clear_mount_label = func(remote string) error {
		cleared = append(cleared, remote)
		return nil
	}

	attachment, err := connector.Attach(context.Background(), Config{Endpoint: testEndpoint, Label: "Tdrive personal"})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	if labeled_remote != remote || labeled_name != "Telegram Drive" {
		t.Fatalf("label applied to (%q, %q); want the mapped remote and \"Telegram Drive\"", labeled_remote, labeled_name)
	}
	if err := connector.Detach(context.Background(), attachment); err != nil {
		t.Fatalf("Detach() error = %v", err)
	}
	if want := []string{testMappedRemote, testBareRemote}; !slices.Equal(cleared, want) {
		t.Fatalf("label cleanup calls = %q; want %q", cleared, want)
	}
}

func TestWindowsLabelFailureDoesNotFailAttach(t *testing.T) {
	remote := testMappedRemote
	system := &windowsFakeSystem{remoteDrive: true, remoteResults: []remoteResult{{remote: remote, mapped: true}}}
	connector := newTestWindowsConnector(&windowsRecordingRunner{}, system)
	connector.set_mount_label = func(string, string) error { return errors.New("registry denied") }

	if _, err := connector.Attach(context.Background(), Config{Endpoint: testEndpoint, Label: "Tdrive personal"}); err != nil {
		t.Fatalf("Attach() error = %v; a cosmetic label failure must not fail the mount", err)
	}
}
