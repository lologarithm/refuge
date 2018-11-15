package rnet

import (
	"log"
	"net"
	"strings"
)

var ThermoSpace = "225.1.2.3:8765"

type Thermostat struct {
	Name     string  // Name of thermostat
	Addr     string  // Address of thermostat
	Target   float32 // Targeted temp
	Temp     float32 // Last temp reading
	Humidity float32 // Last humidity reading
}

type Fireplace struct {
	Name string // Name of fireplace
	Addr string // Address of fireplace
	On   bool
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
