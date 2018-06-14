package climate

import "gitlab.com/lologarithm/thermo/sensor"

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
}

// Control accepts a stream of input and returns a function to set the target state.
func Control(stream chan sensor.Measurement) func(s Settings) {
	// Pretend its 70, we want to stay between 60 and 80. Auto run.
	s := &Settings{Low: 60, High: 80, Mode: AutoMode}
	go func() {
		// Run the climate control system here.
		for {
			v := <-stream
			if v.Temp > s.High {
				// Activate cooling
			}
			if v.Temp < s.Low {
				// Activate heating
			}
		}
	}()

	return func(ns Settings) {
		// Update the current state.
		s = &ns
	}
}
