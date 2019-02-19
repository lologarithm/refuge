package main

import (
	"encoding/json"
	"log"
	"net"
	"time"

	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
)

func fakeMonitor() chan rnet.Msg {
	tstream := make(chan rnet.Msg, 10)
	go func() {
		i := 0
		for {
			tstream <- rnet.Msg{
				Device: &refuge.Device{
					Name:        "Test Living Room",
					Thermostat:  &refuge.Thermostat{Target: 23.5, State: climate.StateCooling, Settings: climate.Settings{High: 25, Low: 18}},
					Thermometer: &refuge.Thermometer{Temp: 25 + (float32(i % 3)), Humidity: 10.1},
				},
			}
			time.Sleep(3 * time.Second)
			dev := &refuge.Device{
				Name:        "Test Family Room",
				Thermostat:  &refuge.Thermostat{Settings: climate.Settings{High: 30, Low: 20}},
				Thermometer: &refuge.Thermometer{Temp: 17 + (float32(i % 3)), Humidity: 10.1},
			}
			if dev.Thermometer.Temp < dev.Thermostat.Settings.Low {
				dev.Thermostat.State = climate.StateHeating
				dev.Thermostat.Target = 21.5
			}
			tstream <- rnet.Msg{
				Device: dev,
			}
			time.Sleep(3 * time.Second)
			tstream <- rnet.Msg{
				Device: &refuge.Device{
					Name:   "Test Fireplace",
					Switch: &refuge.Switch{On: i%2 == 0},
				},
			}
			time.Sleep(3 * time.Second)
			tstream <- rnet.Msg{
				Device: &refuge.Device{
					Name: "Test Garage Door",
					Portal: &refuge.Portal{
						State: refuge.PortalState(i%2 + 1),
					},
				},
			}
			time.Sleep(3 * time.Second)
			i++
		}
	}()

	return tstream
}

// monitor monitors for network messages and decodes/passes them along to the main processor
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
		log.Printf("[Error] Failed to request a ping from discovery network: %s", err)
	}
	udpConn, err := net.ListenUDP("udp", local)
	if err != nil {
		log.Printf("[Error] Failed to listen to udp socket: %s", err)
	}
	n, err := udpConn.WriteToUDP([]byte("{}"), rnet.RefugeDiscovery)
	if n == 0 || err != nil {
		log.Printf("[Error] Failed to write to UDP! Bytes: %d, Err: %s", n, err)
	}
}

type DeviceState struct {
	refuge.Device
	lastUpdate time.Time
	lastOpened time.Time
	lastEmail  time.Time
	numEmails  int
}

const alertTime = time.Minute * 30

func portalAlert(c *Config, deviceUpdates chan refuge.Device) {
	// Portal watcher
	devices := map[string]*DeviceState{}
	for {
		select {
		case up := <-deviceUpdates:
			existing, ok := devices[up.Name]
			if !ok {
				existing = &DeviceState{}
				devices[up.Name] = existing
			}
			port := existing.Portal
			// For now only do alerts on portals
			if port == nil {
				continue
			}
			if port.State != refuge.PortalStateOpen && up.Portal.State == refuge.PortalStateOpen {
				// If just opened, set the time.
				existing.lastOpened = time.Now()
			} else if up.Portal.State != refuge.PortalStateOpen {
				// if not open now, keep updating.
				existing.lastOpened = time.Now()
			}
			existing.Portal = up.Portal
			existing.lastUpdate = time.Now()
		case <-time.After(time.Minute * 5):
			ping()
		}

		for _, p := range devices {
			upDiff := time.Now().Sub(p.lastUpdate)
			opDiff := time.Now().Sub(p.lastOpened)
			emailDiff := time.Now().Sub(p.lastEmail)
			// If our garage isn't working correctly or left open, send an alert
			// But only email once per hour (backing off one hour extra each time)
			if (upDiff > alertTime || opDiff > alertTime) && (emailDiff > time.Hour*time.Duration(p.numEmails)) {
				log.Printf("Portal Alert: %s\n\tOpen duration: %s\n\tLast Updated: %s ago", p.Name, opDiff, upDiff)
				p.lastEmail = time.Now()
				p.numEmails++
				// sendMail(c.Mailgun, "Refuge Alert", "Portal "+p.Name+" has been open for over 30 minutes!")
			}
		}
	}
}
