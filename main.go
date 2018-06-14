package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"gitlab.com/lologarithm/thermo/climate"
	"gitlab.com/lologarithm/thermo/sensor"
)

func main() {
	pinN := flag.Int("pin", -1, "input pin to read")
	host := flag.String("host", ":80", "host:port to serve on")
	flag.Parse()
	if *pinN == -1 {
		fmt.Printf("no input pin set.")
		os.Exit(1)
	}
	run(*pinN, *host)
}

func run(pin int, host string) {

	stream := make(chan sensor.Measurement, 10)
	sensor.Stream(pin, time.Second*30, stream)

	data := make([]sensor.Measurement, 1440)
	var index int32

	climateStream := make(chan sensor.Measurement, 10)
	set := climate.Control(climateStream)
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
		writePage(w)
	})

	http.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			fmt.Printf("Failed to parse set form data: %s\n", err)
			return
		}
		// target, _ = strconv.ParseFloat(r.FormValue("goalc"), 32)
		if _, ok := r.Form["upc"]; ok {
			target++
		} else if _, ok := r.Form["downc"]; ok {
			target--
		}
		lowC := float32(target-5-32) * (5 / 9)
		highC := float32(target+5-32) * (5 / 9)
		// mode := r.FormValue("mode")
		set(climate.Settings{Low: lowC, High: highC, Mode: climate.AutoMode})
		writePage(w)
	})

	err := http.ListenAndServe(host, nil)
	if err != nil {
		log.Fatal(err)
	}
}
