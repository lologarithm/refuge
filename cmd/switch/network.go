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
	fmt.Printf("MyAddrs: %#v", addrs)

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

func setupNetwork(name string) func() int {
	// Open UDP connection to a local addr/port.
	direct := myUDPConn()
	fmt.Printf("Listening on: %s", direct.LocalAddr().String())

	broadcasts, err := net.ListenMulticastUDP("udp", nil, rnet.RefugeDiscovery)
	if err != nil {
		fmt.Printf("failed to listen to thermo broadcast address: %s", err)
		os.Exit(1)
	}

	listeners := []rnet.Listener{}
	state := &refuge.Device{Switch: &refuge.Switch{}, Name: name, Addr: direct.LocalAddr().String()}
	msg := ngservice.WriteMessage(rnet.Context, &rnet.Msg{Device: state})

	// Ping the network to say we are online
	direct.WriteToUDP(ngservice.WriteMessage(rnet.Context, &rnet.Ping{Respond: false}), rnet.RefugeDiscovery)

	b := make([]byte, 256)
	return func() int {
		ping, remoteAddr := rnet.ReadBroadcastPing(broadcasts, b)
		if ping.Respond {
			listeners = rnet.UpdateListeners(listeners, remoteAddr)
			direct.WriteToUDP(msg, remoteAddr) // Emit current state to pinger.
		}

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
				fmt.Printf("Got Direct Message, adding to listeners... %v", remoteAddr)
			}
			listeners = rnet.UpdateListeners(listeners, remoteAddr)
			msg = ngservice.WriteMessage(rnet.Context, &rnet.Msg{Device: state})
			rnet.BroadcastAndTimeout(direct, msg, listeners)
		}
		return requestedState
	}
}
