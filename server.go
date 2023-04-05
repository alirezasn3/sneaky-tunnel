package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// name User to avoid conflict with Client struct
type User struct {
	Connection                     *net.UDPConn
	ConnectionsToLocalApp          map[byte]*net.UDPConn
	LastReceivedPacketTime         int64
	Ready                          bool
	ActualAddress                  *net.UDPAddr
	ShouldClose                    bool
	Mode                           string // possible values: "tunnel", "vpn"
	PacketIDToDestinationPortTable map[byte]uint16
}

type Server struct {
	ServerToClientConnections map[string]*User
	BlockedIPs                []string
}

func (s *Server) IsBlockedIP(ip string) bool {
	for _, i := range s.BlockedIPs {
		if i == ip {
			return true
		}
	}
	return false
}

func (s *Server) Start() {
	s.ServerToClientConnections = make(map[string]*User)

	go func() {
		ticker := time.NewTicker(time.Second * time.Duration(config.KeepAliveInterval[1]))
		for range ticker.C {
			for clientIPAndPort, user := range s.ServerToClientConnections {
				if user.ShouldClose {
					delete(s.ServerToClientConnections, clientIPAndPort)
				}
				diff := time.Now().Unix() - user.LastReceivedPacketTime
				if user.Ready && diff > 60 {
					log.Printf("Evicting disconnected client at %s, received last packet %d seconds ago\n", user.ActualAddress.String(), diff)
					user.ShouldClose = true
				}
			}
			log.Println()
			log.Printf("Hourly check, %d active connections:\n", len(s.ServerToClientConnections))
			for clientIPAndPort := range s.ServerToClientConnections {
				log.Printf("%s\n", clientIPAndPort)
			}
			log.Println()
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Second * time.Duration(config.KeepAliveInterval[0]))
		for range ticker.C {
			for clientIPAndPort, user := range s.ServerToClientConnections {
				if user.ShouldClose {
					delete(s.ServerToClientConnections, clientIPAndPort)
					continue
				}
				if !user.Ready {
					continue
				}
				_, err := user.Connection.WriteToUDP([]byte{2, 0}, user.ActualAddress)
				if err != nil {
					log.Printf("Error sending keep-alive packet to client at %s\n\t%s\n", user.ActualAddress.String(), err.Error())
					user.ShouldClose = true
				}
			}
		}
	}()

	err := http.ListenAndServe("0.0.0.0:80", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.IsBlockedIP(r.RemoteAddr) {
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
				return
			}
			w.WriteHeader(500)
			return
		}

		urlParts := strings.Split(r.URL.String(), "/")
		clientIPAndPort := urlParts[len(urlParts)-1]

		if isValid := isValidAddress(clientIPAndPort); !isValid {
			w.WriteHeader(500)
			s.BlockedIPs = append(s.BlockedIPs, getIPFromAddress(r.RemoteAddr))
			log.Printf("Blocked %s\n", getIPFromAddress(r.RemoteAddr))
			return
		}

		log.Printf("%s %s\n", r.Method, r.URL.String())

		if r.Method == "GET" {
			if _, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
				w.WriteHeader(400)
				return
			}
			lAddr := resolveAddress("0.0.0.0:0")
			conn, err := net.ListenUDP("udp", lAddr)
			if err != nil {
				log.Panic(err)
			}
			s.ServerToClientConnections[clientIPAndPort] = &User{Ready: false, ShouldClose: false, ActualAddress: nil, Connection: conn, ConnectionsToLocalApp: make(map[byte]*net.UDPConn), Mode: ""}
			w.Write([]byte(getPortFromAddress(conn.LocalAddr().String())))
			go s.HandleClient(clientIPAndPort)
		} else if r.Method == "POST" {
			if _, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
				go s.SendDummyPacket(clientIPAndPort)
				w.WriteHeader(200)
			} else {
				w.WriteHeader(400)
			}
		} else {
			w.WriteHeader(500)
			s.BlockedIPs = append(s.BlockedIPs, getIPFromAddress(r.RemoteAddr))
		}
	}))
	if err != nil {
		log.Panic(err)
	}
}

func (s *Server) SendDummyPacket(clientIPAndPort string) {
	if s.ServerToClientConnections[clientIPAndPort].ActualAddress == nil {
		s.ServerToClientConnections[clientIPAndPort].ActualAddress = resolveAddress(clientIPAndPort)

	}
	_, err := s.ServerToClientConnections[clientIPAndPort].Connection.WriteToUDP([]byte{1, 0}, s.ServerToClientConnections[clientIPAndPort].ActualAddress)
	if err != nil {
		log.Printf("Failed to send dummy packet to client at %s\n", s.ServerToClientConnections[clientIPAndPort].ActualAddress)
		s.ServerToClientConnections[clientIPAndPort].ShouldClose = true
		return
	}
	log.Printf("Sent dummy packet to %s\n", clientIPAndPort)
}

func (s *Server) HandleClient(clientIPAndPort string) {
	connectionToClient := s.ServerToClientConnections[clientIPAndPort].Connection
	user := s.ServerToClientConnections[clientIPAndPort]
	buffer := make([]byte, 1024*8)
	var packet Packet
	var n int
	var err error
	var clientActualAddress *net.UDPAddr

mainLoop:
	for {
		n, clientActualAddress, err = connectionToClient.ReadFromUDP(buffer)
		if user.ShouldClose {
			break mainLoop
		}
		if err != nil {
			if user.ShouldClose {
				break mainLoop
			}
			log.Printf("Error reading packet from %s\n%s\n", clientIPAndPort, err)
			user.ShouldClose = true
			break mainLoop
		}

		user.LastReceivedPacketTime = time.Now().Unix()

		// handle flags
		packet.DecodePacket(buffer[:n])
		if packet.Flags > 0 {
			if packet.Flags == 1 { // dummy
				user.Ready = true
				log.Printf("Received dummy packet from %s\n", clientIPAndPort)
				user.ActualAddress = clientActualAddress
				if clientActualAddress.String() != clientIPAndPort {
					log.Printf("Actual address for %s is %s\n", clientIPAndPort, clientActualAddress.String())
				}
			} else if packet.Flags == 3 { // close connection
				log.Printf("Received close connection packet from %s\n", clientIPAndPort)
				user.ShouldClose = true
				break mainLoop
			} else if packet.Flags == 4 { // destination port announcement
				if user.Mode != "" {
					if len(packet.Payload) >= 2 {
						user.PacketIDToDestinationPortTable[packet.ID] = ByteSliceToUint16(packet.Payload)
						log.Printf("Received destination announcement packet with id %d for port %d\n", packet.ID, user.PacketIDToDestinationPortTable[packet.ID])
					} else {
						log.Printf("Received invalid destination port announcement packet from %s\n", user.ActualAddress)
					}
				} else {
					log.Printf("Received distination port announcement packet before mode announcement packet from %s\n", user.ActualAddress)
				}
			} else if packet.Flags == 6 { // mode announcement
				if len(packet.Payload) >= 1 {
					if packet.Payload[0] == 1 {
						user.Mode = "tunnel"
					} else if packet.Payload[0] == 2 {
						user.Mode = "vpn"
					} else {
						log.Printf("Received invalid mode announcement packet from %s\n", user.ActualAddress)
					}
				} else {
					log.Printf("Received invalid mode announcement packet from %s\n", user.ActualAddress)
				}
			}
			continue mainLoop
		}

		if connectionToLocalApp, ok := user.ConnectionsToLocalApp[packet.ID]; ok {
			_, err = connectionToLocalApp.Write(packet.Payload)
			if err != nil {
				if user.ShouldClose {
					break mainLoop
				}
				log.Printf("Error writing packet to 0.0.0.0:%d\n%s\n", user.PacketIDToDestinationPortTable[packet.ID], err)
				user.ShouldClose = true
				break mainLoop
			}
		} else {
			if _, ok := user.PacketIDToDestinationPortTable[packet.ID]; !ok {
				continue
			}
			serviceAddress := resolveAddress(fmt.Sprintf("0.0.0.0:%d", user.PacketIDToDestinationPortTable[packet.ID]))
			user.ConnectionsToLocalApp[packet.ID], err = net.DialUDP("udp", nil, serviceAddress)
			if err != nil {
				if user.ShouldClose {
					break mainLoop
				}
				log.Printf("Failed to dial service at %s\n", serviceAddress)
				user.ShouldClose = true
				break mainLoop
			}

			connectionToLocalApp := user.ConnectionsToLocalApp[packet.ID]
			log.Printf("Created new connection to %s for packets with id %d\n", connectionToLocalApp.RemoteAddr().String(), packet.ID)

			_, err = connectionToLocalApp.Write(packet.Payload)
			if err != nil {
				if user.ShouldClose {
					break mainLoop
				}
				log.Printf("Error writing packet to 0.0.0.0:%d\n%s\n", user.PacketIDToDestinationPortTable[packet.ID], err)
				user.ShouldClose = true
				break mainLoop
			}

			go func(id byte) {
				buffer := make([]byte, (1024*8)-2)
				var packet Packet
				var encodedPacketBytes []byte
				var n int
				var err error
				for {
					n, err = connectionToLocalApp.Read(buffer)
					if err != nil {
						if user.ShouldClose {
							break
						}
						log.Printf("Error reading packet from %s\t\n%s\n", connectionToLocalApp.RemoteAddr().String(), err)
						user.ShouldClose = true
						break
					}
					packet.Flags = 0
					packet.ID = id
					packet.Payload = buffer[:n]
					encodedPacketBytes = packet.EncodePacket()
					_, err = connectionToClient.WriteToUDP(encodedPacketBytes, clientActualAddress)
					if err != nil {
						if user.ShouldClose {
							break
						}
						log.Printf("Error writing packet to %s\n%s\n", connectionToClient.RemoteAddr().String(), err)
						user.ShouldClose = true
						break
					}
				}
			}(packet.ID)
		}
	}
	connectionToClient.WriteToUDP([]byte{3, 0}, user.ActualAddress)
	log.Printf("Sent close connection packet to %s\n", clientActualAddress.String())
	connectionToClient.Close()
	log.Printf("Closed connection to %s\n", clientActualAddress.String())
	delete(s.ServerToClientConnections, clientIPAndPort)
}
