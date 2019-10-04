package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/lologarithm/netgen/lib/ngservice"
	"gitlab.com/lologarithm/refuge/refuge"
	"gitlab.com/lologarithm/refuge/rnet"
)

func myUDPConn() *net.UDPConn {
	addrs := rnet.MyIPs()
	fmt.Printf("MyAddrs: %#v\n", addrs)

	addr, err := net.ResolveUDPAddr("udp", addrs[0]+":0")
	if err != nil {
		fmt.Printf("Failed to resolve udp: %s", err)
		os.Exit(1)
	}
	// Listen to directed udp messages
	direct, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Printf("Failed to listen to udp: %s", err)
		os.Exit(1)
	}
	return direct
}

func setupNetwork(name string) func(refuge.PortalState) refuge.PortalState {
	// Open UDP connection to a local addr/port.
	direct := myUDPConn()
	fmt.Printf("Listening on: %s\n", direct.LocalAddr().String())

	broadcasts, err := net.ListenMulticastUDP("udp", nil, rnet.RefugeDiscovery)
	if err != nil {
		fmt.Printf("failed to listen to thermo broadcast address: %s\n", err)
		os.Exit(1)
	}

	listeners := []rnet.Listener{}
	state := &refuge.Device{Portal: &refuge.Portal{}, Name: name, Addr: direct.LocalAddr().String()}
	msg := ngservice.WriteMessage(rnet.Context, &rnet.Msg{Device: state})

	// Ping the network to say we are online
	direct.WriteToUDP(ngservice.WriteMessage(rnet.Context, &rnet.Ping{Respond: false}), rnet.RefugeDiscovery)

	b := make([]byte, 256)
	return func(newState refuge.PortalState) refuge.PortalState {
		if newState != state.Portal.State {
			state.Portal.State = newState
			fmt.Printf("Broadcasting new state: %#v\n", state.Portal)
			msg = ngservice.WriteMessage(rnet.Context, &rnet.Msg{Device: state})
			rnet.BroadcastAndTimeout(direct, msg, listeners)
		}

		ping, remoteAddr := rnet.ReadBroadcastPing(broadcasts, b)
		if ping.Respond {
			listeners = rnet.UpdateListeners(listeners, remoteAddr)
			direct.WriteToUDP(msg, remoteAddr) // Emit current state to pinger.
		}

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
				fmt.Printf("Got Direct Message, adding to listeners... %v", remoteAddr)
				direct.WriteToUDP(msg, remoteAddr)
			}
			listeners = rnet.UpdateListeners(listeners, remoteAddr)
		}
		return requestedState
	}
}
