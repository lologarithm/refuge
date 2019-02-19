package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
	"gitlab.com/lologarithm/refuge/sensor"
)

var comma = []byte{','}
var colon = []byte{':'}

func jsonSerThemo(dev *refuge.Device) []byte {
	const tmpl string = `{"Device": { "Name":"%s","Addr":"%s", "Thermometer": {"Temp":%f,"Humidity":%f}, "Motion": {"Motion": %d}, "Thermostat":{"State":%d, "Target": %d, "Settings": {"Fan":%d,"Low":%f,"High":%f}}}`
	return []byte(fmt.Sprintf(tmpl, dev.Name, dev.Addr, dev.Thermometer.Temp, dev.Thermometer.Humidity, dev.Motion.Motion, dev.Thermostat.State, dev.Thermostat.Target, dev.Thermostat.Settings.Mode, dev.Thermostat.Settings.Low, dev.Thermostat.Settings.High))
}

func runNetwork(name string, cl climate.Controller, readTherm func() (float32, float32, bool), readMotion func() bool) {
	ds := climate.Settings{
		Low:  19,
		High: 26.66,
		Mode: climate.ModeAuto,
	} // Shove in first desired state
	addrs := rnet.MyIPs()

	addr, err := net.ResolveUDPAddr("udp", addrs[0]+":0")
	if err != nil {
		fmt.Printf("Failed to resolve udp: %s\n", err)
		os.Exit(1)
	}
	// Listen to directed udp messages
	direct, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Printf("Failed to listen to udp: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Listening on: %s\n", direct.LocalAddr())
	directAddr := direct.LocalAddr()

	broadcasts, err := net.ListenMulticastUDP("udp", nil, rnet.RefugeDiscovery)
	if err != nil {
		fmt.Printf("failed to listen to thermo broadcast address: %s\n", err)
		os.Exit(1)
	}

	ts := &refuge.Device{
		Name: name,
		Addr: directAddr.String(),
		Thermostat: &refuge.Thermostat{
			Target: 0,
			Settings: climate.Settings{
				// Default settings on launch
				Mode: ds.Mode,
				Low:  ds.Low,
				High: ds.High,
			},
		},
		Thermometer: &refuge.Thermometer{
			Temp:     0,
			Humidity: 0,
		},
		Motion: &refuge.Motion{
			Motion: 0,
		},
	}

	msg := jsonSerThemo(ts)
	// Look for broadcasts
	// Try to read from network.
	v := climate.Settings{
		High: ts.Thermostat.Settings.High,
		Low:  ts.Thermostat.Settings.Low,
		Mode: climate.ModeAuto,
	}
	lr := time.Time{}
	lastMotion := time.Now()
	motReading := true
	b := make([]byte, 512)
	runControl := false

	readings := []sensor.ThermalReading{}
	for {
		if runControl {
			avgt := float32(0)
			for _, v := range readings {
				avgt += v.Temp
			}
			avgt /= float32(len(readings))
			ts.Thermostat.Target = climate.Control(cl, v, lastMotion, sensor.ThermalReading{Temp: avgt, Humi: ts.Thermometer.Humidity})
			ts.Thermostat.State = cl.State()
			ts.Thermometer.Temp = avgt
			ts.Thermometer.Humidity = readings[len(readings)-1].Humi
			ts.Motion.Motion = lastMotion.Unix()

			msg = jsonSerThemo(ts)
			direct.WriteToUDP(msg, rnet.RefugeMessages)
			runControl = false
		}

		broadcasts.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		n, _, _ := broadcasts.ReadFromUDP(b)
		if n > 0 {
			fmt.Printf("Got message on discovery(%s): %s\n", rnet.RefugeDiscovery, string(b[:n]))
			// Broadcast on ping
			direct.WriteToUDP(msg, rnet.RefugeMessages)
		}

		direct.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		n, _, _ = direct.ReadFromUDP(b)
		if n > 0 {
			bits := b[:n]
			if bits[0] == '{' && bits[len(bits)-1] == '}' {
				assignments := bytes.Split(bits[1:len(bits)-1], comma)
				for _, assign := range assignments {
					parts := bytes.Split(assign, colon)
					name := string(parts[0])
					val := string(parts[1])
					if name == `"Low"` {
						l, _ := strconv.ParseFloat(val, 32)
						fmt.Printf("Assigning low to be %f\n", l)
						v.Low = float32(l)
					} else if name == `"High"` {
						h, _ := strconv.ParseFloat(val, 32)
						v.High = float32(h)
						fmt.Printf("Assigning high to be %f\n", h)
					} else if name == `"Mode"` {
						// v.Mode =
					} else {
						fmt.Printf("Unknown key: %s\n", name)
					}
				}
				ts.Thermostat.Settings.High = v.High
				ts.Thermostat.Settings.Low = v.Low
				// ts.Thermostat.Settings.Mode = v.Mode
				runControl = true
			}
		}

		if readMotion != nil {
			mot := readMotion()
			if mot {
				lastMotion = time.Now()
			}
			if mot != motReading {
				fmt.Printf("Motion State Changed to: %v. Previous Motion was at: %s\n", motReading, lastMotion.Format("Jan 2 15:04:05"))
				motReading = mot
				runControl = true
			}
		} else {
			lastMotion = time.Now()
		}

		if time.Now().Sub(lr) < time.Minute {
			time.Sleep(time.Millisecond * 250)
			continue
		}

		// Reads thermal readings, forwards to the climate controller
		// and copies to the network for the web interface to see.
		for i := 0; i < 10; i++ {
			t, h, csg := readTherm()
			if csg {
				if len(readings) > 0 {
					diff := abs(t - readings[len(readings)-1].Temp)
					if diff > 10 {
						// Unlikely this big of a jump would happen
						print("Last reading >10C different in the past 5 minutes. Ignoring reading.\n")
						break
					}
				}
				readings = append(readings, sensor.ThermalReading{Temp: t, Humi: h})
				runControl = true
				break
			}
		}

		if len(readings) > 2 {
			readings = readings[1:]
		}
		lr = time.Now()
	}
}

func abs(a float32) float32 {
	if a >= 0 {
		return a
	}
	return -a
}
