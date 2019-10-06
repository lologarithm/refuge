package main

import (
	"fmt"
	"os"
	"time"

	"github.com/lologarithm/netgen/lib/ngservice"
	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
	"gitlab.com/lologarithm/refuge/sensor"
)

var defaults = refuge.Settings{
	Low:  19,
	High: 26.66,
	Mode: refuge.ModeAuto,
}

// thermReader is the function that will return the next thermostat reading.
type thermReader func(includeWait bool) (float32, float32, bool)

func runThermostat(name string, cl climate.Controller, close chan os.Signal, readTherm thermReader, readMotion func() bool) {
	direct, broadcasts := rnet.SetupUDPConns()

	directAddr := direct.LocalAddr()
	listeners := []rnet.Listener{}

	ts := &refuge.Device{
		Name: name,
		ID:   name, // TODO: make name configurable
		Addr: directAddr.String(),
		Thermostat: &refuge.Thermostat{
			Target: 0,
			Settings: refuge.Settings{
				// Default settings on launch
				Mode: defaults.Mode,
				Low:  defaults.Low,
				High: defaults.High,
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

	lr := time.Time{}
	lastMotion := time.Now()
	motReading := true
	b := make([]byte, 256)
	runControl := false

	readings := []sensor.ThermalReading{}
	numReadings := 2 // max number of readings to hold for averaging temp
	for {
		select {
		case <-close:
			return // exit!
		default:
			// continue
		}

		if runControl && len(readings) > 0 {
			fmt.Printf("(%s) Starting control loop...", time.Now().Format("15:04:05 MST"))
			avgt := float32(0)
			for _, v := range readings {
				avgt += v.Temp
			}
			avgt /= float32(len(readings))
			ts.Thermostat.Target = climate.Control(cl, ts.Thermostat.Settings, lastMotion, sensor.ThermalReading{Temp: avgt, Humi: ts.Thermometer.Humidity})
			ts.Thermostat.State = cl.State()
			ts.Thermometer.Temp = avgt
			ts.Thermometer.Humidity = readings[len(readings)-1].Humi
			ts.Motion.Motion = lastMotion.Unix()
			msg = ngservice.WriteMessage(rnet.Context, rnet.Msg{Device: ts})
			fmt.Printf("(%s) Broadcasting new state: %#v %#v\n", time.Now().Format("15:04:05 MST"), ts.Thermometer, ts.Thermostat)
			listeners = rnet.BroadcastAndTimeout(direct, msg, listeners)
			runControl = false
		}

		// Check for broadcast pings
		listeners = rnet.ReadBroadcastPing(broadcasts, listeners, b, msg)

		direct.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		n, remoteAddr, _ := direct.ReadFromUDP(b)
		if n > 0 {
			packet, ok := ngservice.ReadPacket(refuge.Context, b[:n])
			if ok && packet.Header.MsgType == refuge.SettingsMsgType {
				settings := packet.NetMsg.(*refuge.Settings)
				fmt.Printf("(%s) Got new settings request: %#v\n", time.Now().Format("15:04:05 MST"), settings)
				ts.Thermostat.Settings.High = settings.High
				ts.Thermostat.Settings.Low = settings.Low
				if settings.Mode != refuge.ModeUnset {
					ts.Thermostat.Settings.Mode = settings.Mode
				}
			} else if packet.Header.MsgType == rnet.PingMsgType {
				// Just letting us know to respond to them now.
			}
			listeners = rnet.UpdateListeners(listeners, remoteAddr)
			runControl = true
			continue
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
			time.Sleep(time.Millisecond * 100)
			continue
		}

		fmt.Printf("(%s) Starting Reading Thermometer...\n", time.Now().Format("15:04:05 MST"))
		// Reads thermal readings, forwards to the climate controller
		// and copies to the network for the web interface to see.
		includeWait := false // first reading is always waited long enough, skip straight to reading!
		for i := 0; i < 10; i++ {
			t, h, csg := readTherm(includeWait)
			if csg {
				if len(readings) > 0 {
					lastReading := readings[len(readings)-1]
					diff := abs(t - lastReading.Temp)
					if diff > 10 {
						// Unlikely this big of a jump would happen
						fmt.Print("Last reading >10C different than previous readings. Ignoring reading.\n")
						break
					}
					if diff < 0.01 && abs(h-lastReading.Humi) < 0.01 {
						fmt.Print("no difference in last reading... ignoring reading.\n")
						lr = time.Now()
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
			includeWait = true // force a wait between readings
		}
	}
}

func abs(a float32) float32 {
	if a >= 0 {
		return a
	}
	return -a
}
