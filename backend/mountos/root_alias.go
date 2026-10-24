package mountos

import "runtime"

// WindowsRootFolder is the one folder name the Windows mount exposes. Explorer
// names a deep-path network mapping after its last path segment, so the drive
// is mapped to <endpoint>/WindowsRootFolder and the server serves the drive
// root under that folder only. The server side of this contract lives in
// mountdav (StartConfig.RootAlias).
const WindowsRootFolder = "Telegram Drive"

// RootFolder returns the root alias the mount endpoint must serve on this
// platform. Only Windows uses one; macOS and Linux mount the capability root.
func RootFolder() string {
	if runtime.GOOS == "windows" {
		return WindowsRootFolder
	}
	return ""
}
