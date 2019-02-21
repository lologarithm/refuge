package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"gitlab.com/lologarithm/refuge/climate"
	"gitlab.com/lologarithm/refuge/refuge"
)

var upgrader = websocket.Upgrader{} // use default options

// Request is sent from websocket client to server to request change to someting
type Request struct {
	Name    string            // Name of device to update
	Climate *climate.Settings // Climate Control Change Request
	Toggle  int               // Toggle of device request.
	Pos     *Position         // Request to change device position
}

// DeviceUpdate is a message to the client containing updated information about
// a particular device
type DeviceUpdate struct {
	*refuge.Device
	Pos Position
}

// Position of a device in the UI
type Position struct {
	X, Y   int
	RoomID string
}

func (srv *server) clientStreamHandler(w http.ResponseWriter, r *http.Request) {
	access := auth(w, r)
	if access == AccessNone {
		return
	}
	c := clientStream(w, r, access, srv)
	msgs := make([]*DeviceUpdate, 0, 10) // 10 seems like a reasonable number of devides.
	srv.datalock.Lock()
	for _, v := range srv.Devices {
		d := v.device
		msgs = append(msgs, &DeviceUpdate{Device: &d, Pos: v.pos})
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
			log.Printf("Got client Request: %#v", v)
			// Readers can't write new settings
			if access != AccessWrite {
				continue
			}
			dev := srv.getDevice(v.Name)
			if v.Pos != nil {
				d, _ := json.Marshal(v.Pos)
				ioutil.WriteFile("./pos/"+dev.device.Name+".pos", d, 0644)
				dev.pos = *v.Pos
			} else if v.Climate != nil {
				setTherm(*v.Climate, dev.conn)
			} else if v.Toggle > 0 {
				if dev.device.Switch != nil {
					toggleSwitch(v.Toggle, dev.conn)
				}
				if dev.device.Portal != nil {
					togglePortal(v.Toggle, dev.conn)
				}
			}
		}
		c.Close()
	}()
	return c
}
