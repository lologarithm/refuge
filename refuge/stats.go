package refuge

import (
	"time"
)

// TempEvent is an event in the temp system.
// Used to track stats through history
type TempEvent struct {
	Name     string       // Name of device
	Time     time.Time    // Time of event
	Temp     float32      // Last temp reading
	Humidity float32      // Last humidity reading
	State    ControlState // Active or Not
}
