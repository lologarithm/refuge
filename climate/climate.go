package climate

import (
	"fmt"

	rpio "github.com/stianeikeland/go-rpio"
	"gitlab.com/lologarithm/thermo/sensor"
)

type Mode byte

const (
	OffMode  Mode = iota // Disable
	AutoMode             // Manage temp range
	FanMode              // Just run fan
)

type Settings struct {
	Low  float32 // low temp in C
	High float32 // high temp in C
	Mode Mode
	Pins Pins
}

type Pins struct {
	Fan  int
	Heat int
	Cool int
}

// Control accepts a stream of input and returns a function to set the target state.
func Control(s Settings, stream chan sensor.Measurement) func(s Settings) {
	// if s.

	fan := rpio.Pin(s.Pins.Fan)
	fan.Mode(rpio.Output)
	cool := rpio.Pin(s.Pins.Cool)
	cool.Mode(rpio.Output)
	heat := rpio.Pin(s.Pins.Heat)
	heat.Mode(rpio.Output)

	go func() {
		// Run the climate control system here.
		for {
			v := <-stream
			fmt.Printf("Temp: %.1f, State: %v\n", v.Temp, s)
			if v.Temp > s.High && cool > 0 {
				fmt.Printf("Activating cooling...\n")
				// Activate cooling
				fan.High()
				cool.High()
				heat.Low()
			} else if v.Temp < s.Low && heat > 0 {
				fmt.Printf("Activating heating...\n")
				// Activate heating
				fan.High()
				cool.Low()
				heat.High()
			} else {
				fmt.Printf("Disabling all climate controls...\n")
				fan.Low()
				cool.Low()
				heat.Low()
			}
		}
	}()

	return func(ns Settings) {
		// Update pins if needed
		if ns.Pins.Cool != int(cool) {
			if int(cool) != 0 {
				cool.Low()
			}
			cool = rpio.Pin(ns.Pins.Cool)
			cool.Mode(rpio.Output)
		}
		if ns.Pins.Heat != int(heat) {
			if int(heat) != 0 {
				heat.Low()
			}
			heat = rpio.Pin(ns.Pins.Heat)
			heat.Mode(rpio.Output)
		}
		if ns.Pins.Fan != int(fan) {
			if int(fan) != 0 {
				fan.Low()
			}
			fan = rpio.Pin(ns.Pins.Fan)
			fan.Mode(rpio.Output)
		}
		// Update the current state.
		s = ns
	}
}
