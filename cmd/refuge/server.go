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
	Switches    map[string]rnet.Switch
	Portals     map[string]rnet.Portal
}

// Request is sent from websocket client to server to request change to someting
type Request struct {
	Climate *ClimateChange
	Switch  *rnet.Switch
	Portal  *rnet.Portal
}

type ClimateChange struct {
	climate.Settings
	Name string // name of thermo to change
}

func serve(host string, deviceStream chan rnet.Msg) {
	// localTime := time.Location{}
	pd := &PageData{
		Mutex:       &sync.Mutex{},
		Thermostats: make(map[string]rnet.Thermostat, 3),
		Switches:    map[string]rnet.Switch{},
		Portals:     map[string]rnet.Portal{},
	}

	updates := make(chan []byte, 10)

	go func() {
		for {
			msg := <-deviceStream

			switch {
			case msg.Thermostat != nil:
				td := msg.Thermostat
				// Update our cached thermostat
				pd.Lock()
				pd.Thermostats[strings.Replace(td.Name, " ", "", -1)] = *td
				pd.Unlock()
				log.Printf("Thermo state is now: %#v", td)
			case msg.Switch != nil:
				fd := msg.Switch
				// Update our cached thermostats
				pd.Lock()
				pd.Switches[strings.Replace(fd.Name, " ", "", -1)] = *fd
				pd.Unlock()
				log.Printf("Switch state is now: %#v", fd)
			case msg.Portal != nil:
				p := msg.Portal
				// Update our cached thermostats
				pd.Lock()
				pd.Portals[strings.Replace(p.Name, " ", "", -1)] = *p
				pd.Unlock()
				log.Printf("Portal state is now: %#v", p)

			}
			// Now push the update to all connected websockets
			d, err := json.Marshal(msg)
			log.Printf("Writing: %v", string(d))
			if err != nil {
				log.Printf("Failed to marshal thermal data to json: %s", err)
			}
			updates <- d
		}
	}()

	http.HandleFunc("/stream", makeClientStream(updates, pd))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if addr := r.Header.Get("X-Echols-A"); addr != "" {
			// Do Auth!
			log.Printf("Forwarded from: %#v", addr)
			if !strings.HasPrefix(addr, "192.168.") {
				user, pwd, ok := r.BasicAuth()
				if !ok || user != "echols" || pwd != "family" {
					w.Header().Set("WWW-Authenticate", `Basic realm="Refuge"`)
					w.WriteHeader(http.StatusForbidden)
					w.Write([]byte("NO ACCESS."))
				}
			}
		}
		// Technically not sending anything over template right now...
		tmpl, err := template.ParseFiles("./assets/house.html")
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
		for _, v := range pd.Switches {
			c.WriteJSON(&rnet.Msg{Switch: &v})
		}
		for _, v := range pd.Thermostats {
			c.WriteJSON(&rnet.Msg{Thermostat: &v})
		}
		for _, v := range pd.Portals {
			c.WriteJSON(&rnet.Msg{Portal: &v})
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
			if v.Switch != nil {
				toggleSwitch(v.Switch.Name, pd)
			}
			if v.Portal != nil {
				togglePortal(v.Portal.Name, pd)
			}
			// TODO: actually make request to remote thermostat!
		}
		c.Close()
	}()
	return c
}

func togglePortal(name string, pd *PageData) {
	var addr string
	state := rnet.PortalStateOpen
	name = strings.Replace(name, " ", "", -1)

	pd.Lock()
	addr = pd.Portals[name].Addr
	if pd.Portals[name].State == rnet.PortalStateOpen {
		state = rnet.PortalStateClosed
	}
	pd.Unlock()

	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatalf("failed to resolve thermo broadcast address: %s", err)
	}

	msg, _ := json.Marshal(rnet.Portal{Name: name, State: state})
	log.Printf("Sending Portal Update to: '%s' to (%s)'%s'", string(msg), addr, raddr)
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		log.Printf("Failed to open UDP: %s", err)
		return
	}
	conn.Write(msg)
	conn.Close()
}

func toggleSwitch(name string, pd *PageData) {
	var addr string
	var state bool
	name = strings.Replace(name, " ", "", -1)
	pd.Lock()
	addr = pd.Switches[name].Addr
	state = pd.Switches[name].On
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
	c.Name = strings.Replace(c.Name, " ", "", -1)
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
