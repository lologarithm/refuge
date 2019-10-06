package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
	"gitlab.com/lologarithm/refuge/refuge"
)

func main() {
	cpin := flag.Int("cpin", 24, "input pin to control")
	spin := flag.Int("spin", 4, "input pin to read if portal is open")
	name := flag.String("name", "", "name of portal")
	flag.Parse()

	fmt.Printf("Name: %s, Control Pin: %d, Sensor Pin: %d\n", *name, *cpin, *spin)
	if *name == "" {
		fmt.Printf("Name parameter is required.")
		os.Exit(1)
	}
	run(*name, *cpin, *spin)
}

func run(name string, cpin int, spin int) {
	poll := setupNetwork(name)

	state := refuge.PortalStateUnknown

	err := rpio.Open()
	if err != nil {
		print("Unable to use real pins...\n")
		for {
			newState := poll(state)
			if newState != refuge.PortalStateUnknown && state != newState {
				fmt.Printf("State is now: %d\n", newState)
				state = newState
			}
			time.Sleep(time.Millisecond * 5)
		}
	}
	sensor := rpio.Pin(spin)
	sensor.PullDown()       // Make sure default state is low
	sensor.Mode(rpio.Input) // Now read for state to go high

	// Set switch to off
	control := rpio.Pin(cpin)
	control.Mode(rpio.Output)
	control.High()

	lastRead := time.Now().Unix()
	readDelay := int64(2) // seconds
	for {
		// 1. try network loop
		requested := poll(state)

		// 2. check sensors every "readDelay" seconds
		if spin > 0 && time.Now().Unix()-lastRead > readDelay {
			// Check to see if portal is open
			sr := sensor.Read()
			if sr == rpio.High {
				if state != refuge.PortalStateClosed {
					fmt.Printf("New Door State: Closed\n")
					state = refuge.PortalStateClosed
				}
			} else {
				if state != refuge.PortalStateOpen {
					fmt.Printf("New Door State: Open\n")
					state = refuge.PortalStateOpen
				}
			}
			lastRead = time.Now().Unix()
		}

		// 3. Update state?
		// If v != current state, trigger the garage to open
		if requested != refuge.PortalStateUnknown && requested != state {
			control.Low()
			time.Sleep(time.Millisecond * 100)
			control.High()
			lastRead = time.Now().Unix() - readDelay + 1 // force a re-read in 1 second
		} else {
			time.Sleep(time.Millisecond * 200)
		}
	}
}
