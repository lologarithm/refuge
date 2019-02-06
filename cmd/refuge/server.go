package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
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

type server struct {
	datalock    *sync.Mutex
	Thermostats map[string]rnet.Thermostat
	Switches    map[string]rnet.Switch
	Portals     map[string]rnet.Portal

	clientslock   *sync.Mutex
	clientStreams []*websocket.Conn
}

func (srv *server) getPortal(name string) (port rnet.Portal) {
	name = strings.Replace(name, " ", "", -1)
	srv.datalock.Lock()
	port = srv.Portals[name]
	srv.datalock.Unlock()
	return port
}

func (srv *server) getSwitch(name string) (sw rnet.Switch) {
	name = strings.Replace(name, " ", "", -1)
	srv.datalock.Lock()
	sw = srv.Switches[name]
	srv.datalock.Unlock()
	return sw
}

func (srv *server) getThermo(name string) (thermo rnet.Thermostat) {
	name = strings.Replace(name, " ", "", -1)
	srv.datalock.Lock()
	thermo = srv.Thermostats[name]
	srv.datalock.Unlock()
	return thermo
}

// serve creates the state object and http handlers and launches the http server.
// blocks on the http server.
func serve(host string, deviceStream chan rnet.Msg) {
	// localTime := time.Location{}
	srv := &server{
		datalock:    &sync.Mutex{},
		Thermostats: make(map[string]rnet.Thermostat, 3),
		Switches:    map[string]rnet.Switch{},
		Portals:     map[string]rnet.Portal{},
		clientslock: &sync.Mutex{},
	}

	portalUpdates := make(chan rnet.Portal, 5)
	go portalAlert(&globalConfig, portalUpdates)

	// Updater goroutine. Updates data state and pushes the new state to websocket clients
	go func() {
		for {
			msg := <-deviceStream
			switch {
			case msg.Thermostat != nil:
				td := msg.Thermostat
				// Update our cached thermostat
				srv.datalock.Lock()
				srv.Thermostats[strings.Replace(td.Name, " ", "", -1)] = *td
				srv.datalock.Unlock()
			case msg.Switch != nil:
				fd := msg.Switch
				// Update our cached thermostats
				srv.datalock.Lock()
				srv.Switches[strings.Replace(fd.Name, " ", "", -1)] = *fd
				srv.datalock.Unlock()
			case msg.Portal != nil:
				p := msg.Portal
				// Update our cached thermostats
				srv.datalock.Lock()
				srv.Portals[strings.Replace(p.Name, " ", "", -1)] = *p
				srv.datalock.Unlock()
				portalUpdates <- *p // push updates to portal alert system
			}

			// Serialize for clients
			d, err := json.Marshal(msg)
			if err != nil {
				log.Printf("Failed to marshal thermal data to json: %s", err)
			}

			// Now push the update to all connected websockets
			deadstreams := []int{}
			srv.clientslock.Lock()
			for i, cs := range srv.clientStreams {
				err := cs.WriteMessage(websocket.TextMessage, d)
				if err != nil {
					deadstreams = append(deadstreams, i)
				}
			}
			// remove dead streams now
			for i := len(deadstreams) - 1; i > -1; i-- {
				idx := deadstreams[i]
				srv.clientStreams = append(srv.clientStreams[:idx], srv.clientStreams[idx+1:]...)
			}
			srv.clientslock.Unlock()
		}
	}()

	http.HandleFunc("/stream", srv.clientStreamHandler)
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
