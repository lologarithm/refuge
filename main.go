package main

import (
	"fmt"

	"github.com/stianeikeland/go-rpio"
)

func main() {
	err := rpio.Open()
	if err != nil {
		fmt.Fatalf("Failed to open pins: %s\n", err)
	}

	pin := rpio.Pin(10)
	pin.Output()
}
