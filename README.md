## Refuge
### Home Automation

This repo is for home automation systems. It is primarily designed around raspberry pi GPIO.


There are currently 3 primary binaries

1. cmd/refuge -- Central web server. Provide a --host=:XXXX to run the webserver. Web clients use a websocket to keep up to date. There is currently no reconnection logic so a referesh is required to reconnect to the web interface.
2. cmd/fireplace -- use name and a control pin. This is expected to be controlling a rely to turn a fireplace on/off. This could probably be made more generic for anything turned on and off by a single relay (lights for example).
3. cmd/thermo -- used to control a thermostat. Expects a list of pins to control the heating/cooling sytem. Also expects a pin to read from a DHT22 temp/humidity sensor.

To build:

go build ./cmd/XXXX

To build a binary for use on a raspberry pi use env vars:
GOOS=linux
GOARCH=arm
GOARM=6 (for raspberry pi zero w)

Then you can upload the binary to the pi and run from there.