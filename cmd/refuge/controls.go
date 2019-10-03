package main

import (
	"log"
	"net"

	"github.com/lologarithm/netgen/lib/ngservice"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
)

// This file holds functions to control the various IoT devices via udp messages
func toggleSwitch(newstate int, conn *net.UDPConn, addr *net.UDPAddr) {
	log.Printf("Attempting to send switch toggle: %#v", newstate)
	if conn == nil {
		log.Printf("[Error] No Connection to device.")
		return
	}
	n, err := conn.WriteToUDP(ngservice.WriteMessage(rnet.Context, refuge.Switch{On: newstate == 1}), addr)
	if n == 0 || err != nil {
		log.Printf("[Error] Send failed: %v", err)
	}
}

func togglePortal(newstate int, conn *net.UDPConn, addr *net.UDPAddr) {
	log.Printf("Attempting to send switch toggle: %#v", newstate)
	if conn == nil {
		log.Printf("[Error] No Connection to device.")
		return
	}
	n, err := conn.WriteToUDP(ngservice.WriteMessage(rnet.Context, refuge.Portal{State: refuge.PortalState(newstate)}), addr)
	if n == 0 || err != nil {
		log.Printf("[Error] Send failed: %v", err)
	}
}

func setTherm(c refuge.Settings, conn *net.UDPConn, addr *net.UDPAddr) {
	log.Printf("Attempting to send therm set request: %#v", c)
	if conn == nil {
		log.Printf("[Error] No Connection to device.")
		return
	}
	n, err := conn.WriteToUDP(ngservice.WriteMessage(rnet.Context, c), addr)
	if n == 0 || err != nil {
		log.Printf("[Error] Send failed: %v", err)
	}
}
