package sensor

import (
	"fmt"
	"runtime/debug"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
)

const (
	maxWait = int64(time.Millisecond) // 100us
)

// Measurement holds a sensor measurement.
type Measurement struct {
	Temp float32
	Humi float32
	Time time.Time
}

// Stream accepts a pin to read on, how often to read, and a stream to write results to.
// Close the stream to stop measurements.
func Stream(p int, measureInterval time.Duration, stream chan Measurement) {
	err := rpio.Open()
	if err != nil {
		fmt.Printf("Unable to open raspberry pi gpio pins: %s\n-----  Defaulting to use fake data.  -----\n", err)
		// send fake data!
		go func() {
			for {
				select {
				case stream <- Measurement{Temp: 20, Humi: 50, Time: time.Now()}:
				default:
					return // bad, exit
				}
				time.Sleep(measureInterval)
			}
		}()
		return
	}
	pin := rpio.Pin(p)
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
			case stream <- Measurement{Temp: t, Humi: h, Time: time.Now()}:
			default:
				return // bad, exit
			}
			time.Sleep(measureInterval)
		}
	}()
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
		s = 0
		end = 0
		// read low pulseLen
		for pin.Read() == rpio.Low {
			if end-s > maxWait {
				break READER
			}
			end++
		}
		pulseLen[i] = end - s

		s = 0
		end = 0
		// read high pulse length
		for pin.Read() == rpio.High {
			if end-s > maxWait {
				break READER
			}
			end++
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
