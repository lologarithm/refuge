package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
	"gitlab.com/lologarithm/thermo/climate"
	"gitlab.com/lologarithm/thermo/refuge/refugenet"
	"gitlab.com/lologarithm/thermo/sensor"
)

func main() {
	tpin := flag.Int("tpin", 4, "input pin to read for temp")
	hpin := flag.Int("hpin", 24, "output pin to turn on heat")
	cpin := flag.Int("cpin", 22, "output pin to turn on cooling")
	fpin := flag.Int("fpin", 23, "output pin to turn on fan")
	flag.Parse()
	fmt.Printf("Thermo Pin: %d\nHeating Pin: %d\nCooling Pin: %d\nFan Pin: %d\n", *tpin, *hpin, *cpin, *fpin)
	run(*tpin, *fpin, *cpin, *hpin)
}

func run(tpin, fanpin, coolpin, heatpin int) {
	stream := make(chan sensor.Measurement, 10)
	climateStream := make(chan sensor.Measurement, 10)
	set := func(_ climate.Settings) {}
	cs := climate.Settings{
		Low:  15.55,
		High: 26.66,
		Mode: climate.AutoMode,
	}

	addr, err := net.ResolveUDPAddr("udp", refugenet.ThermoSpace)
	if err != nil {
		log.Fatalf("Failed to resolve udp: %s", err)
	}
	// Listen to directed udp messages
	direct, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to listen to udp: %s", err)
	}

	enc := json.NewEncoder(direct)
	dec := json.NewDecoder(direct)

	go func() {
		for d := range stream {
			climateStream <- d
			enc.Encode(d)
		}
	}()

	go func() {
		for {
			v := climate.Settings{}
			err := dec.Decode(&v)
			if err != nil {
				//lol
			}
			set(v)
		}
	}()

	err = rpio.Open()
	if err != nil {
		fmt.Printf("Unable to open raspberry pi gpio pins: %s\n-----  Defaulting to use fake data.  -----\n", err)
		// send fake data!
		go func() {
			for {
				select {
				case stream <- sensor.Measurement{Temp: 20, Humi: 50, Time: time.Now()}:
				default:
					return // bad, exit
				}
				time.Sleep(time.Second * 30)
			}
		}()
		set = climate.Control(climate.FakeController{}, cs, climateStream)
	} else {
		controller := climate.NewController(heatpin, coolpin, fanpin)
		fmt.Printf("Controller: %v\n", controller)
		set = climate.Control(controller, cs, climateStream)
		sensor.Stream(tpin, time.Second*30, stream)
	}
}
