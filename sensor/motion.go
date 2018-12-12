package sensor

import (
	"fmt"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
)

// Motion accepts a pin to read on and a stream to write on.
// Streams the int64 unix time of a motion event.
// Close the stream to stop reading
func Motion(p int, stream chan int64) {
	pin := rpio.Pin(p)
	lr := rpio.Low      // last reading
	ltime := time.Now() // last time motion was detected
	for {
		v := pin.Read()
		if lr != v {
			lr = v
			if lr == rpio.High {
				ltime = time.Now()
			}
			fmt.Printf("Reading changed: %d, last motion: %s\n", lr, ltime.Format("Jan _2 15:04:05 MST"))
			select {
			case stream <- ltime.Unix():
			default:
				return // bad, exit
			}
		}
		time.Sleep(time.Second) // read from motion sensor once per second
	}
}

// ReadMotion will check if the pin has recently seen motion.
func ReadMotion(pin rpio.Pin) bool {
	return pin.Read() == rpio.High
}
