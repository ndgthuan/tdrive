package autobackup

import (
	"path/filepath"
	"testing"
	"time"
)

func TestGateUploadChargingOnlyFailsClosed(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	charging := Settings{Enabled: true, ChargingOnly: true}
	cases := []struct {
		name  string
		power PowerState
		want  Block
	}{
		{"no reading", PowerState{}, BlockPowerUnknown},
		{"stale charging reading", PowerState{Known: true, Charging: true, LevelPercent: 80, ObservedAt: now.Add(-3 * time.Minute)}, BlockPowerUnknown},
		{"future reading", PowerState{Known: true, Charging: true, ObservedAt: now.Add(time.Minute)}, BlockPowerUnknown},
		{"on battery", PowerState{Known: true, Charging: false, LevelPercent: 90, ObservedAt: now}, BlockCharging},
		{"charging", PowerState{Known: true, Charging: true, LevelPercent: 40, ObservedAt: now}, BlockNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := GateUpload(charging, tc.power, now); got != tc.want {
				t.Fatalf("GateUpload = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGateUploadStopsOnLowBatteryWithoutChargingRule(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	plain := Settings{Enabled: true}
	if got := GateUpload(plain, PowerState{Known: true, LevelPercent: 15, ObservedAt: now}, now); got != BlockLowBattery {
		t.Fatalf("15%% unplugged = %q, want low battery", got)
	}
	if got := GateUpload(plain, PowerState{Known: true, LevelPercent: 16, ObservedAt: now}, now); got != BlockNone {
		t.Fatalf("16%% unplugged = %q, want none", got)
	}
	if got := GateUpload(plain, PowerState{Known: true, LowPowerMode: true, LevelPercent: 60, ObservedAt: now}, now); got != BlockLowBattery {
		t.Fatalf("low power mode = %q, want low battery", got)
	}
	if got := GateUpload(plain, PowerState{Known: true, Charging: true, LevelPercent: 5, ObservedAt: now}, now); got != BlockNone {
		t.Fatalf("charging at 5%% = %q, want none", got)
	}
	// An unknown reading never blocks a plain backup.
	if got := GateUpload(plain, PowerState{}, now); got != BlockNone {
		t.Fatalf("unknown power without charging rule = %q, want none", got)
	}
}

func TestDecidePhaseReportsPowerBlocks(t *testing.T) {
	on := Settings{Enabled: true, WiFiOnly: false}
	if got := DecidePhase(PhaseInput{Settings: on, Permission: PermissionGranted, PendingItems: 3, PowerBlock: BlockCharging}); got != PhaseWaitingForCharging {
		t.Fatalf("charging block = %q", got)
	}
	if got := DecidePhase(PhaseInput{Settings: on, Permission: PermissionGranted, PendingItems: 3, PowerBlock: BlockLowBattery}); got != PhaseLowBattery {
		t.Fatalf("low battery = %q", got)
	}
	if got := DecidePhase(PhaseInput{Settings: on, Permission: PermissionGranted, PendingItems: 3}); got != PhaseRunning {
		t.Fatalf("no block = %q", got)
	}
}

func TestFolderIDSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auto_backup.json")
	want := Settings{Enabled: true, WiFiOnly: true, FolderID: "folder-42", FolderName: "Pixel"}
	if err := SaveSettings(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path)
	if err != nil || got != want {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
}
