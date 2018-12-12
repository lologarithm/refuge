package rnet

import (
	"log"
	"net"
	"strings"

	"gitlab.com/lologarithm/refuge/climate"
)

// RefugeMessages is the multicast address used by devices to communicate to web server.
var RefugeMessages *net.UDPAddr
var refugeMessages = "225.1.2.3:8765"

// RefugeDiscovery is the multicast address used by webserver to discover devices.
var RefugeDiscovery *net.UDPAddr
var refugeDiscovery = "225.1.2.3:8766"

func init() {
	var err error
	RefugeMessages, err = net.ResolveUDPAddr("udp", refugeMessages)
	failErr("resolving refuge udp addr", err)
	RefugeDiscovery, err = net.ResolveUDPAddr("udp", refugeDiscovery)
	failErr("resolving refuge udp addr", err)
}

// Portal represents any door/window that can be monitored or open/closed
type Portal struct {
	Name string
	Addr string

	State PortalState // Can signal current state or intended state. Unknown, Closed, Open
}

// PortalState is the state of the portal (open/closed)
type PortalState uint64

// Enum of portal states
const (
	PortalStateUnknown PortalState = iota
	PortalStateClosed
	PortalStateOpen
)

// Thermostat is a very specific device -- a thermostat
// Includes current reading as well as temp targets
type Thermostat struct {
	Name string // Name of thermostat
	Addr string // Address of thermostat

	State climate.ControlState

	// Settings
	High float32 // Temp Max
	Low  float32 // Temp Minimum
	Fan  uint8   // If Fan is unset, off, on, or auto

	// Readings
	Temp     float32 // Last temp reading
	Humidity float32 // Last humidity reading
	Motion   int64   // Last motion event
}

// Switch represents any devices that can be switched on/off
// Examples: Lights, Gas Fireplace, etc
type Switch struct {
	Name string // Name of fireplace
	Addr string // Address of fireplace
	On   bool
}

// Msg is what is sent over the broadcast network
// TODO: make this a device message and put name on just the main struct
// then any device can send any readings it wants. (sensors, etc)
type Msg struct {
	Switch     *Switch
	Thermostat *Thermostat
	Portal     *Portal
}

// Ping is a request for discovery of devices
type Ping struct{}

func failErr(ctx string, e error) {
	if e != nil {
		log.Fatalf("Failed to %s: %s", ctx, e)
	}
}

func MyIPs() (mine []string) {
	itfs, err := net.Interfaces()
	failErr("get network interfaces", err)

	for _, itf := range itfs {
		switch {
		case itf.Flags&net.FlagUp != net.FlagUp:
			continue // skip down interfaces
		case itf.Flags&net.FlagLoopback == net.FlagLoopback:
			continue // skip loopbacks
		case itf.HardwareAddr == nil:
			continue // not real network hardware
		case strings.Contains(itf.Name, "docker"):
			continue // ignore docker network
		}
		if multi, err := itf.MulticastAddrs(); err != nil {
			log.Fatal("cant get the IPs MulticastAddress", err)
		} else if len(multi) == 0 {
			continue // no multicast
		}
		addrs, err := itf.Addrs()
		failErr("get addrs", err)

		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			failErr("parse cidr", err)

			ipv4 := ip.To4()
			if ipv4 == nil {
				continue // skip non-ipv4 addrs
			}
			mine = append(mine, ipv4.String())
		}
	}
	return mine
}
