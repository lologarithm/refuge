package rnet

import (
	"net"
	"time"

	"github.com/lologarithm/netgen/lib/ngservice"
)

type Listener struct {
	Addr     *net.UDPAddr `ngen:"-"`
	AddrStr  string
	LastPing int64
}

// BroadcastAndTimeout will broadcast the given msg bytes to all listener UDPAddr via the given udp conn.
// Any listeners who have been idle for over 10min will be removed.
func BroadcastAndTimeout(conn *net.UDPConn, msg []byte, listeners []Listener) []Listener {
	now := time.Now().Unix()
	toremove := []int{}
	for i, l := range listeners {
		if now-l.LastPing > 600 { // 10 minutes
			toremove = append(toremove, i)
			continue
		}
		conn.WriteToUDP(msg, l.Addr)
	}
	// remove the dead listeners
	for i := len(toremove) - 1; i > -1; i-- {
		idx := toremove[i]
		copy(listeners[idx:], listeners[idx+1:]) // copy back
		listeners = listeners[:len(listeners)-1] // slice off end
	}
	return listeners
}

func UpdateListeners(listeners []Listener, addr *net.UDPAddr) []Listener {
	addrStr := addr.String()
	found := false
	for _, l := range listeners {
		if l.AddrStr == addrStr {
			l.LastPing = time.Now().Unix()
			found = true
		}
	}
	if !found {
		listeners = append(listeners, Listener{Addr: addr, LastPing: time.Now().Unix(), AddrStr: addrStr})
	}
	return listeners
}

// ReadBroadcastPing will attempt to read a ping message from given connection
// with a timeout of 10 milliseconds
func ReadBroadcastPing(conn *net.UDPConn, b []byte) (Ping, *net.UDPAddr) {
	conn.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	n, remoteAddr, _ := conn.ReadFromUDP(b)
	if n > 0 {
		packet, ok := ngservice.ReadPacket(Context, b[:n])
		if ok && packet.Header.MsgType == PingMsgType {
			return *packet.NetMsg.(*Ping), remoteAddr
		}
	}
	return Ping{}, nil
}
