package main

import (
	"encoding/json"
	"log"
	"net"
	"time"

	"gitlab.com/lologarithm/refuge/rnet"
)

func fakeMonitor() chan rnet.Msg {
	tstream := make(chan rnet.Msg, 10)
	go func() {
		i := 0
		for {
			tstream <- rnet.Msg{
				Thermostat: &rnet.Thermostat{Name: "Test Living Room", Temp: 17 + (float32(i % 3)), Humidity: 10.1, High: 30, Low: 18},
			}
			time.Sleep(3 * time.Second)
			tstream <- rnet.Msg{
				Thermostat: &rnet.Thermostat{Name: "Test Family Room", Temp: 17 + (float32(i % 3)), Humidity: 10.1, High: 30, Low: 18},
			}
			time.Sleep(3 * time.Second)
			tstream <- rnet.Msg{
				Switch: &rnet.Switch{Name: "Test Fireplace", On: i%2 == 0},
			}
			time.Sleep(3 * time.Second)
			tstream <- rnet.Msg{
				Portal: &rnet.Portal{Name: "Test Garage Door", State: rnet.PortalState(i % 3)},
			}
			time.Sleep(3 * time.Second)
			i++
		}
	}()

	return tstream
}

func monitor(test bool) chan rnet.Msg {
	if test {
		return fakeMonitor()
	}
	tstream := make(chan rnet.Msg, 10)

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
			switch {
			case reading.Thermostat != nil:
				log.Printf("New reading: %#v", reading.Thermostat)
			case reading.Switch != nil:
				log.Printf("New Switch: %#v", reading.Switch)
			case reading.Portal != nil:
				log.Printf("New Portal: %#v", reading.Portal)
			default:
				log.Printf("Unknown message: %#v", reading)
				continue
			}
			tstream <- reading
		}
	}()
	ping()
	return tstream
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
