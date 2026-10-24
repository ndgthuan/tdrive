//go:build windows

package mountos

import (
	"context"
	"slices"
	"testing"
)

func TestWindowsAttachMapsRootFolderUnderEndpoint(t *testing.T) {
	system := &windowsFakeSystem{remoteDrive: true, remoteResults: []remoteResult{{remote: testMappedRemote, mapped: true}}}
	runner := &windowsRecordingRunner{}
	connector := newTestWindowsConnector(runner, system)

	if _, err := connector.Attach(context.Background(), Config{Endpoint: testEndpoint, Label: "Tdrive personal"}); err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	if len(runner.plans) == 0 {
		t.Fatal("no net use command ran")
	}
	want := []string{"use", "T:", testEndpoint + WindowsRootFolder, "/persistent:no"}
	if got := runner.plans[0].Args; !slices.Equal(got, want) {
		t.Fatalf("net use args = %q; want %q", got, want)
	}
}

func TestWindowsRemoteMatchesRootFolderAndBareSpellings(t *testing.T) {
	if !windowsRemoteMatchesEndpoint(testMappedRemote, testEndpoint) {
		t.Fatal("root-folder mapping did not match its endpoint")
	}
	if !windowsRemoteMatchesEndpoint(testBareRemote, testEndpoint) {
		t.Fatal("bare capability remote did not match its endpoint")
	}
	if windowsRemoteMatchesEndpoint(testBareRemote+`\Other`, testEndpoint) {
		t.Fatal("a different folder under the capability matched")
	}
}

func TestWindowsExpectedRootFolderRemote(t *testing.T) {
	got, ok := windowsExpectedRemote(windowsRootEndpoint(testEndpoint))
	if !ok || got != testMappedRemote {
		t.Fatalf("windowsExpectedRemote(root endpoint) = %q, %v; want %q", got, ok, testMappedRemote)
	}
}

func TestWindowsAttachAcceptsBareRemoteReportedByWindows(t *testing.T) {
	system := &windowsFakeSystem{remoteDrive: true, remoteResults: []remoteResult{{remote: testBareRemote, mapped: true}}}
	var labeled []string
	connector := newTestWindowsConnector(&windowsRecordingRunner{}, system)
	connector.set_mount_label = func(remote, _ string) error {
		labeled = append(labeled, remote)
		return nil
	}

	if _, err := connector.Attach(context.Background(), Config{Endpoint: testEndpoint, Label: "Tdrive personal"}); err != nil {
		t.Fatalf("Attach() with bare remote error = %v", err)
	}
	if got := labeled[len(labeled)-1]; got != testBareRemote {
		t.Fatalf("final label remote = %q; want the reported bare remote", got)
	}
}

func TestWindowsRootFolderConstantMatchesPlatformFolder(t *testing.T) {
	if got := RootFolder(); got != WindowsRootFolder {
		t.Fatalf("RootFolder() = %q; want %q on Windows", got, WindowsRootFolder)
	}
}
