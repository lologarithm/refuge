package main

import (
	"fmt"
	"time"

	"github.com/lologarithm/netgen/lib/ngservice"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
)

func setupNetwork(name string) func(refuge.PortalState) refuge.PortalState {
	// Open UDP connection to a local addr/port.
	direct, broadcasts := rnet.SetupUDPConns()
	listeners := []rnet.Listener{}
	state := &refuge.Device{Portal: &refuge.Portal{}, Name: name, Addr: direct.LocalAddr().String()}
	msg := ngservice.WriteMessage(rnet.Context, &rnet.Msg{Device: state})

	b := make([]byte, 256)
	return func(newState refuge.PortalState) refuge.PortalState {
		if newState != state.Portal.State {
			state.Portal.State = newState
			fmt.Printf("Broadcasting new state: %#v\n", state.Portal)
			msg = ngservice.WriteMessage(rnet.Context, &rnet.Msg{Device: state})
			listeners = rnet.BroadcastAndTimeout(direct, msg, listeners)
			fmt.Printf("Listeners: %#v\n", listeners)
		}

		// Check for broadcast pings
		listeners = rnet.ReadBroadcastPing(broadcasts, listeners, b, msg)

		requestedState := refuge.PortalStateUnknown
		direct.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		n, remoteAddr, _ := direct.ReadFromUDP(b)
		if n > 0 {
			packet, ok := ngservice.ReadPacket(refuge.Context, b[:n])
			if ok && packet.Header.MsgType == refuge.PortalMsgType {
				settings := packet.NetMsg.(*refuge.Portal)
				requestedState = settings.State
				fmt.Printf("Newly requested state: %d\n", requestedState)
			} else if packet.Header.MsgType == rnet.PingMsgType {
				// Just letting us know to respond to them now.
				direct.WriteToUDP(msg, remoteAddr)
			} else {
				fmt.Printf("Got message of unknown type: %#v (%#v)", packet, b[:n])
			}
			listeners = rnet.UpdateListeners(listeners, remoteAddr)
			fmt.Printf("Listeners: %#v\n", listeners)
		}
		return requestedState
	}
}
