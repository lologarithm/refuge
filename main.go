package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

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
	go func() {
		innerI := -1
		for d := range stream {
			innerI++
			if innerI == len(data) {
				innerI = 0
			}
			// fmt.Printf("Temp: %.1fC(%dF) Humidity: %.1f%%  I:%d\n", d.temp, int(d.temp*9/5)+32, d.hum, i)
			data[innerI] = d
			atomic.StoreInt32(&index, int32(innerI))
		}
		fmt.Printf("exiting data processor")
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		d := data[atomic.LoadInt32(&index)]
		w.Write([]byte(fmt.Sprintf("<html><body style=\"font-size: 2em;\"><p><b>At:</b>	 %s</p><p><b>Temp:</b> %.1fC(%dF)</p><p><b>Humidity:</b> %.1f%%</p></html></body>", d.Time.Format("Mon Jan _2 15:04:05 2006"), d.Temp, int(d.Temp*9/5)+32, d.Humi)))
	})

	err := http.ListenAndServe(host, nil)
	if err != nil {
		log.Fatal(err)
	}
}
