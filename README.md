# Sneaky Tunnel

This is reversed UDP tunnel, meaning the server will initiate the connection. The port on client side is opened using udp hole punching.

## How it workks
First the client sends an http request to the negotiator containing the servers's ip and the client's port. Then The negotiator sends the client's ip and port to the server. The client sends a udp packet to server to open a new route in the NAT. Then the client sends another http request to the negotiator to ask the server for a udp packet. Once the packet is received on client side, the connection is initiated.

If the server's ip is blocked, the first packet sent by the client, opens the NAT but doesn't reach the server. Since that route is created in the NAT and the packets sent from a server with a blocked ip are not dropped, the server can send a packet to the client and they can communicate.

But if the server's ip is not blocked, technically the connection is initiated by the client and not the server. Nevertheless, the client and the server can communicate with each other. 

All the packets are given an id so multiple devices can send and receive data over one udp connection between server and client. This makes the packets harder to detect for DPI tools.

## sample config.json for client

```json
{
  "role": "client",
  "servicePorts": [1194],
  "serverIP": "1.2.3.4",
  "resolver": "1.1.1.1",
  "negotiator": "https://sneaky-tunnel-negotiator-worker.alirezasn.workers.dev",
  "keepAliveInterval": [0, 20],
  "retryDelay": 1,
  "retryCount": 10,
  "serviceTimeout": 60
}
```

## sample config.json for server

```json
{
  "role": "server",
  "keepAliveInterval": [5, 20]
}
```
