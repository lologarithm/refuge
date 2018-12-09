package climate

import (
	"fmt"

	rpio "github.com/stianeikeland/go-rpio/v4"
	"gitlab.com/lologarithm/refuge/sensor"
)

type Mode byte

const (
	ModeUnset Mode = iota
	ModeOff
	ModeAuto // Manage temp range
	ModeFan  // Just run fan
)

type Settings struct {
	Low  float32 // low temp in C
	High float32 // high temp in C
	Mode Mode
}

type Controller interface {
	Heat()
	Cool()
	Fan()
	Off()
}

type RealController struct {
	FanP  rpio.Pin
	CoolP rpio.Pin
	HeatP rpio.Pin
}

func NewController(h, c, f int) RealController {
	fan := rpio.Pin(f)
	fan.Mode(rpio.Output)
	fan.High()
	cool := rpio.Pin(c)
	cool.Mode(rpio.Output)
	cool.High()
	heat := rpio.Pin(h)
	heat.Mode(rpio.Output)
	heat.High()

	return RealController{
		FanP:  fan,
		CoolP: cool,
		HeatP: heat,
	}
}

func (rc RealController) Heat() {
	rc.CoolP.High()

	rc.FanP.Low()
	rc.HeatP.Low()
}

func (rc RealController) Cool() {
	rc.HeatP.High()

	rc.FanP.Low()
	rc.CoolP.Low()
}

func (rc RealController) Fan() {
	rc.FanP.Low()

	rc.CoolP.High()
	rc.HeatP.High()
}

func (rc RealController) Off() {
	rc.FanP.High()
	rc.CoolP.High()
	rc.HeatP.High()
}

// Does nothing. used for running without actually doing anything
type FakeController struct {
}

func (fc FakeController) Heat() {}
func (fc FakeController) Cool() {}
func (fc FakeController) Fan()  {}
func (fc FakeController) Off()  {}

type controlState byte

const (
	stateIdle controlState = iota
	stateCooling
	stateHeating
)

// Control accepts a stream of input and returns a function to set the target state.
func Control(controller Controller, s Settings, stream chan sensor.Measurement) func(s Settings) {
	go func() {
		// Run the climate control system here.
		state := stateIdle
		for {
			v := <-stream
			fmt.Printf("Temp: %.1f, State: %v\n", v.Temp, s)
			// if state == stateIdle || true { // default to override for now.
			if v.Temp > s.High && state != stateCooling {
				fmt.Printf("Activating cooling...\n")
				state = stateCooling
				controller.Cool()
			} else if v.Temp < s.Low && state != stateHeating {
				fmt.Printf("Activating heating...\n")
				state = stateHeating
				controller.Heat()
			} else {
				fmt.Printf("Disabling all climate controls...\n")
				state = stateIdle
				controller.Off()
			}
			// }
			if s.High == 0 || s.Low == 0 {
				// Exit!
				fmt.Printf("No valid high/low temp specified. Control loop exiting.\n")
				return
			}
		}
	}()

	return func(ns Settings) {
		// Update the current state.
		s = ns
	}
}
