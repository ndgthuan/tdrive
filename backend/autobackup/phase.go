package autobackup

// Phase is what the automatic backup is doing right now, in the order the
// pane explains it to the user.
type Phase string

const (
	PhaseOff                Phase = "off"
	PhaseNeedsPermission    Phase = "needs_permission"
	PhasePaused             Phase = "paused"
	PhaseWaitingForWiFi     Phase = "waiting_for_wifi"
	PhaseWaitingForCharging Phase = "waiting_for_charging"
	PhaseLowBattery         Phase = "low_battery"
	PhaseRunning            Phase = "running"
	PhaseUpToDate           Phase = "up_to_date"
	PhaseFailed             Phase = "failed"
)

// PhaseInput is everything DecidePhase needs. Connectivity and charging are
// tri-state through the Known flags: an unknown reading never blocks backup,
// because a host that cannot report power state would otherwise stop forever.
type PhaseInput struct {
	Settings      Settings
	Permission    Permission
	Paused        bool
	OnWiFi        bool
	WiFiKnown     bool
	Charging      bool
	ChargingKnown bool
	PendingItems  int64
	FailedItems   int64
	PowerBlock    Block
}

// DecidePhase orders the checks from the most blocking condition to the least.
// A denied permission outranks everything, because no amount of waiting makes
// the library readable. Paused outranks the network and power waits, since
// the user chose it explicitly.
func DecidePhase(in PhaseInput) Phase {
	if !in.Settings.Enabled {
		return PhaseOff
	}
	if in.Permission == PermissionDenied || in.Permission == PermissionUndetermined {
		return PhaseNeedsPermission
	}
	if in.Paused {
		return PhasePaused
	}
	if in.Settings.WiFiOnly && in.WiFiKnown && !in.OnWiFi {
		return PhaseWaitingForWiFi
	}
	if in.Settings.ChargingOnly && in.ChargingKnown && !in.Charging {
		return PhaseWaitingForCharging
	}
	switch in.PowerBlock {
	case BlockCharging, BlockPowerUnknown:
		return PhaseWaitingForCharging
	case BlockLowBattery:
		return PhaseLowBattery
	}
	if in.PendingItems > 0 {
		return PhaseRunning
	}
	if in.FailedItems > 0 {
		return PhaseFailed
	}
	return PhaseUpToDate
}
