package autobackup

import "time"

// LowBatteryPercent is the level at or below which an unplugged phone stops
// automatic uploads, so a backup never drains it to nothing.
const LowBatteryPercent = 15

// PowerStaleAfter is how old a native power reading may be before it no longer
// counts. A host that stops reporting must not leave a charging-only backup
// running on an old "charging" answer.
const PowerStaleAfter = 2 * time.Minute

// maxPowerClockSkew bounds how far in the future a reading may claim to be.
const maxPowerClockSkew = 30 * time.Second

// PowerState is one reading from the native host. LevelPercent is -1 when the
// host cannot say.
type PowerState struct {
	Known        bool      `json:"known"`
	Charging     bool      `json:"charging"`
	LevelPercent int       `json:"level_percent"`
	LowPowerMode bool      `json:"low_power_mode"`
	ObservedAt   time.Time `json:"-"`
}

// Block is why automatic uploads must wait. BlockNone means they may run.
type Block string

const (
	BlockNone         Block = ""
	BlockPowerUnknown Block = "power_unknown"
	BlockCharging     Block = "charging"
	BlockLowBattery   Block = "low_battery"
)

// Fresh reports whether a reading is recent enough to act on.
func (p PowerState) Fresh(now time.Time) bool {
	if !p.Known || p.ObservedAt.IsZero() {
		return false
	}
	if p.ObservedAt.After(now.Add(maxPowerClockSkew)) {
		return false
	}
	return now.Sub(p.ObservedAt) <= PowerStaleAfter
}

// GateUpload decides whether power allows uploads right now. With "only while
// charging" on, an unknown or stale reading blocks: the user asked for that
// guarantee, so it fails closed. Without it, a fresh reading still stops
// uploads on a low battery, but an unknown reading never blocks, because a
// host that cannot report power must not stop backup forever.
func GateUpload(settings Settings, power PowerState, now time.Time) Block {
	fresh := power.Fresh(now)
	if settings.ChargingOnly {
		if !fresh {
			return BlockPowerUnknown
		}
		if !power.Charging {
			return BlockCharging
		}
		return BlockNone
	}
	if fresh && !power.Charging && (power.LowPowerMode || (power.LevelPercent >= 0 && power.LevelPercent <= LowBatteryPercent)) {
		return BlockLowBattery
	}
	return BlockNone
}
