# sneaky-tunnel
This is reversed UDP tunnel, meaning the server will initiate the connection. The port on client side is opened using udp punch-holing.

## sample config.json for client
```json
{
  "role": "client",
  "listeningPorts": [1194],
  "serverIP": "1.2.3.4",
  "clientDelay": 3,
  "negotiators": [
    "http://rt.alirezasn.workers.dev"
  ]
}
```

## sample config.json for server
```json
{
  "role": "server"
}
```

### TODOs list
- [ ] add comments
- [ ] add traffic monitoring
- [x] add better error handling
- [ ] add https support for client requests
- [x] add systemd file and script
- [ ] add sample openvpn config for client and server
- [x] add golang install script
- [x] add log file
- [x] improve flag packet logic
- [ ] add diagram and explanation to readme
- [x] add receiver app port on server to the Packet struct
- [ ] add support for TCP over UDP
- [x] add bbr script
- [ ] add minimize to tray on windows
- [x] add better logging
- [x] add close connection packet from server
- [x] validate http request
- [x] check for server timeout at client side