package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	rpio "github.com/stianeikeland/go-rpio/v4"
	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/rnet"
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

	cs := climate.Settings{
		Low:  15.55,
		High: 26.66,
		Mode: climate.ModeAuto,
	}
	cSet <- cs // Shove in first desired state

	addrs := rnet.MyIPs()
	log.Printf("MyAddrs: %#v", addrs)

	addr, err := net.ResolveUDPAddr("udp", addrs[0]+":0")
	if err != nil {
		log.Fatalf("Failed to resolve udp: %s", err)
	}
	// Listen to directed udp messages
	direct, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to listen to udp: %s", err)
	}
	log.Printf("Listening on: %s", direct.LocalAddr())

	directAddr := direct.LocalAddr()

	// enc := json.NewEncoder(direct)
	dec := json.NewDecoder(direct)

	go func() {
		// Reads thermal readings, forwards to the climate controller
		// and copies to the network for the web interface to see.
		ts := rnet.Msg{Thermostat: &rnet.Thermostat{
			Name:     name,
			Addr:     directAddr.String(),
			Fan:      uint8(cs.Mode),
			High:     cs.High,
			Low:      cs.Low,
			Temp:     0,
			Humidity: 0,
			Motion:   0,
		}}
		for {
			select {
			case thReading := <-thermStream:
				ts.Thermostat.Temp = thReading.Temp
				ts.Thermostat.Humidity = thReading.Humi
				controlStream <- thReading
				fmt.Printf("Climate reading: %#v\n", ts)
			case motionTime := <-motionStream:
				ts.Thermostat.Motion = motionTime
				cMot <- motionTime
			}
			msg, merr := json.Marshal(ts)
			if merr != nil {
				fmt.Printf("Failed to marshal climate reading: %s", merr)
				continue
			}
			direct.WriteToUDP(msg, rnet.RefugeMessages)
		}
	}()

	go func() {
		for {
			v := climate.Settings{}
			derr := dec.Decode(&v)
			if derr != nil {
				fmt.Printf("Failed to decode climate setting request: %s", derr)
				continue
			}
			fmt.Printf("Climate set attempt: %#v", v)
			cSet <- v
		}
	}()

	var cl climate.Controller
	err = rpio.Open()
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
