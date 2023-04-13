package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

type Client struct {
	ServerPort                      string
	Port                            string
	ServiceAddresses                map[byte]*net.UDPAddr
	ServiceIDs                      map[string]byte
	Ready                           bool
	ConnectionToServer              *net.UDPConn
	ServiceListeners                map[byte]*net.UDPConn
	LastReceivedPacketTime          int64
	IsListeningForPacketsFromServer bool
}

func (c *Client) NegotiatePorts() {
	res, err := httpClient.Head(config.Negotiator)
	if err != nil {
		log.Fatalln(err)
	}
	if res.StatusCode != 200 {
		log.Fatalf("%s did not respond to HEAD request with status 200\n", config.Negotiator)
	}

	listenAddress := resolveAddress("0.0.0.0:0")
	tempConn, err := net.ListenUDP("udp4", listenAddress)
	if err != nil {
		log.Fatalln(err)
	}
	c.Port = getPortFromAddress(tempConn.LocalAddr().String())
	tempConn.Close()
	log.Printf("Selected port %s as listening port for tunnel\n", c.Port)
	res, err = httpClient.Get(fmt.Sprintf("%s/%s/%s", config.Negotiator, config.ServerIP, c.Port)) // https://negotiator/serverIP/ClientIPAndPort
	if err != nil {
		log.Fatalln(err)
	}
	if res.StatusCode != 200 {
		log.Fatalf("GET %s/%s/%s failed with status %d\n", config.Negotiator, config.ServerIP, c.Port, res.StatusCode)
	}
	portBytes, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalln(err)
	}
	res.Body.Close()
	c.ServerPort = string(portBytes)
	log.Printf("Negotiated server port: %s\n", portBytes)
}

func (c *Client) OpenPortAndSendDummyPacket() {
	listenAddress := resolveAddress("0.0.0.0:0" + c.Port)
	remoteAddress := resolveAddress(config.ServerIP + ":" + c.ServerPort)
	conn, err := net.DialUDP("udp4", listenAddress, remoteAddress)
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("Opened port from %s to %s\n", conn.LocalAddr().String(), remoteAddress.String())
	_, err = conn.Write([]byte{1, 0}) // dummy packet
	if err != nil {
		log.Fatalln(err)
	}
	log.Print("Sent dummy packet to server\n")
	conn.Close()
}

func (c *Client) AskServerToSendDummyPacket() {
	for !c.IsListeningForPacketsFromServer {
		time.Sleep(time.Millisecond * 10)
	}
	log.Printf("Asking server for dummy packet\n")
	res, err := httpClient.Post(fmt.Sprintf("%s/%s/%s", config.Negotiator, config.ServerIP, c.Port), "text/plain", nil) // https://negotiator/serverIP/ClientIPAndPort
	if err != nil {
		log.Fatalln(err)
	}
	if res.StatusCode != 200 {
		log.Fatalf("POST %s/%s/%s failed with status %d\n", config.Negotiator, config.ServerIP, c.Port, res.StatusCode)
	}
}

func (c *Client) Start() {
	c.NegotiatePorts()
	c.OpenPortAndSendDummyPacket()

	c.ServiceAddresses = make(map[byte]*net.UDPAddr)
	c.ServiceIDs = make(map[string]byte)
	c.ServiceListeners = make(map[byte]*net.UDPConn)

	remoteAddress := resolveAddress(config.ServerIP + ":" + c.ServerPort)
	tunnelListenAddress := resolveAddress("0.0.0.0:" + c.Port)
	var err error
	shouldClose := false
	c.ConnectionToServer, err = net.DialUDP("udp4", tunnelListenAddress, remoteAddress)
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("Listening on %s for dummy packet from %s\n", tunnelListenAddress.String(), remoteAddress.String())

	go func() {
		var packet Packet
		buffer := make([]byte, 1024*8)
		var n int
		c.IsListeningForPacketsFromServer = true
		for {
			if shouldClose {
				break
			}
			n, err = c.ConnectionToServer.Read(buffer)
			if err != nil {
				if shouldClose {
					break
				}
				log.Fatalln(err)
			}
			packet.DecodePacket(buffer[:n])

			c.LastReceivedPacketTime = time.Now().Unix()

			// handle flags
			if packet.Flags == 1 {
				log.Printf("Received dummy packet from server\n")
				c.Ready = true
				fmt.Println("READY")
				continue
			} else if packet.Flags == 3 {
				log.Printf("Received close connection packet from server\n")
				shouldClose = true
				break
			} else if packet.Flags == 2 {
				_, err = c.ConnectionToServer.Write([]byte{5, 0}) // keep-alive response packet
				if err != nil {
					if shouldClose {
						break
					}
					log.Fatalln(err)
				}
				continue
			}

			_, err = c.ServiceListeners[packet.ID].WriteTo(packet.Payload, c.ServiceAddresses[packet.ID])
			if err != nil {
				if shouldClose {
					break
				}
				log.Fatalln(err)
			}
		}
	}()

	c.AskServerToSendDummyPacket()

	for _, servicePort := range config.ServicePorts {
		go func(servicePort uint16) {
			serviceListenAddress := resolveAddress(fmt.Sprintf("0.0.0.0:%d", servicePort))
			serviceListener, err := net.ListenUDP("udp4", serviceListenAddress)
			if err != nil {
				if shouldClose {
					return
				}
				log.Fatalln(err)
			}
			log.Printf("Listening on %s for service packets\n", serviceListenAddress.String())

			packet := createPacket()
			buffer := make([]byte, (1024*8)-2)
			var n int
			var serviceRemoteAddress *net.UDPAddr
			for {
				if shouldClose {
					break
				}
				n, serviceRemoteAddress, err = serviceListener.ReadFromUDP(buffer)
				if err != nil {
					if shouldClose {
						break
					}
					log.Fatalln(err)
				}
				if id, ok := c.ServiceIDs[serviceRemoteAddress.String()]; ok {
					packet.ID = id
				} else {
					packet.ID = byte(len(c.ServiceIDs))
					c.ServiceIDs[serviceRemoteAddress.String()] = packet.ID
					c.ServiceAddresses[packet.ID] = serviceRemoteAddress
					c.ServiceListeners[packet.ID] = serviceListener
					log.Printf("Received packet from new user at %s on service at %s with id of %d\n", serviceRemoteAddress.String(), serviceListenAddress.String(), packet.ID)
					announcementPacket := []byte{4, packet.ID}
					announcementPacket = append(announcementPacket, Uint16ToByteSlice(servicePort)...)
					_, err := c.ConnectionToServer.Write(announcementPacket)
					if err != nil {
						if shouldClose {
							break
						}
						log.Fatalln(err)
					}
					log.Printf("Sent port announcement packet to server\n")
				}
				packet.Payload = buffer[:n]
				_, err = c.ConnectionToServer.Write(packet.EncodePacket())
				if err != nil {
					if shouldClose {
						break
					}
					log.Fatalln(err)
				}
			}
		}(servicePort)
	}

	ticker := time.NewTicker(time.Second * time.Duration(config.KeepAliveInterval[1]))
	for range ticker.C {
		diff := time.Now().Unix() - c.LastReceivedPacketTime
		if c.Ready && diff > int64(config.KeepAliveInterval[1]) {
			log.Printf("Did not receive keep-alive packet from server for %d seconds, closing connection\n", diff)
			shouldClose = true
			break
		}
	}
}
