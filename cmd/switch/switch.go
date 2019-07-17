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
	cpin := flag.Int("cpin", 4, "input pin to control")
	name := flag.String("name", "", "name of device to switch")
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
	poll := setupNetwork(name)

	err := rpio.Open()
	if err != nil {
		log.Printf("Unable to use real pins...")
		for {
			v := poll()
			if v > 0 {
				log.Printf("Setting fake switch to: %v", v)
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
	// Set switch to off
	control := rpio.Pin(cpin)
	control.Mode(rpio.Output)
	control.High()

	for {
		v := poll()
		if v == 1 {
			control.Low()
		} else if v == 2 {
			control.High()
		}
		time.Sleep(time.Millisecond * 200)
	}
}
