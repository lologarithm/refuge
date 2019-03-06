package climate

import (
	"fmt"
	"time"

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
	State() ControlState
}

type RealController struct {
	FanP  rpio.Pin
	CoolP rpio.Pin
	HeatP rpio.Pin

	state ControlState
}

func NewController(h, c, f int) *RealController {
	fan := rpio.Pin(f)
	fan.Mode(rpio.Output)
	fan.High()
	cool := rpio.Pin(c)
	cool.Mode(rpio.Output)
	cool.High()
	heat := rpio.Pin(h)
	heat.Mode(rpio.Output)
	heat.High()

	return &RealController{
		FanP:  fan,
		CoolP: cool,
		HeatP: heat,
	}
}

func (rc *RealController) Heat() {
	rc.CoolP.High()

	rc.FanP.Low()
	rc.HeatP.Low()

	rc.state = StateHeating
}

func (rc *RealController) Cool() {
	rc.HeatP.High()

	rc.FanP.Low()
	rc.CoolP.Low()

	rc.state = StateCooling
}

func (rc *RealController) Fan() {
	rc.FanP.Low()

	rc.CoolP.High()
	rc.HeatP.High()

	rc.state = StateFanning
}

func (rc *RealController) Off() {
	rc.FanP.High()
	rc.CoolP.High()
	rc.HeatP.High()

	rc.state = StateIdle
}

func (rc RealController) State() ControlState {
	return rc.state
}

// Does nothing. used for running without actually doing anything
type FakeController struct {
}

func (fc FakeController) Heat()               {}
func (fc FakeController) Cool()               {}
func (fc FakeController) Fan()                {}
func (fc FakeController) Off()                {}
func (fc FakeController) State() ControlState { return 0 }

type ControlState byte

const (
	StateIdle ControlState = iota
	StateCooling
	StateFanning
	StateHeating
)

// ControlLoop accepts a stream of input to control heating/cooling
func ControlLoop(controller Controller, setStream chan Settings, thermStream chan sensor.ThermalReading, motionStream chan int64) {
	s := Settings{}
	// Run the climate control system here.
	lastTherm := <-thermStream
	lastMotion := time.Now()
	fmt.Printf("Starting control loop now...\n")

	for {
		select {
		case v := <-thermStream:
			lastTherm = v
			Control(controller, s, lastMotion, lastTherm)
		case t := <-motionStream:
			lastMotion = time.Unix(t, 0)
			Control(controller, s, lastMotion, lastTherm)
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
func Control(controller Controller, s Settings, lastMotion time.Time, tr sensor.ThermalReading) float32 {
	fmt.Printf("Climate Loop: Temp: %.1f, Hum: %.1f State: %v\n", tr.Temp, tr.Humi, s)
	tempOffset := float32(0)

	if time.Now().Sub(lastMotion) > time.Minute*30 {
		fmt.Printf("Climate Loop: Its been over 30 min since motion was seen, increasing temp range by 2C\n")
		tempOffset = 2
	}
	state := controller.State()
	if state == StateHeating || state == StateCooling {
		tempOffset -= 1.5 // We want to go a little over the temp we are targetting.
	}
	if tr.Temp > s.High+tempOffset {
		if state != StateCooling {
			fmt.Printf("Climate Loop: Activating cooling...\n")
			controller.Cool()
		} else {
			fmt.Printf("Climate Loop: Still cooling...\n")
		}
		return s.High + tempOffset
	} else if tr.Temp < s.Low-tempOffset {
		if state != StateHeating {
			fmt.Printf("Climate Loop: Activating heating...\n")
			controller.Heat()
		} else {
			fmt.Printf("Climate Loop: still heating...\n")
		}
		return s.Low - tempOffset
	} else if state != StateIdle {
		// First time after reaching goal temp, disable climate control
		fmt.Printf("Climate Loop: Disabling all climate controls...\n")
		controller.Off()
	}

	return 0
}
