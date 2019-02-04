package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
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

func loadUserConfig() {
	users = map[string]userAccess{}
	data, err := ioutil.ReadFile("config.json")
	if err == nil {
		jerr := json.Unmarshal(data, &users)
		if jerr != nil {
			log.Printf("Failed to unmarshal: %#v", jerr)
		}
		log.Printf("User Access: %#v", users)
	} else {
		log.Printf("Failed to open config: %v", err)
	}
}
