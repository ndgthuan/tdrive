package app

import "TDrive/backend/applog"

// RecentBackendLog returns the newest backend log lines, redacted, for Copy
// diagnostics in Account > About. It reads the in-memory buffer only, so it
// works on every platform, including phones where the log file is out of reach.
// It fails when the logger was never installed in this process.
func (a *App) RecentBackendLog(maxLines int) (string, error) {
	return applog.RecentLog(maxLines)
}
