package main

import (
	"encoding/gob"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gitlab.com/lologarithm/refuge/refuge"
)

// LoadStats will load all stats from given disk location.
func LoadStats(dir string) []refuge.TempEvent {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		os.Mkdir("stats", os.ModePerm)
		return nil
	}
	gob.Register(refuge.TempEvent{})

	// Load historical data
	events := []refuge.TempEvent{}
	for _, fi := range files {
		name := fi.Name()
		if !strings.HasPrefix(name, "rs_") {
			continue
		}
		file, err := os.OpenFile("./stats/"+name, os.O_RDONLY, os.ModePerm)
		if err != nil {
			log.Printf("Failed to open existing stats file: %s", err)
			continue
		}
		for gdec := gob.NewDecoder(file); err == nil; {
			var e refuge.TempEvent
			err = gdec.Decode(&e)
			if err != nil {
				if err != io.EOF {
					log.Printf("[Error] Failed to deserialize statistics data: %s", err)
				}
			} else {
				events = append(events, e)
			}
		}
	}
	return events
}

func getStatsFile(when time.Time, dir string) *os.File {
	todayUnix := strconv.FormatInt(when.Unix(), 10)
	statFile, err := os.OpenFile(filepath.Join(dir, "/rs_"+todayUnix), os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModePerm)
	if err != nil {
		log.Printf("Failed to open existing stats file: %s", err)
	}
	return statFile
}
