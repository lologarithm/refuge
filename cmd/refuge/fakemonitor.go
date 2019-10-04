package main

import (
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
