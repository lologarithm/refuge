package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/rnet"
)

type userAccess struct {
	Name   string
	Pwd    string
	Access int
}

// Access levels
const (
	AccessNone  int = 0
	AccessRead      = 1
	AccessWrite     = 2
)

func auth(w http.ResponseWriter, r *http.Request) int {
	addr := r.RemoteAddr
	if paddr := r.Header.Get("X-Echols-A"); paddr != "" {
		addr = paddr
	}
	// Allow intra-net access without auth.
	if !strings.HasPrefix(addr, "192.168.") && !strings.HasPrefix(addr, "127.0.0.1") {
		name, pwd, _ := r.BasicAuth()
		user, ok := globalConfig.Users[name]
		if !ok || user.Pwd != pwd {
			w.Header().Set("WWW-Authenticate", `Basic realm="Refuge"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("NO ACCESS."))
			return AccessNone
		}
		return user.Access
	}
	return AccessWrite
}

type PageData struct {
	*sync.Mutex // Mutex for the thermostat list
	Thermostats map[string]rnet.Thermostat
	Switches    map[string]rnet.Switch
	Portals     map[string]rnet.Portal
}

type PortalState struct {
	rnet.Portal
	lastUpdate time.Time
	lastOpened time.Time
	lastEmail  time.Time
}

// Request is sent from websocket client to server to request change to someting
type Request struct {
	Climate *ClimateChange
	Switch  *rnet.Switch
	Portal  *rnet.Portal
	Auth    map[string]string
}

type ClimateChange struct {
	climate.Settings
	Name string // name of thermo to change
}

const alertTime = time.Minute * 30

func serve(host string, deviceStream chan rnet.Msg) {
	// localTime := time.Location{}
	pd := &PageData{
		Mutex:       &sync.Mutex{},
		Thermostats: make(map[string]rnet.Thermostat, 3),
		Switches:    map[string]rnet.Switch{},
		Portals:     map[string]rnet.Portal{},
	}

	updates := make(chan []byte, 10)

	portalUpdates := make(chan rnet.Portal, 5)
	go func(c *Config) {
		// Portal watcher
		portals := map[string]*PortalState{}
		for {
			select {
			case up := <-portalUpdates:
				existing, ok := portals[up.Name]
				if !ok {
					existing = &PortalState{}
					portals[up.Name] = existing
				}
				if existing.State != rnet.PortalStateOpen && up.State == rnet.PortalStateOpen {
					// If just opened, set the time.
					existing.lastOpened = time.Now()
				} else if up.State != rnet.PortalStateOpen {
					// if not open now, keep updating.
					existing.lastOpened = time.Now()
				}
				existing.Portal = up
				existing.lastUpdate = time.Now()
			case <-time.After(time.Second * 15):
			}

			for _, p := range portals {
				upDiff := time.Now().Sub(p.lastUpdate)
				opDiff := time.Now().Sub(p.lastOpened)
				emailDiff := time.Now().Sub(p.lastEmail)
				if upDiff > alertTime || opDiff > alertTime || emailDiff > time.Hour {
					log.Printf("Portal Alert: %s\n\tOpen duration: %s\n\tLast Updated: %s ago", p.Name, opDiff, upDiff)
					p.lastEmail = time.Now()
					sendMail(c.Mailgun, "Refuge Alert", "Portal "+p.Name+" has been open for over 30 minutes!")
				}
			}
		}
	}(&globalConfig)

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
			case msg.Switch != nil:
				fd := msg.Switch
				// Update our cached thermostats
				pd.Lock()
				pd.Switches[strings.Replace(fd.Name, " ", "", -1)] = *fd
				pd.Unlock()
			case msg.Portal != nil:
				p := msg.Portal
				// Update our cached thermostats
				pd.Lock()
				pd.Portals[strings.Replace(p.Name, " ", "", -1)] = *p
				pd.Unlock()
				portalUpdates <- *p
			}
			// Now push the update to all connected websockets
			d, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Failed to marshal thermal data to json: %s", err)
			}
			updates <- d
		}
	}()

	http.HandleFunc("/stream", makeClientStream(updates, pd))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) == AccessNone {
			return // Don't let them access
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
		access := auth(w, r)
		if access == AccessNone {
			return
		}
		c := clientStream(w, r, access, pd)
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

func clientStream(w http.ResponseWriter, r *http.Request, access int, pd *PageData) *websocket.Conn {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade failure:", err)
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
			// Readers can't write new settings
			if access != AccessWrite {
				continue
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
