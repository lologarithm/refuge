package climate

import (
	"fmt"

	rpio "github.com/stianeikeland/go-rpio"
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

type ControlState byte

const (
	StateIdle ControlState = iota
	StateCooling
	StateHeating
)

// ControlLoop accepts a stream of input to control heating/cooling
func ControlLoop(controller Controller, setStream chan Settings, thermStream chan sensor.ThermalReading, motionStream chan int64) {
	s := Settings{}
	// Run the climate control system here.
	state := StateIdle
	// lastMotion := time.Now()
	fmt.Printf("Starting control loop now...\n")

	for {
		select {
		case v := <-thermStream:
			state = Control(controller, state, s, v)
		case <-motionStream:
			// lastMotion = time.Unix(t, 0)
		case set := <-setStream:
			fmt.Printf("Climate Loop: changing settings: %#v\n", set)
			s.High = set.High
			s.Low = set.Low
			if set.Mode != ModeUnset {
				s.Mode = set.Mode
			}
		}
		if s.High == 0 || s.Low == 0 || s.Mode == ModeUnset {
			// Exit!
			fmt.Printf("No valid high/low temp specified. Control loop exiting.\n")
			return
		}
	}
}

// Control accepts current state and decides what to change
func Control(controller Controller, state ControlState, s Settings, tr sensor.ThermalReading) ControlState {
	fmt.Printf("Climate Loop: Temp: %.1f, State: %v\n", tr.Temp, s)
	// if time.Now().Sub(lastMotion) > time.Hour {
	// 	fmt.Printf("Climate Loop: Its been over an hour since motion was seen, increasing temp range by 2C\n")
	// 	tempOffset = 2
	// }
	tempOffset := float32(0)

	if state == StateHeating || state == StateCooling {
		tempOffset -= 1.5 // We want to go a little over the temp we are targetting.
	}
	if tr.Temp > s.High+tempOffset && state != StateCooling {
		fmt.Printf("Climate Loop: Activating cooling...\n")
		controller.Cool()
		return StateCooling
	} else if tr.Temp < s.Low-tempOffset {
		if state != StateHeating {
			fmt.Printf("Climate Loop: Activating heating...\n")
			controller.Heat()
		} else {
			fmt.Printf("Climate Loop: still heating...\n")
		}
		return StateHeating
	} else if state == StateIdle {
		return StateIdle
	}

	fmt.Printf("Climate Loop: Disabling all climate controls...\n")
	controller.Off()
	return StateIdle
}
