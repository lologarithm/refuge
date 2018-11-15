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
	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/rnet"
)

func main() {
	host := flag.String("host", ":80", "host:port to serve on")
	flag.Parse()
	serve(*host, monitor())
}

func monitor() chan rnet.Thermostat {
	stream := make(chan rnet.Thermostat, 100)
	baddr, err := net.ResolveUDPAddr("udp", rnet.ThermoSpace)
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
			reading := rnet.Thermostat{}
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
	Thermostats map[string]rnet.Thermostat
	Fireplace   map[string]rnet.Fireplace
}

func serve(host string, thermoStream chan rnet.Thermostat) {
	// localTime := time.Location{}
	pd := &PageData{
		Mutex:       &sync.Mutex{},
		Thermostats: make(map[string]rnet.Thermostat, 3),
	}

	updates := make(chan []byte, 10)

	go func() {
		for td := range thermoStream {
			// Update our cached thermostats
			pd.Lock()
			pd.Thermostats[strings.Replace(td.Name, " ", "", -1)] = td
			pd.Unlock()

			// Now push the update to all connected websockets
			d, err := json.Marshal(&Update{Thermostat: &td})
			log.Printf("Writing: %v", string(d))
			if err != nil {
				log.Printf("Failed to marshal thermal data to json: %s", err)
			}
			updates <- d
		}
	}()

	http.HandleFunc("/stream", makeClientStream(updates, pd))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Technically not sending anything over template right now...
		tmpl, err := template.ParseFiles("./assets/index.html")
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
		c := clientStream(w, r, pd)
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

type Update struct {
	Thermostat *rnet.Thermostat
	Fireplace  *rnet.Fireplace
}

type Request struct {
	Climate *ClimateChange
}

type ClimateChange struct {
	climate.Settings
	Name string // name of thermo to change
}

func clientStream(w http.ResponseWriter, r *http.Request, pd *PageData) *websocket.Conn {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return nil
	}

	go func() {
		for {
			v := &Request{}
			err := c.ReadJSON(v)
			if err != nil {
				log.Println("read err:", err)
				break
			}
			if v.Climate != nil {
				log.Printf("Attempt to send message to thermostat here!")
				log.Printf("Climate: %#v", v.Climate)

				var addr string
				pd.Lock()
				addr = pd.Thermostats[v.Climate.Name].Addr
				pd.Unlock()

				raddr, err := net.ResolveUDPAddr("udp", addr)
				if err != nil {
					log.Fatalf("failed to resolve thermo broadcast address: %s", err)
				}

				msg, _ := json.Marshal(&v.Climate.Settings)
				log.Printf("Sending: '%s' to (%s)'%s'", string(msg), addr, raddr)
				conn, err := net.DialUDP("udp", nil, raddr)
				if err != nil {
					log.Printf("Failed to open UDP: %s", err)
					continue
				}
				conn.Write(msg)
				conn.Close()

			}
			// TODO: actually make request to remote thermostat!
		}
		c.Close()
	}()
	return c
}
