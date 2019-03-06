package main

import (
	"flag"
)

func main() {
	host := flag.String("host", ":80", "host:port to serve on")
	test := flag.Bool("test", false, "use fake test data so no network is needed.")
	flag.Parse()

	// Setup user access
	loadUserConfig()

	// Launcher monitors and serves web host.
	serve(*host, monitor(*test))
}
