package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	rpio "github.com/stianeikeland/go-rpio"
)

func main() {
	cpin := flag.Int("cpin", 4, "input pin to control")
	name := flag.String("name", "", "name of fireplace")
	flag.Parse()

	fmt.Printf("Name: %s, Control Pin: %d\n", *name, *cpin)
	if *name == "" {
		fmt.Printf("Name parameter is required.")
		os.Exit(1)
	}
	run(*name, *cpin)
}

func run(name string, cpin int) {
	// Listen to network
	stream := runNetwork(name)

	err := rpio.Open()
	if err != nil {
		log.Printf("Unable to use real pins...")
		for v := range stream {
			log.Printf("Setting fake fireplace to: %v", v)
		}
	}
	// Set Fireplace to off
	control := rpio.Pin(cpin)
	control.Mode(rpio.Output)
	control.High()

	// Control the fireplace!
	for v := range stream {
		if v {
			control.Low()
		} else {
			control.High()
		}
	}
}
