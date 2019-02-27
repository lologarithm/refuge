package main

import (
	"encoding/json"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
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
	datalock *sync.Mutex
	Devices  map[string]*refugeDevice

	clientslock   *sync.Mutex
	clientStreams []*websocket.Conn
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

type refugeDevice struct {
	device refuge.Device
	conn   *net.UDPConn
	pos    Position
}

// serve creates the state object "server" and http handlers and launches the http listener.
// Blocks on the http listener.
func serve(host string, deviceStream chan rnet.Msg) {
	// localTime := time.Location{}
	srv := &server{
		datalock:    &sync.Mutex{},
		Devices:     map[string]*refugeDevice{},
		clientslock: &sync.Mutex{},
	}

	deviceUpdates := make(chan refuge.Device, 5)
	go portalAlert(&globalConfig, deviceUpdates)

	// Updater goroutine. Updates data state and pushes the new state to websocket clients
	go func() {
		for {
			msg := <-deviceStream
			td := msg.Device
			existing := srv.getDevice(td.Name)
			new := &refugeDevice{
				device: *td,
			}
			if existing != nil {
				new.pos = existing.pos
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
					new.conn = conn
				} else {
					new.conn = existing.conn
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
				new.conn = conn

				fdata, err := ioutil.ReadFile("./pos/" + td.Name + ".pos")
				if err == nil {
					pos := &Position{}
					json.Unmarshal(fdata, pos)
					new.pos = *pos
				}
			}
			// Update our cached thermostat
			srv.datalock.Lock()
			srv.Devices[strings.Replace(td.Name, " ", "", -1)] = new
			srv.datalock.Unlock()
			deviceUpdates <- *td // push updates to alert system

			up := &DeviceUpdate{
				Device: &new.device,
				Pos:    new.pos,
			}
			// Serialize for clients
			d, err := json.Marshal(up)
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

	weather := []byte{}
	lastWeather := time.Now()
	var wmutex sync.RWMutex
	http.HandleFunc("/weather", func(w http.ResponseWriter, r *http.Request) {
		wmutex.RLock()
		if time.Now().Sub(lastWeather) < time.Minute && len(weather) > 0 {
			w.Write(weather)
			wmutex.RUnlock()
			return
		}
		wmutex.RUnlock()
		wmutex.Lock()
		resp, err := http.Get("http://wttr.in/?format=4")
		if err != nil {
			log.Printf("failed to get weather data: %v", err)
		}
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("failed to get weather data: %v", err)
		}
		resp.Body.Close()
		weather = data
		lastWeather = time.Now()
		w.Write(weather)
		wmutex.Unlock()
	})
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
