package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

type Server struct {
	ServerToClientConnections   map[string]*net.UDPConn
	ServerToLocalAppConnections map[byte]*net.UDPConn
}

func (s *Server) ListenForNegotiationRequests() {
	s.ServerToClientConnections = make(map[string]*net.UDPConn)
	s.ServerToLocalAppConnections = make(map[byte]*net.UDPConn)
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
			conn, err := net.DialUDP("udp4", nil, clientAddress)
			handleError(err)
			s.ServerToClientConnections[clientIPAndPort] = conn
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
	_, err := s.ServerToClientConnections[clientIPAndPort].Write([]byte{0, 0})
	handleError(err)
	fmt.Printf("Sent dummy packets to %s\n", clientIPAndPort)
}

func (s *Server) HandleClientPackets(clientIPAndPort string) {
	localAddress := resolveAddress(config.ConnectTo)

	conn := s.ServerToClientConnections[clientIPAndPort]

	buffer := make([]byte, 1024*8)
	var packet Packet
	var n int
	var err error
	for {
		n, err = conn.Read(buffer)
		handleError(err)
		packet.DecodePacket(buffer[:n])
		if connectionToLocalApp, ok := s.ServerToLocalAppConnections[packet.ID]; ok {
			_, err = connectionToLocalApp.Write(packet.Payload)
			handleError(err)
		} else {
			fmt.Println("Created new connection to local app")
			if len(packet.Payload) == 0 {
				continue
			}
			s.ServerToLocalAppConnections[packet.ID], err = net.DialUDP("udp4", nil, localAddress)
			handleError(err)
			connectionToLocalApp := s.ServerToLocalAppConnections[packet.ID]
			_, err = connectionToLocalApp.Write(packet.Payload)
			handleError(err)

			go func(id byte) {
				buffer := make([]byte, (1024*8)-2)
				var packet Packet
				var encodedPacketBytes []byte
				var n int
				for {
					n, err = connectionToLocalApp.Read(buffer)
					if err != nil {
						panic(err)
					}
					packet.Flags = 0
					packet.ID = id
					packet.Payload = buffer[:n]
					encodedPacketBytes = packet.EncodePacket()
					_, err = conn.Write(encodedPacketBytes)
					if err != nil {
						panic(err)
					}
				}
			}(packet.ID)
		}
	}
}
