# Tesla Hermes Signaling Protocol
Tesla uses a backend messaging service called Hermes. Both the Vehicle and the Tesla App are connected to Hermes via a WebSocket to exchange messages.

This is a reverse engineered implementation of the Tesla Hermes Signaling Protocol as used in the Tesla App, which was possible by intercepting and decrypting network traffic and decompiling the Android apk.

Current scope is to be able to send commands to the vehicle, as the RESTful APIs are [getting disabled soon](https://developer.tesla.com/docs/fleet-api#2023-11-17-rest-api-vehicle-commands-endpoint-deprecation-timeline-action-required). Other parts of the Hermes Protocol have yet to be discovered.

main.go is just a demonstration and not intended as a versatile user-friendly cli. Feel free to re-use the hermes package in your own project.

# Usage
Generate a private key with [tesla-keygen](https://github.com/teslamotors/vehicle-command/tree/main/cmd/tesla-keygen) and save a valid owner api access_token to a file.
```bash
go run ./ -key <filename> -vin <your vin> --ownerApiToken <filename>
```
