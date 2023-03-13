# sneaky-tunnel
This is reversed UDP tunnel, meaning the server will initiate the connection. The port on client side is opened using udp punch-holing.

## sample config.json for client
```
{
  "role": "client",
  "listenOn": "0.0.0.0:1194",
  "server": "5.78.79.102",
  "negotiators": [
    "http://reverse-tunnel.netlify.app",
    "http://rt.alirezasn.workers.dev"
  ]
}
```

## sample config.json for server
```
{
  "role": "server",
  "connectTo": "0.0.0.0:1194"
}
```