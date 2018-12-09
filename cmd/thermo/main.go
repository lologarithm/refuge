package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	rpio "github.com/stianeikeland/go-rpio/v4"
	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/sensor"
)

func main() {
	tpin := flag.Int("tpin", 4, "input pin to read for temp")
	mpin := flag.Int("mpin", 17, "input pin to read for motion")
	hpin := flag.Int("hpin", 24, "output pin to turn on heat")
	cpin := flag.Int("cpin", 22, "output pin to turn on cooling")
	fpin := flag.Int("fpin", 23, "output pin to turn on fan")
	name := flag.String("name", "", "name of thermostat")
	flag.Parse()
	fmt.Printf("Name: %s, Thermo Pin: %d\nHeating Pin: %d\nCooling Pin: %d\nFan Pin: %d\n", *name, *tpin, *hpin, *cpin, *fpin)
	if *name == "" {
		fmt.Printf("Name parameter is required.")
		os.Exit(1)
	}
	// run the thermostat
	run(*name, *tpin, *mpin, *fpin, *cpin, *hpin)

	// Now just hang out until CTRL+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	rpio.Close()
}

// run in short will take sensor readings, emit them on network, and forward them to the climate controller.
// Additionally it will accept new settings from the network and send them into the climate controller.
func run(name string, thermpin, motionpin, fanpin, coolpin, heatpin int) {
	// Incoming streams from sensors
	thermStream := make(chan sensor.ThermalReading, 2) // Stream from the sensor
	motionStream := make(chan int64, 2)                // Stream of last motion events

	// Outgoing streams to climate control system
	controlStream := make(chan sensor.ThermalReading, 2) // Stream to climate control from this function
	cSet := make(chan climate.Settings, 2)               // Stream to send climate settings
	cMot := make(chan int64, 2)                          // Stream to send last motion

	go runNetwork(name, thermStream, motionStream, controlStream, cSet, cMot)

	var cl climate.Controller
	err := rpio.Open()
	if err != nil {
		fmt.Printf("Unable to open raspberry pi gpio pins: %s\n-----  Defaulting to use fake data.  -----\n", err)
		// send fake data!
		go fakeSensors(thermStream)
		cl = climate.FakeController{}
	} else {
		cl = climate.NewController(heatpin, coolpin, fanpin)
		fmt.Printf("Controller: %v\n", cl)
		// Run Sensors
		go sensor.Therm(thermpin, time.Second*30, thermStream)
		go sensor.Motion(motionpin, motionStream)
	}
	// Run Climate control
	go climate.Control(cl, cSet, controlStream, cMot)
}

func fakeSensors(thermStream chan sensor.ThermalReading) {
	for {
		select {
		case thermStream <- sensor.ThermalReading{Temp: 20, Humi: 50, Time: time.Now()}:
		default:
			return // bad, exit
		}
		time.Sleep(time.Second * 30)
	}
}
