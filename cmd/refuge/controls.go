package main

import (
	"encoding/json"
	"log"
	"net"

	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/refuge"
)

// This file holds functions to control the various IoT devices via udp messages
func toggleSwitch(newstate int, conn *net.UDPConn) {
	msg, err := json.Marshal(refuge.Switch{On: newstate == 1})
	if err != nil {
		log.Printf("[Error] Failed to write json switch update: %s", err)
		return
	}
	if conn == nil {
		log.Printf("[Error] No Connection to device.")
		return
	}
	log.Printf("Sending Switch Request: %s", string(msg))
	conn.Write(msg)
}

func togglePortal(newstate int, conn *net.UDPConn) {
	msg, err := json.Marshal(refuge.Portal{State: refuge.PortalState(newstate)})
	if err != nil {
		log.Printf("[Error] Failed to write json portal update: %s", err)
		return
	}
	if conn == nil {
		log.Printf("[Error] No Connection to device.")
		return
	}
	log.Printf("Sending Portal Request: %s", string(msg))
	conn.Write(msg)
}

func setTherm(c climate.Settings, conn *net.UDPConn) {
	msg, err := json.Marshal(c)
	if err != nil {
		log.Printf("[Error] Failed to write json climate update: %s", err)
		return
	}
	log.Printf("Sending Therm Set Request: %s", string(msg))
	if conn == nil {
		log.Printf("[Error] No Connection to device.")
		return
	}
	conn.Write(msg)
}
