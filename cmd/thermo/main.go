package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	rpio "github.com/stianeikeland/go-rpio"
	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/sensor"
)

func main() {
	tpin := flag.Int("tpin", 4, "input pin to read for temp")
	mpin := flag.Int("mpin", 0, "input pin to read for motion")
	hpin := flag.Int("hpin", 24, "output pin to turn on heat")
	cpin := flag.Int("cpin", 22, "output pin to turn on cooling")
	fpin := flag.Int("fpin", 23, "output pin to turn on fan")
	name := flag.String("name", "", "name of thermostat")
	flag.Parse()
	fmt.Printf("Name: %s\n\tThermo Pin: %d\n\tHeating Pin: %d\n\tCooling Pin: %d\n\tFan Pin: %d\n", *name, *tpin, *hpin, *cpin, *fpin)
	if *name == "" {
		fmt.Printf("Name parameter is required.")
		os.Exit(1)
	}
	// run the thermostat
	run(*name, *tpin, *mpin, *fpin, *cpin, *hpin)

	rpio.Close()
}

// run in short will take sensor readings, emit them on network, and forward them to the climate controller.
// Additionally it will accept new settings from the network and send them into the climate controller.
func run(name string, thermpin, motionpin, fanpin, coolpin, heatpin int) {
	// Now just hang out until CTRL+C
	close := make(chan os.Signal, 1)
	signal.Notify(close, os.Interrupt)

	var cl climate.Controller
	err := rpio.Open()
	if err != nil {
		cl = &climate.FakeController{}
		fmt.Printf("Unable to open raspberry pi gpio pins: %s\n-----  Defaulting to use fake data.  -----\n", err)
		getMot := func() bool { return true }
		getTherm := func(includeWait bool) (float32, float32, bool) { return 20, 20, true }
		go runThermostat(name, cl, close, getTherm, getMot)
		return
	}

	cl = climate.NewController(heatpin, coolpin, fanpin)
	var getMot func() bool
	if motionpin != 0 {
		mp := rpio.Pin(motionpin)
		mp.PullDown()
		mp.Mode(rpio.Input)
		getMot = func() bool { return sensor.ReadMotion(mp) }
	} else {
		print("No motion sensor attached. Defaulting to always have motion 'on'.\n")
	}
	tp := rpio.Pin(thermpin)
	// This closure just abstracts the need for knowing the pin to read. The controller logic only cares
	// about returning the values without having to worry about how it got it.
	getTherm := func(includeWait bool) (float32, float32, bool) { return sensor.ReadDHT22(tp, includeWait) }
	runThermostat(name, cl, close, getTherm, getMot)
}
