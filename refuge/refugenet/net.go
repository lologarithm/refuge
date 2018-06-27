package refugenet

var ThermoSpace = "225.1.2.3:8765"

type Thermostat struct {
	Name     string  // Name of thermostat
	Addr     string  // Address of thermostat
	Target   float32 // Targeted temp
	Temp     float32 // Last temp reading
	Humidity float32 // Last humidity reading
}
