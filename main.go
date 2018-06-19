package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
	"gitlab.com/lologarithm/thermo/climate"
	"gitlab.com/lologarithm/thermo/sensor"
)

func main() {
	tpin := flag.Int("tpin", 4, "input pin to read for temp")
	hpin := flag.Int("hpin", 24, "output pin to turn on heat")
	cpin := flag.Int("cpin", 22, "output pin to turn on cooling")
	fpin := flag.Int("fpin", 23, "output pin to turn on fan")
	host := flag.String("host", ":80", "host:port to serve on")
	flag.Parse()
	fmt.Printf("Thermo Pin: %d\nHeating Pin: %d\nCooling Pin: %d\nFan Pin: %d\n", *tpin, *hpin, *cpin, *fpin)
	run(*tpin, *fpin, *cpin, *hpin, *host)
}

func run(tpin, fanpin, coolpin, heatpin int, host string) {
	stream := make(chan sensor.Measurement, 10)
	climateStream := make(chan sensor.Measurement, 10)
	data := make([]sensor.Measurement, 1440)
	var index int32
	set := func(_ climate.Settings) {}
	cs := climate.Settings{
		Low:  15.55,
		High: 26.66,
		Mode: climate.AutoMode,
	}

	err := rpio.Open()
	if err != nil {
		fmt.Printf("Unable to open raspberry pi gpio pins: %s\n-----  Defaulting to use fake data.  -----\n", err)
		// send fake data!
		go func() {
			for {
				select {
				case stream <- sensor.Measurement{Temp: 20, Humi: 50, Time: time.Now()}:
				default:
					return // bad, exit
				}
				time.Sleep(time.Second * 30)
			}
		}()
		set = climate.Control(climate.FakeController{}, cs, climateStream)
	} else {
		controller := climate.NewController(heatpin, coolpin, fanpin)
		fmt.Printf("Controller: %v\n", controller)
		set = climate.Control(controller, cs, climateStream)
		sensor.Stream(tpin, time.Second*30, stream)
	}

	target := 70 // F becauses thats what hallie will want

	go func() {
		innerI := -1
		for d := range stream {
			innerI++
			if innerI == len(data) {
				innerI = 0
			}
			// fmt.Printf("Temp: %.1fC(%dF) Humidity: %.1f%%  I:%d\n", d.temp, int(d.temp*9/5)+32, d.hum, i)
			data[innerI] = d
			climateStream <- d
			atomic.StoreInt32(&index, int32(innerI))
		}
		fmt.Printf("exiting data processor")
	}()

	writePage := func(w http.ResponseWriter) {
		d := data[atomic.LoadInt32(&index)]
		pagedata := fmt.Sprintf(page, d.Time.Local().Format("Mon Jan 2 3:04pm"), d.Temp, int(d.Temp*9/5)+32, d.Humi, target)
		w.Write([]byte(pagedata))
	}
	// localTime := time.Location{}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err == nil {
			// target, _ = strconv.ParseFloat(r.FormValue("goalc"), 32)
			change := true
			if _, ok := r.Form["upc"]; ok {
				target++
			} else if _, ok := r.Form["downc"]; ok {
				target--
			} else {
				change = false
			}
			if change {
				lowC := float32(target-3-32) * (5.0 / 9.0)
				highC := float32(target+3-32) * (5.0 / 9.0)
				fmt.Printf("New target: %dF (%.1f-%.1f)\n", target, lowC, highC)
				// mode := r.FormValue("mode")
				set(climate.Settings{Low: lowC, High: highC, Mode: climate.AutoMode})
				climateStream <- data[atomic.LoadInt32(&index)]
			}
		}
		writePage(w)
	})

	err = http.ListenAndServe(host, nil)
	if err != nil {
		log.Fatal(err)
	}
}
