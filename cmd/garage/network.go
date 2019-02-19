package main

import (
	"encoding/json"
	"log"
	"net"
	"sync"

	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
)

func myUDPConn() *net.UDPConn {
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
	return direct
}

func runNetwork(name string, sensorStream chan refuge.PortalState) chan refuge.PortalState {
	stream := make(chan refuge.PortalState, 1) // output stream

	// Open UDP connection to a local addr/port.
	direct := myUDPConn()
	log.Printf("Listening on: %s", direct.LocalAddr().String())
	dec := json.NewDecoder(direct)

	broadcasts, err := net.ListenMulticastUDP("udp", nil, rnet.RefugeDiscovery)
	if err != nil {
		log.Fatalf("failed to listen to thermo broadcast address: %s", err)
	}
	broadDec := json.NewDecoder(broadcasts)

	mut := &sync.Mutex{}
	state := &rnet.Msg{Device: &refuge.Device{Portal: &refuge.Portal{}, Name: name, Addr: direct.LocalAddr().String()}}
	msg, _ := json.Marshal(state)
	log.Printf("Broadcasting %s to %s", string(msg), rnet.RefugeMessages.String())
	// Broadcast we are online!
	direct.WriteToUDP(msg, rnet.RefugeMessages)

	// Ping listener goroutine
	go func() {
		for {
			v := rnet.Ping{}
			broadDec.Decode(&v)
			log.Printf("Got message on discovery(%s), updating status: %s", rnet.RefugeDiscovery, string(msg))
			// Broadcast on ping
			mut.Lock()
			direct.WriteToUDP(msg, rnet.RefugeMessages)
			mut.Unlock()
		}
	}()

	// Sensor broadcast goroutine
	go func() {
		for {
			ns := <-sensorStream
			mut.Lock()
			// Write to network our new state
			state.Portal.State = ns

			// Re-marshal and broadcast new state
			msg, _ = json.Marshal(state)
			log.Printf("Broadcasting: %s", string(msg))
			direct.WriteToUDP(msg, rnet.RefugeMessages)
			mut.Unlock()
		}
	}()

	// Request listener goroutine
	go func() {
		for {
			v := &refuge.Portal{}
			err := dec.Decode(&v)
			if err != nil {
				log.Printf("Failed to decode fireplace setting: %s", err)
				continue
			}
			log.Printf("Setting door to: %#v", v)
			stream <- refuge.PortalState(v.State)
		}
	}()
	return stream
}
