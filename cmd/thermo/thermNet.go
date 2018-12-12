package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/rnet"
	"gitlab.com/lologarithm/refuge/sensor"
)

var comma = []byte{','}
var colon = []byte{':'}

func jsonSerThemo(ts *rnet.Thermostat) []byte {
	const tmpl string = `{"Thermostat":{"Name":"%s","Addr":"%s","Fan":%d,"Low":%f,"High":%f,"Temp":%f,"Humidity":%f,"Motion":%d,"State":%d}}`
	return []byte(fmt.Sprintf(tmpl, ts.Name, ts.Addr, ts.Fan, ts.Low, ts.High, ts.Temp, ts.Humidity, ts.Motion, ts.State))
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

	ts := rnet.Msg{Thermostat: &rnet.Thermostat{
		// Device Config
		Name: name,
		Addr: directAddr.String(),

		// Default settings on launch
		Fan:  uint8(ds.Mode),
		Low:  ds.Low,
		High: ds.High,

		// No readings yet
		Temp:     0,
		Humidity: 0,
		Motion:   0,
	}}

	msg := jsonSerThemo(ts.Thermostat)
	// Look for broadcasts
	// Try to read from network.
	v := climate.Settings{
		High: ts.Thermostat.High,
		Low:  ts.Thermostat.Low,
	}
	lr := time.Time{}
	lastMotion := time.Now()
	motReading := false
	b := make([]byte, 512)
	runControl := false
	for {
		if runControl {
			climate.Control(cl, v, lastMotion, sensor.ThermalReading{Temp: ts.Thermostat.Temp, Humi: ts.Thermostat.Humidity})
			ts.Thermostat.State = cl.State()
			msg = jsonSerThemo(ts.Thermostat)
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
					fmt.Printf("Assignment: %s\n", assign)
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
					} else {
						fmt.Printf("Unknown key: %s\n", name)
					}
				}
				ts.Thermostat.High = v.High
				ts.Thermostat.Low = v.Low
				// ts.Thermostat.Fan = uint8(v.Mode)
				runControl = true
			}
		}

		mot := readMotion()
		if mot {
			lastMotion = time.Now()
			ts.Thermostat.Motion = lastMotion.Unix()
		}
		if mot != motReading {
			fmt.Printf("Motion State Changed to: %v. Previous Motion was at: %s\n", motReading, lastMotion.Format("Jan 2 15:04:05"))
			motReading = mot
			runControl = true
		}

		if time.Now().Sub(lr) < time.Minute {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		// Reads thermal readings, forwards to the climate controller
		// and copies to the network for the web interface to see.
		for i := 0; i < 10; i++ {
			t, h, csg := readTherm()
			if csg {
				ts.Thermostat.Temp = t
				ts.Thermostat.Humidity = h
				runControl = true
				break
			}
		}
		lr = time.Now()
	}
}
