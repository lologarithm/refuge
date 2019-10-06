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

var idleTimeout = int64(time.Duration(time.Minute * 30).Seconds())

// BroadcastAndTimeout will broadcast the given msg bytes to all listener UDPAddr via the given udp conn.
// Any listeners who have been idle for over "idleTimeout" seconds will be removed.
func BroadcastAndTimeout(conn *net.UDPConn, msg []byte, listeners []Listener) []Listener {
	now := time.Now().Unix()
	n := 0
	for _, lsn := range listeners {
		if now-lsn.LastPing > idleTimeout {
			continue // Expired listener, will be removed.
		}
		listeners[n] = lsn
		n++
		conn.WriteToUDP(msg, lsn.Addr)
	}
	return listeners[:n]
}

// UpdateListeners will either find the listener with the same source address
// and update the last ping time OR append the listener as a new listener if
// it hasn't been seen before.
// This lets us track when we last heard from a listener so we can expire servers that aren't around anymore.
func UpdateListeners(listeners []Listener, addr *net.UDPAddr) []Listener {
	addrStr := addr.String()
	for _, l := range listeners {
		if l.AddrStr == addrStr {
			l.LastPing = time.Now().Unix()
			return listeners // updated last ping, return now.
		}
	}
	// Haven't seen this listener before, append to list and return
	return append(listeners, Listener{Addr: addr, LastPing: time.Now().Unix(), AddrStr: addrStr})
}

// ReadBroadcastPing will attempt to read a ping message from given connection
// with a timeout of 10 milliseconds. On success and if the ping requests a response, return response bytes.
func ReadBroadcastPing(conn *net.UDPConn, listeners []Listener, b []byte, response []byte) []Listener {
	conn.SetReadDeadline(time.Now().Add(time.Millisecond * 10))
	n, remoteAddr, _ := conn.ReadFromUDP(b)
	if n <= 0 {
		return listeners
	}
	packet, ok := ngservice.ReadPacket(Context, b[:n])
	if !ok || packet.Header.MsgType != PingMsgType {
		return listeners
	}
	if ping := (packet.NetMsg.(*Ping)); !ping.Respond {
		return listeners
	}
	// We got a request to broadcast latest state.
	return BroadcastAndTimeout(conn, response, UpdateListeners(listeners, remoteAddr))
}
