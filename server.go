package main

import (
	"fmt"
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
}

type Server struct {
	ServerToClientConnections map[string]*User
}

func (s *Server) ListenForNegotiationRequests() {
	s.ServerToClientConnections = make(map[string]*User)
	err := http.ListenAndServe("0.0.0.0:80", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("%s %s\n", r.Method, r.URL.String())
		if r.Method == "GET" {
			urlParts := strings.Split(r.URL.String(), "/")
			clientIPAndPort := urlParts[len(urlParts)-1]
			if _, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
				w.WriteHeader(400)
				return
			}
			clientAddress := resolveAddress(clientIPAndPort)
			conn, err := net.DialUDP("udp", nil, clientAddress)
			handleError(err)
			s.ServerToClientConnections[clientIPAndPort] = &User{}
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
	_, err := s.ServerToClientConnections[clientIPAndPort].Connection.Write([]byte{0, 0})
	handleError(err)
	fmt.Printf("Sent dummy packets to %s\n", clientIPAndPort)
}

func (s *Server) HandleClientPackets(clientIPAndPort string) {
	localAddress := resolveAddress(config.ConnectTo)
	conn := s.ServerToClientConnections[clientIPAndPort].Connection
	buffer := make([]byte, 1024*8)
	var packet Packet
	var n int
	var err error
	var shouldClose bool = false
	for {
		n, err = conn.Read(buffer)
		if err != nil {
			if shouldClose {
				break
			}
			fmt.Printf("Error reading packet from %s\n%s\n", clientIPAndPort, err)
			continue
		}
		packet.DecodePacket(buffer[:n])
		if packet.Flags == 1 { // keep alive packet
			s.ServerToClientConnections[clientIPAndPort].LastReceivedPacketTime = time.Now().Unix()
			continue
		}
		if connectionToLocalApp, ok := s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID]; ok {
			_, err = connectionToLocalApp.Write(packet.Payload)
			if err != nil {
				if shouldClose {
					break
				}
				fmt.Printf("Error writing packet to %s\n%s\n", config.ConnectTo, err)
				continue
			}
		} else {
			fmt.Println("Created new connection to local app")
			if len(packet.Payload) == 0 {
				continue
			}

			s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID], err = net.DialUDP("udp", nil, localAddress)
			handleError(err)

			connectionToLocalApp := s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID]

			_, err = connectionToLocalApp.Write(packet.Payload)
			if err != nil {
				if shouldClose {
					break
				}
				fmt.Printf("Error writing packet to %s\n%s\n", config.ConnectTo, err)
				continue
			}

			go func() {
				for {
					time.Sleep(time.Second * 10)
					if time.Now().Unix()-s.ServerToClientConnections[clientIPAndPort].LastReceivedPacketTime > 10 {
						fmt.Printf("Client %s timed out, closing the connection\n", clientIPAndPort)
						shouldClose = true
						s.ServerToClientConnections[clientIPAndPort].Connection.Close()
						delete(s.ServerToClientConnections, clientIPAndPort)
						return
					}
				}
			}()

			go func(id byte) {
				buffer := make([]byte, (1024*8)-2)
				var packet Packet
				var encodedPacketBytes []byte
				var n int
				for {
					n, err = connectionToLocalApp.Read(buffer)
					if err != nil {
						if shouldClose {
							break
						}
						fmt.Printf("Error reading packet from %s\n%s\n", localAddress, err)
						continue
					}
					packet.Flags = 0
					packet.ID = id
					packet.Payload = buffer[:n]
					encodedPacketBytes = packet.EncodePacket()
					_, err = conn.Write(encodedPacketBytes)
					if err != nil {
						if shouldClose {
							break
						}
						fmt.Printf("Error writing packet to %s\n%s\n", conn.RemoteAddr().String(), err)
						continue
					}
				}
			}(packet.ID)
		}
	}
}
