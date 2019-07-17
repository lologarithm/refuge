package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/lologarithm/netgen/lib/ngservice"
	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
	"gitlab.com/lologarithm/refuge/sensor"
)

func runNetwork(name string, cl climate.Controller, readTherm func() (float32, float32, bool), readMotion func() bool) {
	ds := refuge.Settings{
		Low:  19,
		High: 26.66,
		Mode: refuge.ModeAuto,
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

	listeners := []rnet.Listener{}

	ts := &refuge.Device{
		Name: name,
		Addr: directAddr.String(),
		Thermostat: &refuge.Thermostat{
			Target: 0,
			Settings: refuge.Settings{
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

	msg := ngservice.WriteMessage(rnet.Context, rnet.Msg{Device: ts})
	// Look for broadcasts
	// Try to read from network.
	v := refuge.Settings{
		High: ts.Thermostat.Settings.High,
		Low:  ts.Thermostat.Settings.Low,
		Mode: refuge.ModeAuto,
	}
	lr := time.Time{}
	lastMotion := time.Now()
	motReading := true
	b := make([]byte, 256)
	runControl := false

	readings := []sensor.ThermalReading{}
	numReadings := 2 // max number of readings to hold for averaging temp
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
			msg = ngservice.WriteMessage(rnet.Context, rnet.Msg{Device: ts})
			rnet.BroadcastAndTimeout(direct, msg, listeners)
			runControl = false
		}

		broadcasts.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		n, remoteAddr, _ := broadcasts.ReadFromUDP(b)
		if n > 0 {
			fmt.Printf("Got message on discovery(%s) from: %s\n", rnet.RefugeDiscovery, remoteAddr.String())
			listeners = rnet.UpdateListeners(listeners, remoteAddr)
			// Emit current state to pinger.
			direct.WriteToUDP(msg, remoteAddr)
		}

		direct.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		n, remoteAddr, _ = direct.ReadFromUDP(b)
		if n > 0 {
			packet, ok := ngservice.ReadPacket(refuge.Context, b[:n])
			if ok && packet.Header.MsgType == refuge.SettingsMsgType {
				settings := packet.NetMsg.(*refuge.Settings)
				ts.Thermostat.Settings.High = settings.High
				ts.Thermostat.Settings.Low = settings.Low
				ts.Thermostat.Settings.Mode = settings.Mode
			}
			listeners = rnet.UpdateListeners(listeners, remoteAddr)
			runControl = true
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

		// Only re-read sensors once every 2 minutes and when there is no re-controlling to run.
		if time.Now().Sub(lr) < time.Minute*2 && !runControl {
			time.Sleep(time.Millisecond * 200)
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
						print("Last reading >10C different than previous readings. Ignoring reading.\n")
						break
					}
				}
				readings = append(readings, sensor.ThermalReading{Temp: t, Humi: h})
				if len(readings) > numReadings {
					copy(readings, readings[1:])
					readings = readings[:numReadings]
				}
				runControl = true
				lr = time.Now()
				break
			}
		}
	}
}

func abs(a float32) float32 {
	if a >= 0 {
		return a
	}
	return -a
}
