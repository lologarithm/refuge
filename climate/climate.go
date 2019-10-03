package climate

import (
	"fmt"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/sensor"
)

type Controller interface {
	Heat()
	Cool()
	Fan()
	Off()
	State() refuge.ControlState
}

type RealController struct {
	FanP  rpio.Pin
	CoolP rpio.Pin
	HeatP rpio.Pin

	state refuge.ControlState
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

	rc.state = refuge.StateHeating
}

func (rc *RealController) Cool() {
	rc.HeatP.High()

	rc.FanP.Low()
	rc.CoolP.Low()

	rc.state = refuge.StateCooling
}

func (rc *RealController) Fan() {
	rc.FanP.Low()

	rc.CoolP.High()
	rc.HeatP.High()

	rc.state = refuge.StateFanning
}

func (rc *RealController) Off() {
	rc.FanP.High()
	rc.CoolP.High()
	rc.HeatP.High()

	rc.state = refuge.StateIdle
}

func (rc RealController) State() refuge.ControlState {
	return rc.state
}

// Does nothing. used for running without actually doing anything
type FakeController struct {
}

func (fc FakeController) Heat()                      {}
func (fc FakeController) Cool()                      {}
func (fc FakeController) Fan()                       {}
func (fc FakeController) Off()                       {}
func (fc FakeController) State() refuge.ControlState { return 0 }

// ControlLoop accepts a stream of input to control heating/cooling
func ControlLoop(controller Controller, setStream chan refuge.Settings, thermStream chan sensor.ThermalReading, motionStream chan int64) {
	s := refuge.Settings{}
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
			if set.Mode != refuge.ModeUnset {
				s.Mode = set.Mode
			}
		}
		if s.High == 0 || s.Low == 0 || s.Mode == refuge.ModeUnset {
			// Exit!
			fmt.Printf("No valid high/low temp specified. Control loop exiting.\n")
			return
		}
	}
}

// Control accepts current state and decides what to change
func Control(controller Controller, s refuge.Settings, lastMotion time.Time, tr sensor.ThermalReading) float32 {
	fmt.Printf(" Climate Loop: Temp: %.1f, Hum: %.1f State: %v\n", tr.Temp, tr.Humi, s)
	state := controller.State()

	if s.Mode == refuge.ModeOff {
		if state != refuge.StateIdle {
			fmt.Println("Thermostat was manually disabled.")
			controller.Off()
		}
		return -1
	}

	tempOffset := float32(0)
	if time.Now().Sub(lastMotion) > time.Minute*30 {
		fmt.Printf("Climate Loop: Its been over 30 min since motion was seen, increasing temp range by 2C\n")
		tempOffset = 2
	}
	if state == refuge.StateHeating || state == refuge.StateCooling {
		tempOffset -= 1.5 // We want to go a little over the temp we are targetting.
	}

	if tr.Temp > s.High+tempOffset {
		if state != refuge.StateCooling {
			fmt.Printf("Climate Loop: Activating cooling...\n")
			controller.Cool()
		} else {
			fmt.Printf("Climate Loop: Still cooling...\n")
		}
		return s.High + tempOffset
	} else if tr.Temp < s.Low-tempOffset {
		if state != refuge.StateHeating {
			fmt.Printf("Climate Loop: Activating heating...\n")
			controller.Heat()
		} else {
			fmt.Printf("Climate Loop: still heating...\n")
		}
		return s.Low - tempOffset
	} else if state != refuge.StateIdle {
		if s.Mode == refuge.ModeAuto {
			// First time after reaching goal temp, disable climate control
			fmt.Printf("Climate Loop: Disabling all climate controls...\n")
			controller.Off()
		}
	} else if s.Mode == refuge.ModeFan && state == refuge.StateIdle {
		controller.Fan()
	}

	return 0
}
