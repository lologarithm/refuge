package main

import (
	"encoding/json"
	"log"
	"net"

	"gitlab.com/lologarithm/refuge/rnet"
)

// This file holds functions to control the various IoT devices via udp messages
func togglePortal(port rnet.Portal) {
	state := rnet.PortalStateOpen
	if port.State == rnet.PortalStateOpen {
		state = rnet.PortalStateClosed
	}
	log.Printf("Toggling Portal (%#v).", port)

	raddr, err := net.ResolveUDPAddr("udp", port.Addr)
	if err != nil {
		log.Fatalf("failed to resolve thermo broadcast address: %s", err)
	}

	msg, _ := json.Marshal(rnet.Portal{Name: port.Name, State: state})
	log.Printf("Sending Portal Update to: '%s' to (%s)'%s'", string(msg), port.Addr, raddr)
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Printf("Failed to open UDP: %s", err)
		return
	}
	conn.Write(msg)
	conn.Close()
}

func toggleSwitch(sw rnet.Switch) {
	raddr, err := net.ResolveUDPAddr("udp", sw.Addr)
	if err != nil {
		log.Fatalf("failed to resolve thermo broadcast address: %s", err)
	}
	msg, _ := json.Marshal(rnet.Switch{Name: sw.Name, On: !sw.On})
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Printf("Failed to open UDP: %s", err)
		return
	}
	conn.Write(msg)
	conn.Close()
}

func setTherm(c ClimateChange, thermo rnet.Thermostat) {
	raddr, err := net.ResolveUDPAddr("udp", thermo.Addr)
	if err != nil {
		log.Fatalf("failed to resolve thermo broadcast address: %s", err)
	}
	msg, _ := json.Marshal(&c.Settings)
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Printf("Failed to open UDP: %s", err)
		return
	}
	conn.Write(msg)
	conn.Close()
}
