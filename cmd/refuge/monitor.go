package main

import (
	"encoding/json"
	"log"
	"net"
	"time"

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
					Thermostat:  &refuge.Thermostat{Target: 23.5, State: refuge.StateCooling, Settings: refuge.Settings{High: 25, Low: 18}},
					Thermometer: &refuge.Thermometer{Temp: 25 + (float32(i % 3)), Humidity: 10.1},
				},
			}
			time.Sleep(3 * time.Second)
			dev := &refuge.Device{
				Name:        "Test Family Room",
				Thermostat:  &refuge.Thermostat{Settings: refuge.Settings{High: 26, Low: 18}},
				Thermometer: &refuge.Thermometer{Temp: 17 + (float32(i % 3)), Humidity: 10.1},
			}
			if dev.Thermometer.Temp < dev.Thermostat.Settings.Low {
				dev.Thermostat.State = refuge.StateHeating
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
			err := dec.Decode(&reading)
			if err != nil {
				log.Printf("Failed to decode json msg: %s", err)
				continue
			}
			if reading.Device == nil {
				log.Printf("Device is nil")
				continue
			}
			switch {
			case reading.Thermostat != nil:
				// log.Printf("New reading: %#v", reading.Thermostat)
			case reading.Switch != nil:
				// log.Printf("New Switch: %#v", reading.Switch)
			case reading.Portal != nil:
				log.Printf("Portal Update: %#v", reading.Portal)
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

const openAlertTime = time.Minute * 30
const upAlertTime = time.Minute * 5

func portalAlert(c *Config, deviceUpdates chan refuge.Device) {
	// Portal watcher
	devices := map[string]*DeviceState{}
	for {
		select {
		case up, ok := <-deviceUpdates:
			if !ok {
				return
			}
			// For now only do alerts on portals
			if up.Portal == nil {
				break
			}
			existing, ok := devices[up.Name]
			if !ok {
				existing = &DeviceState{Device: refuge.Device{Portal: &refuge.Portal{}}}
				devices[up.Name] = existing
			}
			port := existing.Portal
			if port.State != refuge.PortalStateOpen && up.Portal.State == refuge.PortalStateOpen {
				// If just opened, set the time.
				log.Printf("Portal %s is open... starting timer for alert.", up.Name)
				existing.lastOpened = time.Now()
			} else if up.Portal.State != refuge.PortalStateOpen {
				// if not open now, keep updating.
				existing.lastOpened = time.Now()
				existing.numEmails = 0 // reset emails sent
			}
			existing.Portal = up.Portal
			existing.lastUpdate = time.Now()
		case <-time.After(time.Second * 5):
			break
		}

		pinged := false
		for _, p := range devices {
			upDiff := time.Now().Sub(p.lastUpdate)
			opDiff := time.Now().Sub(p.lastOpened)
			emailDiff := time.Now().Sub(p.lastEmail)

			if upDiff > time.Minute*2 { // if we haven't heard from device in >2min, ping for an update.
				// If we haven't heard in 5min... something is prob wrong.
				if upDiff > upAlertTime && (emailDiff > time.Hour*time.Duration(p.numEmails)) {
					log.Printf("Haven't heard from device: %s since %s", p.Name, p.lastUpdate)
					sendMail(c.Mailgun, "Refuge Device", "Device "+p.Name+" has not responded in over 5 minutes.")
					p.numEmails++
					p.lastEmail = time.Now()
				}
				if !pinged {
					ping()
					pinged = true
				}
				// If we haven't heard from a device we dont know its status, skip to next device
				continue
			}

			// If our garage isn't working correctly or left open, send an alert
			// But only email once per hour (backing off one hour extra each time)
			if opDiff > openAlertTime && emailDiff > time.Hour*time.Duration(p.numEmails) {
				log.Printf("Portal Alert: %s\n\tOpen duration: %s\n\tLast Updated: %s ago", p.Name, opDiff, upDiff)
				p.lastEmail = time.Now()
				p.numEmails++
				sendMail(c.Mailgun, "Refuge Alert", "Portal "+p.Name+" has been open for over 30 minutes!")
			}
		}
	}
}
