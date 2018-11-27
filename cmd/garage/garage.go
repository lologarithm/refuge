package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"sync/atomic"

	"gitlab.com/lologarithm/refuge/rnet"

	rpio "github.com/stianeikeland/go-rpio"
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

	state := uint64(rnet.PortalStateUnknown)

	if spin > 0 {
		sensor := rpio.Pin(spin)
		sensor.PullDown() // Make sure default state is low
		sensor.Mode(rpio.Input) // Now read for state to go high

		go func() {
			for {
				// Check to see if
				if sensor.Read() == rpio.High {
					atomic.StoreUint64(&state, uint64(rnet.PortalStateClosed))
				}
				time.Sleep(time.Second)
			}
		}()
	}

	// Control the portal!
	for v := range stream {
		// If v != current state, toggle the control
		// Hopefully this is how garage door openers work
		if v != rnet.PortalState(atomic.LoadUint64(&state)) {
			control.Low()
			time.Sleep(time.Millisecond * 100)
			control.High()
		}
	}

}
