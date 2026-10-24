package mountos

import (
	"runtime"
	"testing"
)

func TestRootFolderIsWindowsOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by the Windows root folder test")
	}
	if got := RootFolder(); got != "" {
		t.Fatalf("RootFolder() = %q on %s; want no alias", got, runtime.GOOS)
	}
}
