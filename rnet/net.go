package rnet

import (
	"log"
	"net"
	"strings"

	"gitlab.com/lologarithm/refuge/refuge"
)

// RefugeDiscovery is the multicast address used by webserver to discover devices.
var RefugeDiscovery *net.UDPAddr
var refugeDiscovery = "225.1.2.3:8778"

func init() {
	var err error
	RefugeDiscovery, err = net.ResolveUDPAddr("udp", refugeDiscovery)
	failErr("resolving refuge udp addr", err)
}

// Msg is what is sent over the broadcast network
type Msg struct {
	*refuge.Device // The device this message is about
}

// Ping is a request for discovery of devices
type Ping struct {
	Respond bool
}

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
