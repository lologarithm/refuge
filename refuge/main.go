package main

import (
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"gitlab.com/lologarithm/thermo/climate"
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
	*sync.Mutex // Mutex for the thermostat list
	Thermostats map[string]refugenet.Thermostat
}

func serve(host string, stream chan refugenet.Thermostat) {
	// localTime := time.Location{}
	pd := &PageData{
		Mutex:       &sync.Mutex{},
		Thermostats: make(map[string]refugenet.Thermostat, 3),
	}

	updates := make(chan []byte, 10)

	go func() {
		for td := range stream {
			// Update our cached thermostats
			pd.Lock()
			pd.Thermostats[strings.Replace(td.Name, " ", "", -1)] = td
			pd.Unlock()

			// Now push the update to all connected websockets
			d, err := json.Marshal(td)
			if err != nil {
				log.Printf("Failed to marshal thermal data to json: %s", err)
			}
			updates <- d
		}
	}()

	http.HandleFunc("/stream", makeClientStream(updates, pd))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Technically not sending anything over template right now...
		tmpl, err := template.ParseFiles("index.html")
		if err != nil {
			log.Fatalf("unable to parse html: %s", err)
		}
		tmpl.Execute(w, nil)
	})
	log.Printf("starting webhost on: %s", host)
	err := http.ListenAndServe(host, nil)
	if err != nil {
		log.Fatal(err)
	}
}

var upgrader = websocket.Upgrader{} // use default options

func makeClientStream(updates chan []byte, pd *PageData) http.HandlerFunc {
	streamlock := sync.Mutex{}
	clientStreams := make([]*websocket.Conn, 0, 10)

	go func() {
		// This goroutine will push updates from server to each connected client.
		// Any socket that is dead will be removed here.
		for v := range updates {
			deadstreams := []int{}
			streamlock.Lock()
			for i, cs := range clientStreams {
				err := cs.WriteMessage(websocket.TextMessage, v)
				if err != nil {
					deadstreams = append(deadstreams, i)
				}
			}
			// remove dead streams now
			for i := len(deadstreams) - 1; i > -1; i-- {
				idx := deadstreams[i]
				clientStreams = append(clientStreams[:idx], clientStreams[idx+1:]...)
			}
			streamlock.Unlock()
		}
	}()

	return func(w http.ResponseWriter, r *http.Request) {
		c := clientStream(w, r)
		pd.Lock()
		for _, v := range pd.Thermostats {
			c.WriteJSON(v)
		}
		pd.Unlock()
		streamlock.Lock()
		clientStreams = append(clientStreams, c)
		streamlock.Unlock()
	}
}

type changeRequest struct {
	climate.Settings
	Name string // name of thermo to change
}

func clientStream(w http.ResponseWriter, r *http.Request) *websocket.Conn {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return nil
	}
	go func() {
		for {
			v := &changeRequest{}
			err := c.ReadJSON(v)
			if err != nil {
				log.Println("read err:", err)
				break
			}
			log.Printf("recv: %#v", v)
			// TODO: actually make request to remote thermostat!
		}
		c.Close()
	}()
	return c
}
