package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
)

const (
	maxWait = int64(time.Millisecond) // 100us
)

func main() {
	pinN := flag.Int("p", -1, "input pin to read")
	flag.Parse()
	if *pinN == -1 {
		fmt.Printf("no input pin set.")
		os.Exit(1)
	}
	stream := runTherm(*pinN)
	mutex := sync.Mutex{}
	data := make([]env, 1440)
	var latest env
	i := 0
	go func() {
		for d := range stream {
			// fmt.Printf("Temp: %.1fC(%dF) Humidity: %.1f%%  I:%d\n", d.temp, int(d.temp*9/5)+32, d.hum, i)
			mutex.Lock()
			latest = d
			data[i] = d
			i++
			if i == len(data) {
				i = 0
			}
			mutex.Unlock()
		}
		fmt.Printf("exiting data processor")
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		d := latest
		mutex.Unlock()
		w.Write([]byte(fmt.Sprintf("<html><body style=\"font-size: 2em;\"><p><b>At:</b> %s</p><p><b>Temp:</b> %.1fC(%dF)</p><p><b>Humidity:</b> %.1f%%</p></html></body>", d.t.Format("Mon Jan _2 15:04:05 2006"), d.temp, int(d.temp*9/5)+32, d.hum)))
	})

	err := http.ListenAndServe(":80", nil)
	if err != nil {
		log.Fatal(err)
	}
}

type env struct {
	temp float32
	hum  float32
	t    time.Time
}

func runTherm(p int) chan env {
	err := rpio.Open()
	if err != nil {
		fmt.Printf("Failed to open pins: %s\n", err)
		os.Exit(1)
	}

	pin := rpio.Pin(p)

	stream := make(chan env, 10)
	go func() {
		for {
			var t, h float32
			var csg bool
			debug.SetGCPercent(-1)
			for i := 0; i < 10; i++ {
				t, h, csg = readDHT22(pin)
				if csg {
					break
				}
			}
			debug.SetGCPercent(100)
			// fmt.Printf("Temp: %.1fC(%dF) Humidity: %.1f%%\n", t, int(t*9/5)+32, h)
			select {
			case stream <- env{temp: t, hum: h, t: time.Now()}:
				// good!
			default:
				return // bad, exit
			}
			time.Sleep(time.Second * 60)
		}
	}()
	return stream
}

func readDHT22(pin rpio.Pin) (float32, float32, bool) {
	// early allocations before time critical code
	pulseLen := make([]int64, 82)

	time.Sleep(1700 * time.Millisecond)
	pin.Mode(rpio.Output)
	pin.High()

	// send init values
	time.Sleep(400 * time.Millisecond)
	pin.Low()

	// spinlock for milliseconds while pin is low.
	// this signals the request for reading
	s := time.Now().UnixNano()
	to := int64(time.Millisecond * 20)
	for time.Now().UnixNano()-s < to {
	}
	pin.Mode(rpio.Input)
	pin.PullUp()

	// now we wait for DHT to pull low
	s = time.Now().UnixNano()
	firstWaitMax := int64(time.Millisecond * 5)
	for pin.Read() == rpio.High {
		if time.Now().UnixNano()-s > firstWaitMax {
			return -1, -1, false // DHT never pulled low... probably retry
		}
	}

	// DHT pulls low for 80us and then 80us to signal its starting
	// After that we read 40 low and 40 high pulses.
	var end int64
READER:
	for i := 0; i < 81; i += 2 {
		s = time.Now().UnixNano()
		end = time.Now().UnixNano()
		// read low pulseLen
		for pin.Read() == rpio.Low {
			if end-s > maxWait {
				break READER
			}
			end = time.Now().UnixNano()
		}
		pulseLen[i] = end - s

		s = time.Now().UnixNano()
		end = time.Now().UnixNano()
		// read high pulse length
		for pin.Read() == rpio.High {
			if end-s > maxWait {
				break READER
			}
			end = time.Now().UnixNano()
		}
		pulseLen[i+1] = end - s
	}
	pin.PullOff()

	var threshold int64
	for i := 2; i < 82; i += 2 {
		threshold += pulseLen[i]
	}
	threshold /= 40

	// convert to bytes
	bytes := make([]uint8, 5)

	for i := 3; i < 82; i += 2 {
		bi := (i - 3) / 16
		bytes[bi] <<= 1
		if pulseLen[i] > threshold {
			bytes[bi] |= 0x01
		}
	}

	// calculate humidity
	humidity := float32(uint16(bytes[0])*256+uint16(bytes[1])) / 10.0
	// calculate temperature
	temperature := float32((uint16(bytes[2])&0x7F)*256+uint16(bytes[3])) / 10.0
	// check for negative temperature
	if uint16(bytes[2])&0x80 > 0 {
		temperature *= -1
	}
	return temperature, humidity, checksum(bytes)
}

func checksum(bytes []uint8) bool {
	var sum uint8
	for i := 0; i < 4; i++ {
		sum += bytes[i]
	}
	return sum == bytes[4]
}
