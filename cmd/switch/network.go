package main

import (
	"fmt"
	"time"

	"github.com/lologarithm/netgen/lib/ngservice"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
)

// setupNetwork returns a function that will poll for both broadcast requests
// and direct requests to toggle the switch. If a request is found, the new toggle state is returned.
func setupNetwork(name string) func() int {
	// Open UDP connection to a local addr/port.
	direct, broadcasts := rnet.SetupUDPConns()
	listeners := []rnet.Listener{}
	state := &refuge.Device{Switch: &refuge.Switch{}, Name: name, Addr: direct.LocalAddr().String()}
	msg := ngservice.WriteMessage(rnet.Context, &rnet.Msg{Device: state})

	b := make([]byte, 256)
	return func() int {
		// Check for broadcast pings
		listeners = rnet.ReadBroadcastPing(broadcasts, listeners, b, msg)

		requestedState := 0
		direct.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		n, remoteAddr, _ := direct.ReadFromUDP(b)
		if n > 0 {
			packet, ok := ngservice.ReadPacket(refuge.Context, b[:n])
			if ok && packet.Header.MsgType == refuge.SwitchMsgType {
				settings := packet.NetMsg.(*refuge.Switch)
				requestedState = 1
				if settings.On == true {
					requestedState = 2
				}
				fmt.Printf("Newly requested state: %d\n", requestedState)
				state.Switch.On = settings.On
			} else if packet.Header.MsgType == rnet.PingMsgType {
				// Just letting us know to respond to them now.
			}
			msg = ngservice.WriteMessage(rnet.Context, &rnet.Msg{Device: state})
			listeners = rnet.BroadcastAndTimeout(direct, msg, rnet.UpdateListeners(listeners, remoteAddr))
		}
		return requestedState
	}
}
