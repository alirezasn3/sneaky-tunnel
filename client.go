package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type Client struct {
	PublicIP           string
	Negotiator         string
	ServerPort         string
	Port               string
	Clients            map[byte]*net.UDPAddr
	ClientConnections  map[string]byte
	Ready              bool
	ConnectionToServer *net.UDPConn
}

func (c *Client) GetPublicIP() {
	res, err := http.Get("http://api.ipify.org")
	handleError(err)
	ipBytes, err := io.ReadAll(res.Body)
	handleError(err)
	res.Body.Close()
	c.PublicIP = string(ipBytes)
	Log(fmt.Sprintf("Public IP: %s\n", ipBytes))
}

func (c *Client) SelectNegotiator() {
	for i, negotiator := range config.Negotiators {
		Log(fmt.Sprintf("Testing negotiator: %s\n", negotiator))
		res, err := http.Head(negotiator)
		handleError(err)
		if res.StatusCode == 200 {
			c.Negotiator = negotiator
			Log(fmt.Sprintf("Negotitator selected: %s\n", negotiator))
			break
		} else {
			Log(fmt.Sprintf("%s did not respond to HEAD request with status 200\n", negotiator))
		}
		if i == len(config.Negotiators)-1 {
			panic("Failed to select negotiator, none of them responded with 200 status.")
		}
	}
}

func (c *Client) NegotiatePorts() {
	c.Port = config.ClientPort
	res, err := http.Get(fmt.Sprintf("%s/%s/%s:%s", c.Negotiator, config.ServerIP, c.PublicIP, c.Port)) // https://negotiator/serverIP/ClientIPAndPort
	handleError(err)
	if res.StatusCode != 200 {
		panic(fmt.Sprintf("GET %s/%s/%s:%s failed with status %d", c.Negotiator, config.ServerIP, c.PublicIP, c.Port, res.StatusCode))
	}
	portBytes, err := io.ReadAll(res.Body)
	handleError(err)
	res.Body.Close()
	c.ServerPort = string(portBytes)
	Log(fmt.Sprintf("Negotiated server port: %s\n", portBytes))
}

func (c *Client) OpenPortAndSendDummyPacket() {
	listenAddress := resolveAddress("0.0.0.0:" + c.Port)
	remoteAddress := resolveAddress(config.ServerIP + ":" + c.ServerPort)
	conn, err := net.DialUDP("udp", listenAddress, remoteAddress)
	handleError(err)
	Log(fmt.Sprintf("Opened port from %s to %s\n", conn.LocalAddr().String(), remoteAddress.String()))
	conn.Write([]byte{1, 0, 0, 0}) // dummy packet
	Log("Sent dummy packet to server\n")
	conn.Close()
}

func (c *Client) AskServerToSendDummyPacket() {
	Log("Waiting 3 seconds before asking server for dummy packet")
	for i := 0; i < 3; i++ {
		time.Sleep(time.Second)
		Log(".")
	}
	Log("\n")
	res, err := http.Post(fmt.Sprintf("%s/%s/%s:%s", c.Negotiator, config.ServerIP, c.PublicIP, c.Port), "text/plain", nil) // https://negotiator/serverIP/ClientIPAndPort
	handleError(err)
	if res.StatusCode != 200 {
		panic(fmt.Sprintf("POST %s/%s/%s:%s failed with status %d", c.Negotiator, config.ServerIP, c.PublicIP, c.Port, res.StatusCode))
	}
}

func (c *Client) Start() {
	c.Clients = make(map[byte]*net.UDPAddr)
	c.ClientConnections = make(map[string]byte)

	remoteAddress := resolveAddress(config.ServerIP + ":" + c.ServerPort)
	localAddress := resolveAddress("0.0.0.0:" + c.Port)
	listenAddress := resolveAddress(fmt.Sprintf("0.0.0.0:%d", config.AppPort))

	var err error
	c.ConnectionToServer, err = net.DialUDP("udp", localAddress, remoteAddress)
	handleError(err)
	Log(fmt.Sprintf("Listening on %s for dummy packet from %s\n", localAddress.String(), remoteAddress.String()))

	localConn, err := net.ListenUDP("udp", listenAddress)
	handleError(err)
	Log(fmt.Sprintf("Listening on %s for local connections\n", listenAddress.String()))

	go c.AskServerToSendDummyPacket()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		buffer := make([]byte, (1024*8)-4)
		var packet Packet
		packet.Flags = 0
		var encodedPacketBytes []byte
		var localAppAddress *net.UDPAddr
		var n int
		for {
			n, localAppAddress, err = localConn.ReadFromUDP(buffer)
			handleError(err)
			if id, ok := c.ClientConnections[localAppAddress.String()]; ok {
				packet.ID = id
			} else {
				packet.ID = byte(len(c.ClientConnections))
				c.ClientConnections[localAppAddress.String()] = packet.ID
				c.Clients[packet.ID] = localAppAddress
			}
			packet.Payload = buffer[:n]
			encodedPacketBytes = packet.EncodePacket()
			_, err = c.ConnectionToServer.Write(encodedPacketBytes)
			handleError(err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		buffer := make([]byte, 1024*8)
		var packet Packet
		var n int
		for {
			n, err = c.ConnectionToServer.Read(buffer)
			handleError(err)
			packet.DecodePacket(buffer[:n])

			// handle flags
			if packet.Flags == 1 {
				Log("Received dummy packet from server\n")
				continue
			}

			_, err = localConn.WriteTo(packet.Payload, c.Clients[packet.ID])
			handleError(err)
		}
	}()

	wg.Add(1)
	go func() {
		var err error
		for {
			time.Sleep(time.Second * 5)
			_, err = c.ConnectionToServer.Write([]byte{2, 0, 0, 0}) // keep-alive packet
			handleError(err)
		}
	}()

	wg.Wait()
}

func (c *Client) CleanUp() {
	c.ConnectionToServer.Write([]byte{3, 0, 0, 0}) // close connection packet
	Log("Sent close connection packet to server")
}
