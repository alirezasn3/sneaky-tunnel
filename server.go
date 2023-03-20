package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// name User to avoid conflict with Client struct
type User struct {
	Connection             *net.UDPConn
	ConnectionsToLocalApp  map[byte]*net.UDPConn
	LastReceivedPacketTime int64
	Ready                  bool
	ActualAddress          *net.UDPAddr
	ShouldClose            bool
}

type Server struct {
	ServerToClientConnections map[string]*User
}

func (s *Server) ListenForNegotiationRequests() {
	s.ServerToClientConnections = make(map[string]*User)

	go func() {
		ticker := time.NewTicker(time.Second * 60)
		for range ticker.C {
			for clientIPAndPort, user := range s.ServerToClientConnections {
				if user.Ready && time.Now().Unix()-user.LastReceivedPacketTime > 10 {
					if user.ShouldClose {
						log.Printf("Evicting dissconnected client at %s\n", user.ActualAddress.String())
						delete(s.ServerToClientConnections, clientIPAndPort)
					} else {
						user.ShouldClose = true
					}
				}
			}
		}
	}()

	err := http.ListenAndServe("0.0.0.0:80", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s\n", r.Method, r.URL.String())
		if r.Method == "GET" {
			urlParts := strings.Split(r.URL.String(), "/")
			clientIPAndPort := urlParts[len(urlParts)-1]
			clientIPAndPortParts := strings.Split(clientIPAndPort, ":")
			if len(clientIPAndPortParts) != 2 {
				w.WriteHeader(400)
				return
			}
			ip := clientIPAndPortParts[0]
			port := clientIPAndPortParts[1]
			if net.ParseIP(ip) == nil {
				w.WriteHeader(400)
				return
			}
			_, err := strconv.ParseUint(port, 10, 16)
			if err != nil {
				w.WriteHeader(400)
				return
			}
			if _, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
				w.WriteHeader(400)
				return
			}
			lAddr := resolveAddress("0.0.0.0:0")
			conn, err := net.ListenUDP("udp", lAddr)
			handleError(err)
			s.ServerToClientConnections[clientIPAndPort] = &User{}
			s.ServerToClientConnections[clientIPAndPort].Ready = false
			s.ServerToClientConnections[clientIPAndPort].ShouldClose = false
			s.ServerToClientConnections[clientIPAndPort].ActualAddress = nil
			s.ServerToClientConnections[clientIPAndPort].Connection = conn
			s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp = make(map[byte]*net.UDPConn)
			localAddrParts := strings.Split(conn.LocalAddr().String(), ":")
			w.Write([]byte(localAddrParts[len(localAddrParts)-1]))
			go s.HandleClientPackets(clientIPAndPort)
		} else if r.Method == "POST" {
			urlParts := strings.Split(r.URL.String(), "/")
			clientIPAndPort := urlParts[len(urlParts)-1]
			if _, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
				go s.SendDummyPacket(clientIPAndPort)
				w.WriteHeader(200)
			} else {
				w.WriteHeader(400)
			}
		} else {
			w.WriteHeader(400)
		}
	}))
	handleError(err)
}

func (s *Server) SendDummyPacket(clientIPAndPort string) {
	if s.ServerToClientConnections[clientIPAndPort].ActualAddress == nil {
		s.ServerToClientConnections[clientIPAndPort].ActualAddress = resolveAddress(clientIPAndPort)
		_, err := s.ServerToClientConnections[clientIPAndPort].Connection.WriteToUDP([]byte{1, 0, 0, 0}, s.ServerToClientConnections[clientIPAndPort].ActualAddress)
		handleError(err)
	} else {
		_, err := s.ServerToClientConnections[clientIPAndPort].Connection.WriteToUDP([]byte{1, 0, 0, 0}, s.ServerToClientConnections[clientIPAndPort].ActualAddress)
		handleError(err)
	}
	log.Printf("Sent dummy packet to %s\n", clientIPAndPort)
}

func (s *Server) HandleClientPackets(clientIPAndPort string) {
	connectionToClient := s.ServerToClientConnections[clientIPAndPort].Connection
	user := s.ServerToClientConnections[clientIPAndPort]
	buffer := make([]byte, 1024*8)
	var packet Packet
	var n int
	var err error
	var clientActualAddress *net.UDPAddr
	var destinationPort uint16
mainLoop:
	for {
		n, clientActualAddress, err = connectionToClient.ReadFromUDP(buffer)
		if user.ShouldClose {
			break mainLoop
		}
		if err != nil {
			log.Printf("Error reading packet from %s\n%s\n", clientIPAndPort, err)
			user.ShouldClose = true
			break mainLoop
		}

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
			} else if packet.Flags == 2 { // keep-alive
				user.LastReceivedPacketTime = time.Now().Unix()
			} else if packet.Flags == 3 { // close connection
				log.Printf("Received close connection packet from %s\n", clientIPAndPort)
				user.ShouldClose = true
				break mainLoop
			} else if packet.Flags == 4 { // destination port announcement
				if len(packet.Payload) == 0 {
					destinationPort = 1194
					log.Printf("Received empty announcement packet, assuming port 1194")
				} else {
					destinationPort = ByteSliceToUint16(packet.Payload)
					log.Printf("Received destination announcement packet with id %d for port %d\n", packet.ID, ByteSliceToUint16(packet.Payload))
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
				log.Printf("Error writing packet to 0.0.0.0:%d\n%s\n", destinationPort, err)
				user.ShouldClose = true
				break mainLoop
			}
		} else {
			user.LastReceivedPacketTime = time.Now().Unix()
			user.Ready = true

			user.ConnectionsToLocalApp[packet.ID], err = net.DialUDP("udp", nil, resolveAddress(fmt.Sprintf("0.0.0.0:%d", destinationPort)))
			handleError(err)

			connectionToLocalApp := user.ConnectionsToLocalApp[packet.ID]
			log.Printf("Created new connection to %s for packets with id %d\n", connectionToLocalApp.RemoteAddr().String(), packet.ID)

			_, err = connectionToLocalApp.Write(packet.Payload)
			if err != nil {
				if user.ShouldClose {
					break mainLoop
				}
				log.Printf("Error writing packet to 0.0.0.0:%d\n%s\n", destinationPort, err)
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
	connectionToClient.Write([]byte{3, 0})
	log.Printf("Sent close connection packet to %s\n", clientActualAddress.String())
	connectionToClient.Close()
	log.Printf("Closed connection to %s\n", clientActualAddress.String())
	delete(s.ServerToClientConnections, clientIPAndPort)
}
