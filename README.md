# sneaky-tunnel
This is reversed UDP tunnel, meaning the server will initiate the connection. The port on client side is opened using udp hole punching.

## sample config.json for client
```json
{
  "role": "client",
  "servicePorts": [1194],
  "serverIP": "1.2.3.4",
  "resolver": "1.1.1.1",
  "negotiator": "https://sneaky-tunnel-negotiator-worker.alirezasn.workers.dev",
  "keepAliveInterval": [5, 20]
}
```

## sample config.json for server
```json
{
  "role": "server",
  "keepAliveInterval": [5, 20]
}
```