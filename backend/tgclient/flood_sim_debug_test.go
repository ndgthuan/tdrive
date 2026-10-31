// TEST ONLY - DO NOT MERGE

package tgclient

import (
	"testing"
	"time"

	"github.com/gotd/td/tgerr"
)

func TestFloodSimGate(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	current := base
	saved_now := flood_sim_now
	saved_last := flood_sim_last_ok
	t.Cleanup(func() {
		flood_sim_now = saved_now
		flood_sim_last_ok = saved_last
	})
	flood_sim_now = func() time.Time { return current }
	flood_sim_last_ok = time.Time{}

	check_deny := func(offset time.Duration, want int) {
		t.Helper()
		current = base.Add(offset)
		err := flood_sim_gate()
		if err == nil {
			t.Fatalf("at +%s: expected FLOOD_WAIT_%d, got allow", offset, want)
		}
		rpc, ok := tgerr.As(err)
		if !ok || rpc.Argument != want {
			t.Fatalf("at +%s: expected FLOOD_WAIT_%d, got %v", offset, want, err)
		}
		if wait, ok := FloodWaitDuration(normalizeError(err)); !ok || wait != time.Duration(want)*time.Second {
			t.Fatalf("at +%s: normalized wait = %v, %v", offset, wait, ok)
		}
	}
	check_allow := func(offset time.Duration) {
		t.Helper()
		current = base.Add(offset)
		if err := flood_sim_gate(); err != nil {
			t.Fatalf("at +%s: expected allow, got %v", offset, err)
		}
	}

	check_allow(0)
	check_deny(5*time.Second, 15)
	check_deny(5500*time.Millisecond, 15)
	check_deny(19500*time.Millisecond, 1)
	check_allow(20 * time.Second)
	check_deny(21*time.Second, 19)
}
