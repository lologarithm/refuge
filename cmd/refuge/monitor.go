package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/lologarithm/netgen/lib/ngservice"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
)

// networkMonitor monitors for network messages and decodes/passes them along to the main processor
// the message channel returned by the function is the stream of messages decoded from the network.
func networkMonitor(test bool) (chan rnet.Msg, *net.UDPConn) {
	if test {
		return fakeMonitor(), nil
	}
	tstream := make(chan rnet.Msg, 10)

	local, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		log.Printf("[Error] Failed to request a ping from discovery network: %s", err)
	}
	udpConn, err := net.ListenUDP("udp", local)
	if err != nil {
		log.Printf("[Error] Failed to listen to udp socket: %s", err)
	}

	broadcasts, err := net.ListenMulticastUDP("udp", nil, rnet.RefugeDiscovery)
	if err != nil {
		fmt.Printf("failed to listen to thermo broadcast address: %s\n", err)
		os.Exit(1)
	}

	go func() {
		// This will have us listen for non-respond ping messages.
		// These will come from devices that first came online
		// We will ping them directly so they know to update the main server.
		broadBuf := make([]byte, 2048)
		for {
			n, remoteAddr, _ := broadcasts.ReadFromUDP(broadBuf)
			if n > 0 {
				packet, ok := ngservice.ReadPacket(rnet.Context, broadBuf[:n])
				if ok && packet.Header.MsgType == rnet.PingMsgType {
					if !(packet.NetMsg.(*rnet.Ping)).Respond {
						udpConn.WriteToUDP(pingmsg, remoteAddr)
					}
				}
			}
		}
	}()

	go readNetwork(udpConn, tstream)

	ping(udpConn) // send a ping out to network to find all devices available right now.

	return tstream, udpConn
}

func readNetwork(udpConn *net.UDPConn, tstream chan rnet.Msg) {
	buf := make([]byte, 2048)
	var reading *rnet.Msg
	for {
		n, _, _ := udpConn.ReadFromUDP(buf)
		if n > 0 {
			packet, ok := ngservice.ReadPacket(rnet.Context, buf[:n])
			if ok && packet.Header.MsgType == rnet.MsgMsgType {
				reading = packet.NetMsg.(*rnet.Msg)
			} else {
				log.Printf("Failed to read network message... %v", buf[:n])
				continue
			}
		}
		if reading == nil || reading.Device == nil {
			log.Printf("Device is nil")
			continue
		}
		switch {
		case reading.Thermostat != nil:
			log.Printf("New reading (%s, %s): %#v", reading.Device.Name, reading.Device.Addr, reading.Thermostat)
		case reading.Switch != nil:
			log.Printf("New Switch: %#v", reading.Switch)
		case reading.Portal != nil:
			log.Printf("Portal Update: %#v", reading.Portal)
		default:
			log.Printf("Unknown message: %#v", reading)
			continue
		}
		tstream <- *reading
	}
}

var pingmsg = ngservice.WriteMessage(rnet.Context, &rnet.Ping{Respond: true})

func ping(udpConn *net.UDPConn) {
	// Ping network to find stuff.
	n, err := udpConn.WriteToUDP(pingmsg, rnet.RefugeDiscovery)
	if n == 0 || err != nil {
		log.Printf("[Error] Failed to write to UDP! Bytes: %d, Err: %s", n, err)
	}
}

type DeviceState struct {
	refuge.Device
	lastPing   time.Time
	lastUpdate time.Time
	lastOpened time.Time
	lastEmail  time.Time
}

const openAlertTime = time.Minute * 30
const upAlertTime = time.Minute * 15

func portalAlert(c *Config, deviceUpdates chan refuge.Device, udpConn *net.UDPConn) {
	// Portal watcher
	devices := map[string]*DeviceState{}
	for {
		select {
		case up, ok := <-deviceUpdates:
			if !ok {
				return
			}
			existing, ok := devices[up.Name]
			if !ok {
				existing = &DeviceState{Device: up}
				devices[up.Name] = existing
			}
			if port := existing.Portal; port != nil {
				if port.State != refuge.PortalStateOpen && up.Portal.State == refuge.PortalStateOpen {
					// If just opened, set the time.
					log.Printf("Portal %s is open... starting timer for alert.", up.Name)
					existing.lastOpened = time.Now()
				} else if up.Portal.State != refuge.PortalStateOpen {
					// if not open now, keep updating.
					existing.lastOpened = time.Now()
				}
				existing.Portal = up.Portal
			} else {
				existing.Device = up
			}
			log.Printf("Got update (%s)", up.Name)
			existing.lastUpdate = time.Now()
		case <-time.After(time.Minute * 5):
			break
		}

		for _, p := range devices {
			upDiff := time.Now().Sub(p.lastUpdate)
			emailDiff := time.Now().Sub(p.lastEmail)

			if upDiff > time.Minute*5 { // if we haven't heard from device in >3min, ping for an update.
				// Ping every 5 minutes
				if time.Now().Sub(p.lastPing) > time.Minute*5 {
					addr, err := net.ResolveUDPAddr("udp", p.Addr)
					if err != nil {
						log.Printf("Failed to resolve address of device: %s", err.Error())
					}
					log.Printf("Writing ping to device: %s at %s", p.Name, p.Device.Addr)
					udpConn.WriteToUDP(pingmsg, addr)
					p.lastPing = time.Now()
				}

				// If we haven't gotten an update in a while something is probably wrong.
				// Email once an hour until we figure it out.
				if upDiff > upAlertTime && emailDiff > time.Hour {
					log.Printf("Haven't heard from device: %s since %s", p.Name, p.lastUpdate)
					// sendMail(c.Mailgun, "Refuge Device", "Device '"+p.Name+"' has not responded in over 10 minutes.")
					p.lastEmail = time.Now()
				}
			}
			if p.Portal == nil {
				continue // Dont need t do open checks on non-portals
			}
			opDiff := time.Now().Sub(p.lastOpened)
			// If our garage isn't working correctly or left open, send an alert
			// But only email once per hour (backing off one hour extra each time)
			if opDiff > openAlertTime && emailDiff > time.Hour {
				log.Printf("Portal Alert: %s\n\tOpen duration: %s\n\tLast Updated: %s ago", p.Name, opDiff, upDiff)
				sendMail(c.Mailgun, "Refuge Alert", "Portal "+p.Name+" has been open for over 30 minutes!")
				p.lastEmail = time.Now()
			}
		}
	}
}
