package sensor

import (
	"fmt"
	"time"

	rpio "github.com/stianeikeland/go-rpio/v4"
)

// Motion accepts a pin to read on and a stream to write on.
// Streams the int64 unix time of a motion event.
// Close the stream to stop reading
func Motion(p int, stream chan int64) {
	pin := rpio.Pin(p)
	lr := rpio.Low
	ltime := time.Now().Unix()
	for {
		v := pin.Read()
		if lr != v {
			lr = v
			if lr == rpio.High {
				ltime = time.Now().Unix()
			}
			fmt.Printf("Reading changed: %d, last motion: %s", lr, time.Unix(ltime, 0).Format("Jan _2 15:04:05"))
			select {
			case stream <- ltime:
			default:
				return // bad, exit
			}
		}
		time.Sleep(time.Second) // read from motion sensor once per second
	}
}
