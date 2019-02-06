package main

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/rnet"
)

var upgrader = websocket.Upgrader{} // use default options

// Request is sent from websocket client to server to request change to someting
type Request struct {
	Climate *ClimateChange
	Switch  *rnet.Switch
	Portal  *rnet.Portal
	Auth    map[string]string
}

// ClimateChange is a climate change request from websocket client
type ClimateChange struct {
	climate.Settings
	Name string // name of thermo to change
}

func (srv *server) clientStreamHandler(w http.ResponseWriter, r *http.Request) {
	access := auth(w, r)
	if access == AccessNone {
		return
	}
	c := clientStream(w, r, access, srv)
	msgs := []*rnet.Msg{}
	srv.datalock.Lock()
	for _, v := range srv.Switches {
		msgs = append(msgs, &rnet.Msg{Switch: &v})
	}
	for _, v := range srv.Thermostats {
		msgs = append(msgs, &rnet.Msg{Thermostat: &v})
	}
	for _, v := range srv.Portals {
		msgs = append(msgs, &rnet.Msg{Portal: &v})
	}
	srv.datalock.Unlock()
	for _, msg := range msgs {
		c.WriteJSON(msg)
	}
	srv.clientslock.Lock()
	srv.clientStreams = append(srv.clientStreams, c)
	srv.clientslock.Unlock()
}

func clientStream(w http.ResponseWriter, r *http.Request, access int, srv *server) *websocket.Conn {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade failure:", err)
		return nil
	}

	// websocket reader closure.
	// Handles requests from websocket client.
	go func() {
		for {
			v := &Request{}
			err := c.ReadJSON(v)
			if err != nil {
				log.Println("Disconnecting user: ", err)
				break
			}
			// Readers can't write new settings
			if access != AccessWrite {
				continue
			}
			if v.Climate != nil {
				setTherm(*v.Climate, srv.getThermo(v.Climate.Name))
			}
			if v.Switch != nil {
				toggleSwitch(srv.getSwitch(v.Switch.Name))
			}
			if v.Portal != nil {
				togglePortal(srv.getPortal(v.Portal.Name))
			}
		}
		c.Close()
	}()
	return c
}
