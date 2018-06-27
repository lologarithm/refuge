package main

import (
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"net"
	"net/http"
	"time"

	"gitlab.com/lologarithm/thermo/refuge/refugenet"
)

func main() {
	host := flag.String("host", ":80", "host:port to serve on")
	flag.Parse()
	serve(*host, monitor())
}

type Thermostat struct {
	Name     string  // Name of thermostat
	Target   float32 // Targeted temp
	Temp     float32 // Last temp reading
	Humidity float32 // Last humidity reading
}

func monitor() chan Thermostat {
	stream := make(chan Thermostat, 100)
	baddr, _ := net.ResolveUDPAddr("udp", refugenet.ThermoSpace)
	udp, _ := net.ListenUDP("udp", baddr)
	buffer := make([]byte, 2048)
	dec := json.NewDecoder(udp)
	go func() {
		reading := Thermostat{}
		err := dec.Decode(&reading)
		if err != nil {
			// lol
		}
		stream <- reading
	}()
	go func() {
		v, n, err := udp.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Failed to read from buffer, sleeping a bit before trying again")
			time.Sleep(time.Second * 5)
			udp = net.ListenUDP()
		}
	}()
	return stream
}

type PageData struct {
	Master Thermostat
	Living Thermostat
	Family Thermostat
}

func serve(host string, stream chan Thermostat) {
	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		log.Fatalf("unable to parse html: %s", err)
	}
	// localTime := time.Location{}
	pd := &PageData{}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl.Execute(w, pd)
	})

	err = http.ListenAndServe(host, nil)
	if err != nil {
		log.Fatal(err)
	}
}
