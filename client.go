package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Client struct {
	PublicIP          string
	Negotiator        string
	ServerPort        string
	Port              string
	Clients           map[byte]*net.UDPAddr
	ClientConnections map[string]byte
	Ready             bool
}

func (c *Client) GetPublicIP() {
	res, err := http.Get("http://api.ipify.org")
	handleError(err)
	ipBytes, err := io.ReadAll(res.Body)
	handleError(err)
	res.Body.Close()
	c.PublicIP = string(ipBytes)
	fmt.Printf("Public IP: %s\n", ipBytes)
}

func (c *Client) SelectNegotiator() {
	for i, negotiator := range config.Negotiators {
		fmt.Printf("Testing negotiator: %s\n", negotiator)
		res, err := http.Head(negotiator)
		handleError(err)
		if res.StatusCode == 200 {
			c.Negotiator = negotiator
			fmt.Printf("Negotitator selected: %s\n", negotiator)
			break
		} else {
			fmt.Printf("%s did not respond to HEAD request with status 200\n", negotiator)
		}
		if i == len(config.Negotiators)-1 {
			panic("Failed to select negotiator, none of them responded with 200 status.")
		}
	}
}

func (c *Client) FindUnusedPort() {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:")
	handleError(err)
	addrParts := strings.Split(conn.LocalAddr().String(), ":")
	c.Port = addrParts[len(addrParts)-1]
	conn.Close()
	fmt.Printf("Port %s selected as the receiving port for UDP connection to server\n", c.Port)
}

func (c *Client) NegotiatePorts() {
	res, err := http.Get(fmt.Sprintf("%s/%s/%s:%s", c.Negotiator, config.Server, c.PublicIP, c.Port)) // https://negotiator/serverIP/ClientIPAndPort
	handleError(err)
	if res.StatusCode != 200 {
		panic(fmt.Sprintf("GET %s/%s/%s:%s failed with status %d", c.Negotiator, config.Server, c.PublicIP, c.Port, res.StatusCode))
	}
	portBytes, err := io.ReadAll(res.Body)
	handleError(err)
	res.Body.Close()
	c.ServerPort = string(portBytes)
	fmt.Printf("Negotiated server port: %s\n", portBytes)
}

func (c *Client) OpenPortAndSendDummyPacket() {
	remoteAddress := resolveAddress(config.Server + ":" + c.ServerPort)
	conn, err := net.ListenPacket("udp4", "0.0.0.0:"+c.Port)
	handleError(err)
	fmt.Printf("Opened port from %s to %s\n", conn.LocalAddr().String(), remoteAddress.String())
	conn.WriteTo([]byte{0, 0}, remoteAddress)
	conn.Close()
}

func (c *Client) AskServerToSendDummyPacket() {
	fmt.Print("Waiting 3 seconds before asking server for dummy packet")
	for i := 0; i < 3; i++ {
		time.Sleep(time.Second)
		fmt.Print(".")
	}
	fmt.Println()
	res, err := http.Post(fmt.Sprintf("%s/%s/%s:%s", c.Negotiator, config.Server, c.PublicIP, c.Port), "text/plain", nil) // https://negotiator/serverIP/ClientIPAndPort
	handleError(err)
	if res.StatusCode != 200 {
		panic(fmt.Sprintf("POST %s/%s/%s:%s failed with status %d", c.Negotiator, config.Server, c.PublicIP, c.Port, res.StatusCode))
	}
}

func (c *Client) Start() {
	c.Clients = make(map[byte]*net.UDPAddr)
	c.ClientConnections = make(map[string]byte)

	remoteAddress := resolveAddress(config.Server + ":" + c.ServerPort)
	localAddress := resolveAddress("0.0.0.0:" + c.Port)
	listenAddress := resolveAddress(config.ListenOn)

	/* conn, err := net.DialUDP("udp4", localAddress, remoteAddress) */
	conn, err := net.ListenUDP("udp4", localAddress)
	handleError(err)
	fmt.Printf("Listening on %s for dummy packet from %s\n", localAddress.String(), remoteAddress.String())

	localConn, err := net.ListenUDP("udp4", listenAddress)
	handleError(err)
	fmt.Printf("Listening on %s for local connections\n", localAddress.String())

	go c.AskServerToSendDummyPacket()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		buffer := make([]byte, (1024*8)-2)
		var packet Packet
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
			packet.Flags = 0
			packet.Payload = buffer[:n]
			encodedPacketBytes = packet.EncodePacket()
			/* _, err = conn.Write(encodedPacketBytes) */
			_, err = conn.WriteToUDP(encodedPacketBytes, remoteAddress)
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
			n, err = conn.Read(buffer)
			handleError(err)
			packet.DecodePacket(buffer[:n])
			if len(packet.Payload) == 0 {
				fmt.Println("received dummy packet from server")
				continue
			}
			_, err = localConn.WriteTo(packet.Payload, c.Clients[packet.ID])
			handleError(err)
		}
	}()

	wg.Wait()
}
