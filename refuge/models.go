package refuge

import (
	"gitlab.com/lologarithm/refuge/climate"
)

// Portal represents any door/window that can be monitored or open/closed
type Portal struct {
	State PortalState // Can signal current state or intended state. Unknown, Closed, Open
}

// PortalState is the state of the portal (open/closed)
type PortalState uint64

// Enum of portal states
const (
	PortalStateUnknown PortalState = iota
	PortalStateClosed
	PortalStateOpen
)

// Thermostat is a device that controls temp by setting acceptable temp ranges.
// Technically doesn't work without a Thermometer but they are separate devices
// so that other things can have a thermometer without a thermostat.
type Thermostat struct {
	State    climate.ControlState // Active or Not
	Target   float32              // Target for heating/cooling
	Settings climate.Settings
}

// Thermometer is a thermometer reading.
type Thermometer struct {
	Temp     float32 // Last temp reading
	Humidity float32 // Last humidity reading
}

// Motion is a motion sensor reading
type Motion struct {
	Motion int64 // Last motion event
}

// Switch represents any devices that can be switched on/off
// Examples: Lights, Gas Fireplace, etc
type Switch struct {
	On bool
}

// Device represents a single device in the network.
// It can have many physical sensors and controls.
type Device struct {
	Name string
	Addr string

	// List of things that the device could have
	Switch      *Switch
	Thermostat  *Thermostat
	Thermometer *Thermometer
	Portal      *Portal
	Motion      *Motion
}
