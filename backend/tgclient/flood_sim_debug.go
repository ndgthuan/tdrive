// TEST ONLY - DO NOT MERGE
// Simulates Telegram FLOOD_WAIT while listing dialogs. Driven by the Android
// system property debug.tdrive.flood ("old" or "fixed"); empty or unset means
// the simulation is off and nothing here changes behavior.

package tgclient

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/tgerr"
)

const (
	flood_sim_window        = 20 * time.Second
	flood_sim_probe_timeout = 2 * time.Second
	flood_sim_cache_ttl     = time.Second
)

var (
	flood_sim_now = time.Now

	flood_sim_cache_mu  sync.Mutex
	flood_sim_cached    string
	flood_sim_cached_at time.Time

	flood_sim_mu      sync.Mutex
	flood_sim_last_ok time.Time
)

// flood_sim_mode returns "old", "fixed", or "" when the simulation is off.
// The property is read at most once per second.
func flood_sim_mode() string {
	flood_sim_cache_mu.Lock()
	defer flood_sim_cache_mu.Unlock()
	now := flood_sim_now()
	if now.Sub(flood_sim_cached_at) < flood_sim_cache_ttl {
		return flood_sim_cached
	}
	flood_sim_cached = flood_sim_probe()
	flood_sim_cached_at = now
	return flood_sim_cached
}

// flood_sim_probe reads debug.tdrive.flood with getprop. Any error (for example
// on Windows or Linux dev machines) yields "".
func flood_sim_probe() string {
	for _, binary := range []string{"getprop", "/system/bin/getprop"} {
		ctx, cancel := context.WithTimeout(context.Background(), flood_sim_probe_timeout)
		out, err := exec.CommandContext(ctx, binary, "debug.tdrive.flood").Output()
		cancel()
		if err != nil {
			continue
		}
		switch strings.TrimSpace(string(out)) {
		case "old":
			return "old"
		case "fixed":
			return "fixed"
		}
		return ""
	}
	return ""
}

// flood_sim_gate admits one request per flood_sim_window. A request inside the
// window gets a FLOOD_WAIT_n error, where n is the remaining time rounded up.
func flood_sim_gate() error {
	mode := flood_sim_mode()
	flood_sim_mu.Lock()
	defer flood_sim_mu.Unlock()
	now := flood_sim_now()
	if now.Sub(flood_sim_last_ok) >= flood_sim_window {
		flood_sim_last_ok = now
		slog.Warn("FLOOD_SIM", "mode", mode, "decision", "allow", "remaining", 0)
		return nil
	}
	remaining := flood_sim_window - now.Sub(flood_sim_last_ok)
	seconds := int((remaining + time.Second - 1) / time.Second)
	slog.Warn("FLOOD_SIM", "mode", mode, "decision", "deny", "remaining", seconds)
	return tgerr.New(420, fmt.Sprintf("FLOOD_WAIT_%d", seconds))
}
