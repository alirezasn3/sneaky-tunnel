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
	Ready                  bool
}

type Server struct {
	ServerToClientConnections map[string]*User
}

func (s *Server) ListenForNegotiationRequests() {
	s.ServerToClientConnections = make(map[string]*User)
	err := http.ListenAndServe("0.0.0.0:80", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Log(fmt.Sprintf("%s %s\n", r.Method, r.URL.String()))
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
					Log(fmt.Sprintf("Closed Connection to %s because client did not send a packet in time\n", clientIPAndPort))
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
	_, err := s.ServerToClientConnections[clientIPAndPort].Connection.Write([]byte{1, 0, 0, 0})
	handleError(err)
	Log(fmt.Sprintf("Sent dummy packets to %s\n", clientIPAndPort))
}

func (s *Server) HandleClientPackets(clientIPAndPort string) {
	localAddress := resolveAddress(fmt.Sprintf("0.0.0.0:%d", config.AppPort))
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
				if c, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
					c.Connection.Close()
					c.ConnectionsToLocalApp[packet.ID].Close()
					delete(s.ServerToClientConnections, clientIPAndPort)
				}
				break
			}
			Log(fmt.Sprintf("Error reading packet from %s\n%s\n", clientIPAndPort, err))
			continue
		}

		// handle flags
		packet.DecodePacket(buffer[:n])
		if packet.Flags > 0 {
			if packet.Flags == 1 { // dummy
				s.ServerToClientConnections[clientIPAndPort].Ready = true
				Log(fmt.Sprintf("Received dummy packet from %s\n", clientIPAndPort))
			} else if packet.Flags == 2 { // keep-alive
				s.ServerToClientConnections[clientIPAndPort].LastReceivedPacketTime = time.Now().Unix()
			} else if packet.Flags == 3 { // close connection
				Log(fmt.Sprintf("Received close connection packet from %s\n", clientIPAndPort))
				shouldClose = true
				if c, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
					c.Connection.Close()
					c.ConnectionsToLocalApp[packet.ID].Close()
					delete(s.ServerToClientConnections, clientIPAndPort)
				}
				break
			}
			continue
		}

		if connectionToLocalApp, ok := s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID]; ok {
			_, err = connectionToLocalApp.Write(packet.Payload)
			if err != nil {
				if shouldClose {
					if c, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
						c.Connection.Close()
						c.ConnectionsToLocalApp[packet.ID].Close()
						delete(s.ServerToClientConnections, clientIPAndPort)
					}
					break
				}
				Log(fmt.Sprintf("Error writing packet to 0.0.0.0:%d\n%s\n", config.AppPort, err))
				continue
			}
		} else {
			s.ServerToClientConnections[clientIPAndPort].Ready = true
			Log("Created new connection to local app\n")
			if len(packet.Payload) == 0 {
				continue
			}

			s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID], err = net.DialUDP("udp", nil, localAddress)
			handleError(err)

			connectionToLocalApp := s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID]

			_, err = connectionToLocalApp.Write(packet.Payload)
			if err != nil {
				if shouldClose {
					if c, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
						c.Connection.Close()
						c.ConnectionsToLocalApp[packet.ID].Close()
						delete(s.ServerToClientConnections, clientIPAndPort)
					}
					break
				}
				Log(fmt.Sprintf("Error writing packet to 0.0.0.0:%d\n%s\n", config.AppPort, err))
				continue
			}

			s.ServerToClientConnections[clientIPAndPort].LastReceivedPacketTime = time.Now().Unix()

			go func() {
				for {
					if shouldClose {
						if c, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
							c.Connection.Close()
							c.ConnectionsToLocalApp[packet.ID].Close()
							delete(s.ServerToClientConnections, clientIPAndPort)
						}
						break
					}
					if time.Now().Unix()-s.ServerToClientConnections[clientIPAndPort].LastReceivedPacketTime > 10 {
						Log(fmt.Sprintf("Client %s timed out, closing the connection\n", clientIPAndPort))
						shouldClose = true
						s.ServerToClientConnections[clientIPAndPort].Connection.Close()
						s.ServerToClientConnections[clientIPAndPort].ConnectionsToLocalApp[packet.ID].Close()
						delete(s.ServerToClientConnections, clientIPAndPort)
						break
					}
					time.Sleep(time.Second * 10)
				}
			}()

			go func(id byte) {
				buffer := make([]byte, (1024*8)-4)
				var packet Packet
				var encodedPacketBytes []byte
				var n int
				var err error
				for {
					n, err = connectionToLocalApp.Read(buffer)
					if err != nil {
						if shouldClose {
							if c, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
								c.Connection.Close()
								c.ConnectionsToLocalApp[packet.ID].Close()
								delete(s.ServerToClientConnections, clientIPAndPort)
							}
							break
						}
						Log(fmt.Sprintf("Error reading packet from %s\n%s\n", localAddress, err))
						continue
					}
					packet.Flags = 0
					packet.ID = id
					packet.Payload = buffer[:n]
					encodedPacketBytes = packet.EncodePacket()
					_, err = conn.Write(encodedPacketBytes)
					if err != nil {
						if shouldClose {
							if c, ok := s.ServerToClientConnections[clientIPAndPort]; ok {
								c.Connection.Close()
								c.ConnectionsToLocalApp[packet.ID].Close()
								delete(s.ServerToClientConnections, clientIPAndPort)
							}
							break
						}
						Log(fmt.Sprintf("Error writing packet to %s\n%s\n", conn.RemoteAddr().String(), err))
						continue
					}
				}
			}(packet.ID)
		}
	}
}
