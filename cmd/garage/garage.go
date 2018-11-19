package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
)

func main() {
	cpin := flag.Int("cpin", 11, "input pin to control")
	spin := flag.Int("cpin", 17, "input pin to read if portal is open")
	name := flag.String("name", "", "name of device to switch")
	flag.Parse()

	fmt.Printf("Name: %s, Control Pin: %d, Sensor Pin: %d\n", *name, *cpin, *spin)
	if *name == "" {
		fmt.Printf("Name parameter is required.")
		os.Exit(1)
	}
	run(*name, *cpin, *spin)
}

// PortalState is the state of the portal (open/closed)
type PortalState uint8

// Enum of portal states
const (
	PortalStateUnknown PortalState = iota
	PortalStateClosed
	PortalStateOpen
)

func run(name string, cpin int, spin int) {
	// Listen to network
	stream := runNetwork(name)

	err := rpio.Open()
	if err != nil {
		log.Printf("Unable to use real pins...")
		for v := range stream {
			log.Printf("Setting fake switch to: %v", v)
		}
	}

	// Set switch to off
	control := rpio.Pin(cpin)
	control.Mode(rpio.Output)
	control.Low()

	state := PortalStateUnknown

	if spin > 0 {
		sensor := rpio.Pin(spin)
		sensor.Mode(rpio.Input)

		go func() {
			for {
				// Check to see if
				if sensor.Read() == rpio.High {
					state = PortalStateClosed
				}
				time.Sleep(time.Millisecond * 100)
			}
		}()
	}

	// Control the portal!
	for v := range stream {
		// If v != current state, toggle the control
		// Hopefully this is how garage door openers work
		if v != state {
			control.High()
			time.Sleep(time.Millisecond * 20)
			control.Low()
		}
	}

}
