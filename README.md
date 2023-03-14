# sneaky-tunnel
This is reversed UDP tunnel, meaning the server will initiate the connection. The port on client side is opened using udp punch-holing.

## sample config.json for client
```json
{
  "role": "client",
  "listenOn": "0.0.0.0:1194",
  "server": "1.2.3.4",
  "clientPort": "12345",
  "negotiators": [
    "http://reverse-tunnel.netlify.app",
    "http://rt.alirezasn.workers.dev"
  ]
}
```

## sample config.json for server
```json
{
  "role": "server",
  "connectTo": "0.0.0.0:1194"
}
```