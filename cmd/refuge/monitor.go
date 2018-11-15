package main

import (
	"encoding/json"
	"log"
	"net"

	"gitlab.com/lologarithm/refuge/rnet"
)

func monitor() (chan rnet.Thermostat, chan rnet.Fireplace) {
	tstream := make(chan rnet.Thermostat, 10)
	fstream := make(chan rnet.Fireplace, 10)
	udp, err := net.ListenMulticastUDP("udp", nil, rnet.RefugeMessages)
	if err != nil {
		log.Fatalf("failed to listen to thermo broadcast address: %s", err)
	}
	log.Printf("Now listening to %s for device updates.", rnet.RefugeMessages.String())
	dec := json.NewDecoder(udp)
	go func() {
		for {
			reading := rnet.Msg{}
			err := dec.Decode(&reading)
			if err != nil {
				log.Printf("Failed to decode json msg: %s", err)
				// lol
			}
			if reading.Thermostat != nil {
				log.Printf("New reading: %#v", reading.Thermostat)
				tstream <- *reading.Thermostat
			} else if reading.Fireplace != nil {
				log.Printf("New fireplace: %#v", reading.Fireplace)
				fstream <- *reading.Fireplace
			}
		}
	}()
	ping()
	return tstream, fstream
}

func ping() {
	// Ping network to find stuff.
	local, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		log.Fatalf("Failed to request a ping from discovery network: %s", err)
	}
	udpConn, err := net.ListenUDP("udp", local)
	if err != nil {
		log.Fatalf("Failed to listen to udp socket: %s", err)
	}
	udpConn.WriteToUDP([]byte("{}"), rnet.RefugeDiscovery)
}
