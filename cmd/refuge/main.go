package main

import (
	"flag"
)

func main() {
	host := flag.String("host", ":80", "host:port to serve on")
	flag.Parse()
	// Launcher monitors and serves web host.
	serve(*host, monitor())
}
