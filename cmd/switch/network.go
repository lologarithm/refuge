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

	b := make([]byte, 256)
	return func() int {
		broadcasts.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		n, remoteAddr, _ := broadcasts.ReadFromUDP(b)
		if n > 0 {
			fmt.Printf("Got message on discovery(%s), updating status: %s", rnet.RefugeDiscovery, string(msg))
			rnet.UpdateListeners(listeners, remoteAddr)
			// Emit current state to pinger.
			direct.WriteToUDP(msg, remoteAddr)
		}

		requestedState := 0
		direct.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
		n, remoteAddr, _ = direct.ReadFromUDP(b)
		if n > 0 {
			packet, ok := ngservice.ReadPacket(refuge.Context, b[:n])
			if ok && packet.Header.MsgType == refuge.SwitchMsgType {
				settings := packet.NetMsg.(*refuge.Switch)
				requestedState = 1
				if settings.On == true {
					requestedState = 2
				}
				fmt.Printf("Newly requested state: %d\n", requestedState)
			}
			listeners = rnet.UpdateListeners(listeners, remoteAddr)
			msg = ngservice.WriteMessage(rnet.Context, &rnet.Msg{Device: state})
			rnet.BroadcastAndTimeout(direct, msg, listeners)
		}
		return requestedState
	}
}
