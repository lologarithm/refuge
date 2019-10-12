package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
)

var globalConfig Config

// Config is server configuration.
// Includes users&access levels as well as Mailgun config to send warning emails.
type Config struct {
	Users    map[string]userAccess
	Mailgun  MailgunConfig
	StatsDir string
}

// MailgunConfig is the settings needed to use Mailgun for emails.
type MailgunConfig struct {
	APIKey     string
	Domain     string
	Sender     string
	Recipients []string
}

func loadUserConfig() {
	globalConfig = Config{
		Users:    map[string]userAccess{},
		StatsDir: "./stats",
	}
	data, err := ioutil.ReadFile("config.json")
	if err == nil {
		jerr := json.Unmarshal(data, &globalConfig)
		if jerr != nil {
			log.Printf("Failed to unmarshal: %#v", jerr)
		}
		for name, v := range globalConfig.Users {
			log.Printf("User: %s, Access: %d", name, v.Access)
		}
		log.Printf("Mailgun Config:\n\t%#v", globalConfig.Mailgun)
	} else {
		log.Printf("Failed to open config: %v", err)
	}
}
