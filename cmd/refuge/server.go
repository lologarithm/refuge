package main

import (
	"encoding/json"
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

type PageData struct {
	*sync.Mutex // Mutex for the thermostat list
	Thermostats map[string]rnet.Thermostat
	Fireplace   map[string]rnet.Switch
}

// Update notifies websocket listeners of updates to thermostats and fireplaces
type Update struct {
	Thermostat *rnet.Thermostat
	Fireplace  *rnet.Switch
}

// Request is sent from websocket client to server to request change to someting
type Request struct {
	Climate   *ClimateChange
	Fireplace *rnet.Switch
}

type ClimateChange struct {
	climate.Settings
	Name string // name of thermo to change
}

func serve(host string, thermoStream chan rnet.Thermostat, switchStream chan rnet.Switch) {
	// localTime := time.Location{}
	pd := &PageData{
		Mutex:       &sync.Mutex{},
		Thermostats: make(map[string]rnet.Thermostat, 3),
		Fireplace:   map[string]rnet.Switch{},
	}

	updates := make(chan []byte, 10)

	go func() {
		for {
			select {
			case td := <-thermoStream:
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
			case fd := <-switchStream:
				// Update our cached thermostats
				pd.Lock()
				pd.Fireplace[strings.Replace(fd.Name, " ", "", -1)] = fd
				log.Printf("Fireplace state is now: %#v", fd)
				pd.Unlock()

				// Now push the update to all connected websockets
				d, err := json.Marshal(&Update{Fireplace: &fd})
				log.Printf("Writing: %v", string(d))
				if err != nil {
					log.Printf("Failed to marshal thermal data to json: %s", err)
				}
				updates <- d

			}
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
		for _, v := range pd.Fireplace {
			c.WriteJSON(&Update{Fireplace: &v})
		}
		for _, v := range pd.Thermostats {
			c.WriteJSON(&Update{Thermostat: &v})
		}
		pd.Unlock()
		streamlock.Lock()
		clientStreams = append(clientStreams, c)
		streamlock.Unlock()
	}
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
				writeNewTherm(*v.Climate, pd)
			}
			if v.Fireplace != nil {
				toggleFireplace(v.Fireplace.Name, pd)
			}
			// TODO: actually make request to remote thermostat!
		}
		c.Close()
	}()
	return c
}

func toggleFireplace(name string, pd *PageData) {
	var addr string
	var state bool

	pd.Lock()
	addr = pd.Fireplace[name].Addr
	state = pd.Fireplace[name].On
	pd.Unlock()

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatalf("failed to resolve thermo broadcast address: %s", err)
	}

	msg, _ := json.Marshal(rnet.Switch{Name: name, On: !state})
	log.Printf("Sending: '%s' to (%s)'%s'", string(msg), addr, raddr)
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Printf("Failed to open UDP: %s", err)
		return
	}
	conn.Write(msg)
	conn.Close()
}

func writeNewTherm(c ClimateChange, pd *PageData) {
	log.Printf("Climate: %#v", c)
	var addr string

	pd.Lock()
	addr = pd.Thermostats[c.Name].Addr
	pd.Unlock()

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatalf("failed to resolve thermo broadcast address: %s", err)
	}

	msg, _ := json.Marshal(&c.Settings)
	log.Printf("Sending: '%s' to (%s)'%s'", string(msg), addr, raddr)
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Printf("Failed to open UDP: %s", err)
		return
	}
	conn.Write(msg)
	conn.Close()
}
