package main

import (
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"net"
	"net/http"
	"sync"

	"gitlab.com/lologarithm/thermo/refuge/refugenet"
)

func main() {
	host := flag.String("host", ":80", "host:port to serve on")
	flag.Parse()
	serve(*host, monitor())
}

func monitor() chan refugenet.Thermostat {
	stream := make(chan refugenet.Thermostat, 100)
	baddr, err := net.ResolveUDPAddr("udp", refugenet.ThermoSpace)
	if err != nil {
		log.Fatalf("failed to resolve thermo broadcast address: %s", err)
	}
	udp, err := net.ListenMulticastUDP("udp", nil, baddr)
	if err != nil {
		log.Fatalf("failed to listen to thermo broadcast address: %s", err)
	}
	dec := json.NewDecoder(udp)
	go func() {
		for {
			reading := refugenet.Thermostat{}
			err := dec.Decode(&reading)
			if err != nil {
				log.Printf("Failed to decode json msg: %s", err)
				// lol
			}
			log.Printf("New reading: %#v", reading)
			stream <- reading
		}
	}()
	return stream
}

type PageData struct {
	Thermostats map[string]refugenet.Thermostat
}

func serve(host string, stream chan refugenet.Thermostat) {
	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		log.Fatalf("unable to parse html: %s", err)
	}
	// localTime := time.Location{}
	l := sync.Mutex{}
	pd := &PageData{
		Thermostats: make(map[string]refugenet.Thermostat, 3),
	}

	go func() {
		for td := range stream {
			l.Lock()
			pd.Thermostats[td.Name] = td
			l.Unlock()
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		l.Lock()
		tmpl.Execute(w, pd)
		l.Unlock()
	})

	log.Printf("starting webhost on: %s", host)
	err = http.ListenAndServe(host, nil)
	if err != nil {
		log.Fatal(err)
	}
}
