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
	Connection             *net.UDPConn
	ConnectionsToLocalApp  map[byte]*net.UDPConn
	LastReceivedPacketTime int64
	Ready                  bool
	ActualAddress          *net.UDPAddr
}

type Server struct {
	ServerToClientConnections map[string]*User
}

func (s *Server) ListenForNegotiationRequests() {
	s.ServerToClientConnections = make(map[string]*User)

	go func() {
		ticker := time.NewTicker(time.Second * 10)
		for range ticker.C {
			for _, user := range s.ServerToClientConnections {
				if user.Ready && time.Now().Unix()-user.LastReceivedPacketTime > 10 {
					log.Printf("Evicting discoonected client at %s\n", user.ActualAddress.String())
				}
			}
		}
	}()

	err := http.ListenAndServe("0.0.0.0:80", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s\n", r.Method, r.URL.String())
		if r.Method == "GET" {
			urlParts := strings.Split(r.URL.String(), "/")
			clientIPAndPort := urlParts[len(urlParts)-1]
			if _, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
				w.WriteHeader(400)
				return
			}
			// clientAddress := resolveAddress(clientIPAndPort)
			lAddr := resolveAddress("0.0.0.0:0")
			conn, err := net.ListenUDP("udp", lAddr)
			handleError(err)
			s.ServerToClientConnections[clientIPAndPort] = &User{}
			s.ServerToClientConnections[clientIPAndPort].Ready = false
			s.ServerToClientConnections[clientIPAndPort].Connection = conn
			s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp = make(map[byte]*net.UDPConn)
			localAddrParts := strings.Split(conn.LocalAddr().String(), ":")
			w.Write([]byte(localAddrParts[len(localAddrParts)-1]))
			go s.HandleClientPackets(clientIPAndPort)
			go func() {
				time.Sleep(time.Second * 10)
				if !s.ServerToClientConnections[clientIPAndPort].Ready {
					s.ServerToClientConnections[clientIPAndPort].Connection.Close()
					delete(s.ServerToClientConnections, clientIPAndPort)
					log.Printf("Closed Connection to %s because client did not send a packet in time\n", clientIPAndPort)
				}
			}()
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
	_, err := s.ServerToClientConnections[clientIPAndPort].Connection.WriteToUDP([]byte{1, 0, 0, 0}, s.ServerToClientConnections[clientIPAndPort].ActualAddress)
	handleError(err)
	log.Printf("Sent dummy packet to %s\n", clientIPAndPort)
}

func (s *Server) HandleClientPackets(clientIPAndPort string) {
	connectionToClient := s.ServerToClientConnections[clientIPAndPort].Connection
	buffer := make([]byte, 1024*8)
	var packet Packet
	var n int
	var err error
	var shouldClose bool = false
	var clientActualAddress *net.UDPAddr
	var destinationPort uint16
	defer log.Printf("Closed connection to %s\n", clientIPAndPort)
	defer delete(s.ServerToClientConnections, clientIPAndPort)
	defer connectionToClient.Close()
	defer log.Printf("Sent close connection packet to %s\n", clientActualAddress.String())
	defer connectionToClient.Write([]byte{3, 0})
mainLoop:
	for {
		n, clientActualAddress, err = connectionToClient.ReadFromUDP(buffer)
		if err != nil {
			if shouldClose {
				break mainLoop
			}
			log.Printf("Error reading packet from %s\n%s\n", clientIPAndPort, err)
			shouldClose = true
			break mainLoop
		}

		// handle flags
		packet.DecodePacket(buffer[:n])
		if packet.Flags > 0 {
			if packet.Flags == 1 { // dummy
				s.ServerToClientConnections[clientIPAndPort].Ready = true
				log.Printf("Received dummy packet from %s\n", clientIPAndPort)
				s.ServerToClientConnections[clientIPAndPort].ActualAddress = clientActualAddress
				if clientActualAddress.String() != clientIPAndPort {
					log.Printf("Actual address for %s is %s\n", clientIPAndPort, clientActualAddress.String())
				}
			} else if packet.Flags == 2 { // keep-alive
				s.ServerToClientConnections[clientIPAndPort].LastReceivedPacketTime = time.Now().Unix()
			} else if packet.Flags == 3 { // close connection
				log.Printf("Received close connection packet from %s\n", clientIPAndPort)
				shouldClose = true
				break mainLoop
			} else if packet.Flags == 4 { // destination port announcement
				destinationPort = ByteSliceToUint16(packet.Payload)
				log.Printf("Received destination announcement packet with id %d for port %d\n", packet.ID, ByteSliceToUint16(packet.Payload))
			}
			continue mainLoop
		}

		if connectionToLocalApp, ok := s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID]; ok {
			_, err = connectionToLocalApp.Write(packet.Payload)
			if err != nil {
				if shouldClose {
					break mainLoop
				}
				log.Printf("Error writing packet to 0.0.0.0:%d\n%s\n", destinationPort, err)
				shouldClose = true
				break mainLoop
			}
		} else {
			s.ServerToClientConnections[clientIPAndPort].Ready = true

			s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID], err = net.DialUDP("udp", nil, resolveAddress(fmt.Sprintf("0.0.0.0:%d", destinationPort)))
			handleError(err)

			connectionToLocalApp := s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID]
			log.Printf("Created new connection to %s for packets with id %d\n", connectionToLocalApp.RemoteAddr().String(), packet.ID)

			_, err = connectionToLocalApp.Write(packet.Payload)
			if err != nil {
				if shouldClose {
					break mainLoop
				}
				log.Printf("Error writing packet to 0.0.0.0:%d\n%s\n", destinationPort, err)
				shouldClose = true
				break mainLoop
			}

			s.ServerToClientConnections[clientIPAndPort].LastReceivedPacketTime = time.Now().Unix()

			go func(id byte) {
				buffer := make([]byte, (1024*8)-2)
				var packet Packet
				var encodedPacketBytes []byte
				var n int
				var err error
				for {
					n, err = connectionToLocalApp.Read(buffer)
					if err != nil {
						if shouldClose {
							break
						}
						log.Printf("Error reading packet from %s\t\n%s\n", connectionToLocalApp.RemoteAddr().String(), err)
						shouldClose = true
						break
					}
					packet.Flags = 0
					packet.ID = id
					packet.Payload = buffer[:n]
					encodedPacketBytes = packet.EncodePacket()
					_, err = connectionToClient.WriteToUDP(encodedPacketBytes, clientActualAddress)
					if err != nil {
						if shouldClose {
							break
						}
						log.Printf("Error writing packet to %s\n%s\n", connectionToClient.RemoteAddr().String(), err)
						shouldClose = true
						break
					}
				}
			}(packet.ID)
		}
	}
}
