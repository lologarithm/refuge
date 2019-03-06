package main

import (
	"encoding/gob"
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/websocket"
	"gitlab.com/lologarithm/refuge/refuge"
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
	datalock   *sync.RWMutex
	Devices    map[string]*refugeDevice
	devUpdates chan refuge.Device

	clientslock   *sync.Mutex
	clientStreams []*websocket.Conn

	eventData []refuge.TempEvent
	done      chan struct{}
}

func runServer(deviceStream chan rnet.Msg) *server {
	srv := &server{
		datalock:    &sync.RWMutex{},
		Devices:     map[string]*refugeDevice{},
		clientslock: &sync.Mutex{},
		done:        make(chan struct{}, 1),
		devUpdates:  make(chan refuge.Device, 5), // Updates from network -> portal watcher
	}
	go portalAlert(&globalConfig, srv.devUpdates)
	// Updater goroutine. Updates data state and pushes the new state to websocket clients
	go eventListener(srv, deviceStream, srv.devUpdates, srv.done)
	return srv
}

// Getter functions convert names to device keys (avoiding spaces)

func (srv *server) getDevice(name string) (device *refugeDevice) {
	name = strings.Replace(name, " ", "", -1)
	if name == "" {
		log.Printf("[Error] Attempted to fetch an empty name string!")
		return nil
	}
	srv.datalock.Lock()
	device = srv.Devices[name]
	srv.datalock.Unlock()
	return device
}
func (srv *server) stop() {
	close(srv.devUpdates)
	<-srv.done
}

type refugeDevice struct {
	device refuge.Device
	conn   *net.UDPConn
	pos    Position
}

// serve creates the state object "server" and http handlers and launches the http listener.
// Blocks on ctrl+c so we can safely write the stats file.
func serve(host string, deviceStream chan rnet.Msg) {
	srv := runServer(deviceStream)
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		access := auth(w, r)
		if access == AccessNone {
			return
		}
		enc := json.NewEncoder(w)
		srv.datalock.RLock()
		enc.Encode(srv.eventData)
		srv.datalock.RUnlock()
	})
	// Little weather proxy/cache for the frontends
	http.HandleFunc("/weather", weather())
	http.HandleFunc("/stream", srv.clientStreamHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) == AccessNone {
			return // Don't let them access
		}
		path := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		switch path[0] {
		case "static", "assets":
			if path[1] == "house.html" {
				break
			}
			Static(w, r, path)
			return
		}
		// Default to base site.
		tmpl, err := template.ParseFiles("./assets/house.html")
		if err != nil {
			log.Fatalf("unable to parse html: %s", err)
		}
		tmpl.Execute(w, nil)

	})

	log.Printf("starting webhost on: %s", host)
	go func() {
		err := http.ListenAndServe(host, nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	close(deviceStream) // close the stats file we have been writing.
	srv.stop()          // wait for server to stop
	log.Printf("Done!")
}

// Static will serve the given file from the path in the url.
func Static(w http.ResponseWriter, r *http.Request, path []string) {
	if len(path) < 2 || !strings.Contains(path[len(path)-1], ".") {
		// Only serve actual files here.
		return
	}
	file := strings.Join(path, "/")
	if strings.Contains(file, "/gz/") {
		w.Header().Set("Content-Encoding", "gzip")
	}
	http.ServeFile(w, r, file)
}

func eventListener(srv *server, deviceStream chan rnet.Msg, deviceUpdates chan refuge.Device, done chan struct{}) {
	// Load all existing stats from file.
	events := LoadStats()
	srv.datalock.Lock()
	srv.eventData = events
	srv.datalock.Unlock()
	// Now open new file to write to
	statFile := GetStatsFile()
	enc := gob.NewEncoder(statFile)
	for {
		msg, ok := <-deviceStream
		if !ok {
			statFile.Sync()
			statFile.Close()
			done <- struct{}{}
			return
		}
		td := msg.Device
		existing := srv.getDevice(td.Name)
		newd := &refugeDevice{
			device: *td,
		}
		if existing != nil {
			newd.pos = existing.pos
			if existing.device.Addr != td.Addr {
				raddr, err := net.ResolveUDPAddr("udp", td.Addr)
				if err != nil {
					log.Fatalf("failed to resolve thermo broadcast address: %s", err)
				}
				conn, err := net.DialUDP("udp", nil, raddr)
				if err != nil {
					log.Printf("Failed to open UDP: %s", err)
					continue
				}
				newd.conn = conn
			} else {
				newd.conn = existing.conn
			}
		} else {
			raddr, err := net.ResolveUDPAddr("udp", td.Addr)
			if err != nil {
				log.Fatalf("failed to resolve thermo broadcast address: %s", err)
			}
			conn, err := net.DialUDP("udp", nil, raddr)
			if err != nil {
				log.Printf("Failed to open UDP: %s", err)
				continue
			}
			newd.conn = conn

			fdata, err := ioutil.ReadFile("./pos/" + td.Name + ".pos")
			if err == nil {
				pos := &Position{}
				json.Unmarshal(fdata, pos)
				newd.pos = *pos
			}
		}
		if newd.device.Thermostat != nil {
			te := refuge.TempEvent{
				Name:     newd.device.Name,
				Time:     time.Now(),
				Temp:     newd.device.Thermometer.Temp,
				Humidity: newd.device.Thermometer.Humidity,
				State:    newd.device.Thermostat.State,
			}
			enc.Encode(&te)
			srv.datalock.Lock()
			srv.eventData = append(srv.eventData, te)
			srv.datalock.Unlock()
		}
		// Update our cached thermostat
		srv.datalock.Lock()
		srv.Devices[strings.Replace(td.Name, " ", "", -1)] = newd
		srv.datalock.Unlock()
		deviceUpdates <- *td // push updates to alert system

		up := &DeviceUpdate{
			Device: &newd.device,
			Pos:    newd.pos,
		}
		// Serialize for clients
		d, err := json.Marshal(up)
		if err != nil {
			log.Printf("[Error] Failed to marshal thermal data to json: %s", err)
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
}

func weather() http.HandlerFunc {
	weather := []byte{}
	lastWeather := time.Now()
	var wmutex sync.RWMutex

	return func(w http.ResponseWriter, r *http.Request) {
		wmutex.RLock()
		if time.Now().Sub(lastWeather) < (time.Minute*15) && len(weather) > 0 {
			w.Write(weather)
			wmutex.RUnlock()
			return
		}
		wmutex.RUnlock()
		wmutex.Lock()
		defer wmutex.Unlock()

		resp, err := http.Get("http://wttr.in/Bozeman?format=4")
		if err != nil {
			log.Printf("[Error] Failed to get weather data: %v", err)
			w.Write(weather)
			return
		}
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[Error] Failed to get weather data: %v", err)
			w.Write(weather)
			return
		}

		resp.Body.Close()
		weather = data
		lastWeather = time.Now()
		w.Write(weather)
	}
}

func auth(w http.ResponseWriter, r *http.Request) int {
	addr := r.RemoteAddr
	if paddr := r.Header.Get("X-Echols-A"); paddr != "" {
		addr = paddr
	}

	// Allow intra-net access without auth.
	if !strings.HasPrefix(addr, "192.168.") && !strings.HasPrefix(addr, "127.0.0.1") && !strings.HasPrefix(addr, "[::1]") {
		log.Printf("Unauthed User: %s", addr)
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
