package main

import (
	"fmt"
	"io"
	"log"
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
	ConncetionsToUsers map[byte]*net.UDPAddr
	ClientConnections  map[string]byte
	Ready              bool
	ConnectionToServer *net.UDPConn
	LocalListeners     map[byte]*net.UDPConn
}

func (c *Client) GetPublicIP() {
	res, err := http.Get("http://api.ipify.org")
	handleError(err)
	ipBytes, err := io.ReadAll(res.Body)
	handleError(err)
	res.Body.Close()
	c.PublicIP = string(ipBytes)
	log.Printf("Client public IP: %s\n", ipBytes)
}

func (c *Client) SelectNegotiator() {
	for i, negotiator := range config.Negotiators {
		log.Printf("Testing negotiator: %s\n", negotiator)
		res, err := http.Head(negotiator)
		handleError(err)
		if res.StatusCode == 200 {
			c.Negotiator = negotiator
			log.Printf("\tNegotitator selected: %s\n", negotiator)
			break
		} else {
			log.Printf("\t%s did not respond to HEAD request with status 200\n", negotiator)
		}
		if i == len(config.Negotiators)-1 {
			log.Fatalln("Failed to select negotiator, none of them responded with 200 status.")
		}
	}
}

func (c *Client) NegotiatePorts() {
	listenAddress := resolveAddress("0.0.0.0:0")
	tempConn, err := net.ListenUDP("udp4", listenAddress)
	handleError(err)
	c.Port = getPortFromAddress(tempConn.LocalAddr().String())
	tempConn.Close()
	log.Printf("Selected port %s as listening port for tunnel\n", c.Port)
	res, err := http.Get(fmt.Sprintf("%s/%s/%s:%s", c.Negotiator, config.ServerIP, c.PublicIP, c.Port)) // https://negotiator/serverIP/ClientIPAndPort
	handleError(err)
	if res.StatusCode != 200 {
		log.Fatalf("GET %s/%s/%s:%s failed with status %d\n", c.Negotiator, config.ServerIP, c.PublicIP, c.Port, res.StatusCode)
	}
	portBytes, err := io.ReadAll(res.Body)
	handleError(err)
	res.Body.Close()
	c.ServerPort = string(portBytes)
	log.Printf("Negotiated server port: %s\n", portBytes)
}

func (c *Client) OpenPortAndSendDummyPacket() {
	listenAddress := resolveAddress("0.0.0.0:0" + c.Port)
	remoteAddress := resolveAddress(config.ServerIP + ":" + c.ServerPort)
	conn, err := net.DialUDP("udp4", listenAddress, remoteAddress)
	handleError(err)
	log.Printf("Opened port from %s to %s\n", conn.LocalAddr().String(), remoteAddress.String())
	_, err = conn.Write([]byte{1, 0}) // dummy packet
	handleError(err)
	log.Print("Sent dummy packet to server\n")
	conn.Close()
}

func (c *Client) AskServerToSendDummyPacket() {
	if config.ClientDelay > 0 {
		log.Printf("Waiting %d seconds before asking server for dummy packet", config.ClientDelay)
		for i := 0; i < config.ClientDelay; i++ {
			time.Sleep(time.Second)
			log.Print(".")
		}
		log.Print("\n")
	}
	res, err := http.Post(fmt.Sprintf("%s/%s/%s:%s", c.Negotiator, config.ServerIP, c.PublicIP, c.Port), "text/plain", nil) // https://negotiator/serverIP/ClientIPAndPort
	handleError(err)
	if res.StatusCode != 200 {
		log.Fatalf("POST %s/%s/%s:%s failed with status %d\n", c.Negotiator, config.ServerIP, c.PublicIP, c.Port, res.StatusCode)
	}
}

func (c *Client) Start() {
	c.ConncetionsToUsers = make(map[byte]*net.UDPAddr)
	c.ClientConnections = make(map[string]byte)
	c.LocalListeners = make(map[byte]*net.UDPConn)

	remoteAddress := resolveAddress(config.ServerIP + ":" + c.ServerPort)
	tunnelListenAddress := resolveAddress("0.0.0.0:" + c.Port)
	var err error
	var wg sync.WaitGroup
	shouldClose := false
	c.ConnectionToServer, err = net.DialUDP("udp4", tunnelListenAddress, remoteAddress)
	handleError(err)
	log.Printf("Listening on %s for dummy packet from %s\n", tunnelListenAddress.String(), remoteAddress.String())

	wg.Add(1)
	go func() {
		defer wg.Done()
		buffer := make([]byte, 1024*8)
		var packet Packet
		var n int
		for {
			if shouldClose {
				break
			}
			n, err = c.ConnectionToServer.Read(buffer)
			handleError(err)
			packet.DecodePacket(buffer[:n])

			// handle flags
			if packet.Flags == 1 {
				log.Printf("Received dummy packet from server\n")
				fmt.Println("READY")
				continue
			} else if packet.Flags == 3 {
				log.Printf("Received close connection packet from server\n")
				shouldClose = true
				break
			}

			_, err = c.LocalListeners[packet.ID].WriteTo(packet.Payload, c.ConncetionsToUsers[packet.ID])
			handleError(err)
		}
	}()

	c.AskServerToSendDummyPacket()

	wg.Add(1)
	go func() {
		var err error
		for {
			if shouldClose {
				break
			}
			time.Sleep(time.Second * 5)
			_, err = c.ConnectionToServer.Write([]byte{2, 0}) // keep-alive packet
			handleError(err)
		}
	}()

	for _, servicePort := range config.ListeningPorts {
		go func(servicePort uint16) {
			serviceListenAddress := resolveAddress(fmt.Sprintf("0.0.0.0:%d", servicePort))
			serviceConnection, err := net.ListenUDP("udp4", serviceListenAddress)
			handleError(err)
			log.Printf("Listening on %s for service packets\n", serviceListenAddress.String())

			buffer := make([]byte, (1024*8)-2)
			var packet Packet
			packet.Flags = 0
			var encodedPacketBytes []byte
			var serviceRemoteAddress *net.UDPAddr
			var n int
			for {
				if shouldClose {
					break
				}
				n, serviceRemoteAddress, err = serviceConnection.ReadFromUDP(buffer)
				handleError(err)
				if id, ok := c.ClientConnections[serviceRemoteAddress.String()]; ok {
					packet.ID = id
				} else {
					packet.ID = byte(len(c.ClientConnections))
					c.ClientConnections[serviceRemoteAddress.String()] = packet.ID
					c.ConncetionsToUsers[packet.ID] = serviceRemoteAddress
					c.LocalListeners[packet.ID] = serviceConnection
					log.Printf("Received packet from\n\tnew user at %s\n\ton service at %s\n\twith id of %d\n", serviceRemoteAddress.String(), serviceListenAddress.String(), packet.ID)
					announcementPacket := []byte{4, packet.ID}
					announcementPacket = append(announcementPacket, Uint16ToByteSlice(servicePort)...)
					_, err := c.ConnectionToServer.Write(announcementPacket)
					handleError(err)
					log.Printf("Sent port announcement packet to server\n")
				}
				packet.Payload = buffer[:n]
				encodedPacketBytes = packet.EncodePacket()
				_, err = c.ConnectionToServer.Write(encodedPacketBytes)
				handleError(err)
			}
		}(servicePort)
	}

	wg.Wait()
}

func (c *Client) CleanUp() {
	c.ConnectionToServer.Write([]byte{3, 0}) // close connection packet
	log.Printf("Sent close connection packet to server")
}
