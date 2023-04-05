# sneaky-tunnel
This is reversed UDP tunnel, meaning the server will initiate the connection. The port on client side is opened using udp hole punching.

## sample config.json for client
```json
{
  "role": "client",
  "servicePorts": [1194],
  "serverIP": "1.2.3.4",
  "resolver": "1.1.1.1",
  "negotiators": [
    "https://sneaky-tunnel-negotiator-worker.alirezasn.workers.dev"
  ]
}
```

## sample config.json for server
```json
{
  "role": "server"
}
```