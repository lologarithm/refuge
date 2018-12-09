package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/rnet"
	"gitlab.com/lologarithm/refuge/sensor"
)

func runNetwork(name string, thermStream chan sensor.ThermalReading, motionStream chan int64, controlStream chan sensor.ThermalReading, cSet chan climate.Settings, cMot chan int64) {
	cSet <- climate.Settings{
		Low:  15.55,
		High: 26.66,
		Mode: climate.ModeAuto,
	} // Shove in first desired state

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

	broadcasts, err := net.ListenMulticastUDP("udp", nil, rnet.RefugeDiscovery)
	if err != nil {
		log.Fatalf("failed to listen to thermo broadcast address: %s", err)
	}
	broadDec := json.NewDecoder(broadcasts)

	dec := json.NewDecoder(direct)

	ts := rnet.Msg{Thermostat: &rnet.Thermostat{
		// Device Config
		Name: name,
		Addr: directAddr.String(),

		// Default settings on launch
		Fan:  uint8(climate.ModeAuto),
		Low:  15.55,
		High: 26.66,

		// No readings yet
		Temp:     0,
		Humidity: 0,
		Motion:   0,
	}}

	msg, merr := json.Marshal(ts)
	if merr != nil {
		fmt.Printf("Failed to marshal climate reading: %s", merr)
		return
	}

	// Reads thermal readings, forwards to the climate controller
	// and copies to the network for the web interface to see.
	for {
		// Look for broadcasts
		if broadDec.More() {
			v := rnet.Ping{}
			broadDec.Decode(&v)
			log.Printf("Got message on discovery(%s)", rnet.RefugeDiscovery)
			// Broadcast on ping
			direct.WriteToUDP(msg, rnet.RefugeMessages)
		}

		// Try to read from network.
		if dec.More() {
			v := climate.Settings{}
			derr := dec.Decode(&v)
			if derr != nil {
				fmt.Printf("Failed to decode climate setting request: %s", derr)
				continue
			}
			fmt.Printf("Climate set attempt: %#v", v)
			ts.Thermostat.High = v.High
			ts.Thermostat.Low = v.Low
			ts.Thermostat.Fan = uint8(v.Mode)

			cSet <- v // copy to the climate controller
		}

		// Check for any new sensor readings, otherwise, wait a second and try the whole loop again.
		select {
		case thReading := <-thermStream:
			ts.Thermostat.Temp = thReading.Temp
			ts.Thermostat.Humidity = thReading.Humi
			controlStream <- thReading
			fmt.Printf("Climate reading: %#v\n", ts)
			msg, merr = json.Marshal(ts)
		case motionTime := <-motionStream:
			ts.Thermostat.Motion = motionTime
			cMot <- motionTime
			msg, merr = json.Marshal(ts)
		default:
			// Sleep for a bit, and then try again later.
			time.Sleep(time.Second)
			continue
		}

		if merr != nil {
			fmt.Printf("Failed to marshal climate reading: %s", merr)
			continue
		}

		// This means we got something new to send to the network
		direct.WriteToUDP(msg, rnet.RefugeMessages)
	}
}
