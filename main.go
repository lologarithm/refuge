package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
)

const (
	MinCollecting = 2 * time.Second
	maxWait       = int64(time.Millisecond) // 100
)

func main() {
	pinN := flag.Int("p", -1, "input pin to read")
	flag.Parse()
	if *pinN == -1 {
		fmt.Printf("no input pin set.")
		os.Exit(1)
	}
	err := rpio.Open()
	if err != nil {
		fmt.Printf("Failed to open pins: %s\n", err)
		os.Exit(1)
	}

	pin := rpio.Pin(22)

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

	fmt.Printf("T: %f, H: %f\n", t, h)
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
			panic("DHT never pulled low after we did.")
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

	// fmt.Printf("Completed read: %#v\n", pulseLen)
	if pulseLen[66] == 0 {
		// fmt.Printf("missing data, returning early.\n")
		return -1, -1, false
	}

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
