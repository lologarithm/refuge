## Refuge
### Home Automation

This repo is for home automation systems. It is primarily designed around raspberry pi GPIO.


There are currently 3 primary binaries

1. cmd/refuge -- Central web server. Provide a --host=:XXXX to run the webserver. Web clients use a websocket to keep up to date. Web client will attempt to reconnect the socket.
2. cmd/fireplace -- use name and a control pin. This is expected to be controlling a rely to turn a fireplace on/off. This could probably be made more generic for anything turned on and off by a single relay (lights for example).
3. cmd/thermo -- used to control a thermostat. Expects a list of pins to control the heating/cooling sytem. Also expects a pin to read from a DHT22 temp/humidity sensor.

To build:

go build ./cmd/XXXX

To build a binary for use on a raspberry pi use env vars:
GOOS=linux
GOARCH=arm
GOARM=6 (for raspberry pi zero w)

Then you can upload the binary to the pi and run from there.

### TODO
1. If we haven't heard from a device in X time, change it to 'inactive' and send a ping. If we still haven't heard from it after a reasonable timeout, remove the device from the list.
2. Motion sensors on the Thermostats
3. Upgrade from using JSON to netgen to improve perf on the poor little pi's.

### Big Plans
1. Probably renamed rnet to some kind of model. This is becoming the definition of device types and how they communicate with central refuge webserver.
2. Generalize devices in the rnet/model package. We can probably start to generalize a device to have sensors and controls. Then have different kinds of those devices.
3. With the generalization of devices we might be able to then also generalize the web server and interface to handle more generic features
4. Replace current web interface from being a list of devices to instead being a map. Then we can define 'rooms' or spaces and what devices are found in each room. This should make it easier to figure out what sensor data is coming from where!
