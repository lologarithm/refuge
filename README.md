## Refuge
### Home Automation

This repo is for home automation systems. It is primarily designed around raspberry pi GPIO.


There are currently 3 primary binaries

1. cmd/refuge -- Central web server. Provide a --host=:XXXX to run the webserver. Web clients use a websocket to keep up to date. Web client will attempt to reconnect the socket. See './cmd/refuge/config.go' for configuration options. Loads from a file called 'config.json'
2. cmd/switch -- use name and a control pin. This is expected to be controlling a rely to turn a device on/off.
3. cmd/thermo -- used to control a thermostat. Expects a list of pins to control the heating/cooling sytem. Also expects a pin to read from a DHT22 temp/humidity sensor. Supports a motion detector pin as well to dynamically change temp.
4. cmd/garage -- used to control a garage but could be made more generic for any portal (doors/windows/etc). Currently just has 'current state' and the ability to set to open/closed. Need to add ability to lock/unlock to support house doors.

To build:

go build ./cmd/XXXX

To build a binary for use on a raspberry pi use env vars:
GOOS=linux
GOARCH=arm
GOARM=6 (for raspberry pi zero w)

Then you can upload the binary to the pi and run from there.

### Short TODO List
1. If we haven't heard from a device in X time, change it to 'inactive' and send a ping. If we still haven't heard from it after a reasonable timeout, remove the device from the list.
2. Upgrade from using JSON to netgen to improve perf on the poor little pi's.
3. Add config for location for weather.
4. Stats - nice to have graphs of the historical data.
