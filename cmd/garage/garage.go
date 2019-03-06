package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync/atomic"
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
	// Listen to network
	stateStream := make(chan refuge.PortalState, 1)
	stream := runNetwork(name, stateStream)

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
	control.High()

	state := uint32(refuge.PortalStateUnknown)

	if spin > 0 {
		sensor := rpio.Pin(spin)
		sensor.PullDown()       // Make sure default state is low
		sensor.Mode(rpio.Input) // Now read for state to go high

		// Sensor listener stream
		go func() {
			for {
				// Check to see if portal is open
				sr := sensor.Read()
				if sr == rpio.High {
					if refuge.PortalState(atomic.LoadUint32(&state)) != refuge.PortalStateClosed {
						fmt.Printf("Door Closed. Updating network.\n")
						atomic.StoreUint32(&state, uint32(refuge.PortalStateClosed))
						stateStream <- refuge.PortalStateClosed
					}
				} else {
					if refuge.PortalState(atomic.LoadUint32(&state)) != refuge.PortalStateOpen {
						fmt.Printf("Door Opened. Updating network.\n")
						atomic.StoreUint32(&state, uint32(refuge.PortalStateOpen))
						stateStream <- refuge.PortalStateOpen
					}
				}
				time.Sleep(time.Second)
			}
		}()
	}

	// Control the portal!
	for v := range stream {
		// If v != current state, trigger the garage to open
		if v != refuge.PortalState(atomic.LoadUint32(&state)) {
			control.Low()
			time.Sleep(time.Millisecond * 100)
			control.High()
		}
	}
}
