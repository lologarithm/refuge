package main

import (
	"encoding/json"
	"log"
	"net"

	"gitlab.com/lologarithm/refuge/rnet"
)

func monitor() (chan rnet.Thermostat, chan rnet.Switch) {
	tstream := make(chan rnet.Thermostat, 10)
	fstream := make(chan rnet.Switch, 10)

	udp, err := net.ListenMulticastUDP("udp", nil, rnet.RefugeMessages)
	if err != nil {
		log.Fatalf("failed to listen to thermo broadcast address: %s", err)
	}
	log.Printf("Now listening to %s for device updates.", rnet.RefugeMessages.String())

	dec := json.NewDecoder(udp)
	go func() {
		for {
			reading := rnet.Msg{}
			log.Printf("Waiting for message...")
			err := dec.Decode(&reading)
			if err != nil {
				log.Printf("Failed to decode json msg: %s", err)
				continue
			}
			if reading.Thermostat != nil {
				log.Printf("New reading: %#v", reading.Thermostat)
				tstream <- *reading.Thermostat
			} else if reading.Switch != nil {
				log.Printf("New Switch: %#v", reading.Switch)
				fstream <- *reading.Switch
				log.Printf("fireplace update sent...")
			} else {
				log.Printf("Unknown update msg: %#v", reading)
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
	n, err := udpConn.WriteToUDP([]byte("{}"), rnet.RefugeDiscovery)
	if n == 0 || err != nil {
		log.Fatalf("Bytes: %d, Err: %s", n, err)
	}
}
