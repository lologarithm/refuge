package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	rpio "github.com/stianeikeland/go-rpio"

	"gitlab.com/lologarithm/refuge/rnet"
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

func runNetwork(name string) chan bool {
	stream := make(chan bool, 1)
	addrs := rnet.MyIPs()
	log.Printf("MyAddrs: %#s", addrs)

	addr, err := net.ResolveUDPAddr("udp", addrs[0]+":0")
	if err != nil {
		log.Fatalf("Failed to resolve udp: %s", err)
	}
	// Listen to directed udp messages
	direct, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to listen to udp: %s", err)
	}
	directAddr := direct.LocalAddr()
	log.Printf("Listening on: %s", directAddr)
	dec := json.NewDecoder(direct)

	broadcasts, err := net.ListenMulticastUDP("udp", nil, rnet.RefugeDiscovery)
	if err != nil {
		log.Fatalf("failed to listen to thermo broadcast address: %s", err)
	}
	broadDec := json.NewDecoder(broadcasts)

	state := &rnet.Msg{Fireplace: &rnet.Fireplace{Name: name, Addr: directAddr.String()}}
	msg, _ := json.Marshal(state)
	log.Printf("Broadcasting %s to %s", string(msg), rnet.RefugeMessages.String())
	// Broadcast we are online!
	direct.WriteToUDP(msg, rnet.RefugeMessages)

	go func() {
		for {
			v := rnet.Ping{}
			broadDec.Decode(&v)
			log.Printf("Got message on discovery(%s), updating status: %s", rnet.RefugeDiscovery, string(msg))
			// Broadcast on ping
			direct.WriteToUDP(msg, rnet.RefugeMessages)
		}
	}()

	go func() {
		for {
			v := &rnet.Fireplace{}
			err := dec.Decode(&v)
			if err != nil {
				//lol
			}
			stream <- v.On

			// Write to network our new state
			state.Fireplace.On = v.On

			// Re-marshal and broadcast new state
			msg, _ = json.Marshal(state)
			direct.WriteToUDP(msg, rnet.RefugeMessages)
		}
	}()
	return stream
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
