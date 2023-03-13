package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
)

var config Config
var clientConnections map[string]byte = make(map[string]byte)
var clientAdresses map[byte]*net.UDPAddr = make(map[byte]*net.UDPAddr)
var serverConnections map[byte]*net.UDPConn = make(map[byte]*net.UDPConn)

type Packet struct {
	Payload []byte // max length : 1024*8 - 1 - 1
	ID      byte   // max length : 1
	Flags   byte   // max length : 1
}

type Config struct {
	Role        string   `json:"role"`
	ListenOn    string   `json:"listenOn"`
	ConnectTo   string   `json:"connectTo"`
	Server      string   `json:"server"`
	Negotiators []string `json:"negotiators"`
}

func (p *Packet) EncodePacket() []byte {
	if len(p.Payload) > 8190 { // 8192 - 1 - 1
		panic("payload was larger than 8190 bytes")
	}
	var bytes []byte
	bytes = append(bytes, p.Flags)
	bytes = append(bytes, p.ID)
	bytes = append(bytes, p.Payload...)
	return bytes
}

func (p *Packet) DecodePacket(bytes []byte) {
	p.Flags = bytes[0]
	p.ID = bytes[1]
	p.Payload = bytes[2:]
}

func handleError(e error) {
	if e != nil {
		panic(e)
	}
}

func resolveAddress(adress string) *net.UDPAddr {
	a, err := net.ResolveUDPAddr("udp", adress)
	handleError(err)
	return a
}

func init() {
	bytes, err := os.ReadFile("config.json")
	handleError(err)
	err = json.Unmarshal(bytes, &config)
	handleError(err)
}

func main() {
	if os.Args[1] == "client" {
		remoteAddress := resolveAddress("5.78.79.102:23456")
		localAddress := resolveAddress("0.0.0.0:12345")
		listenAddress := resolveAddress("0.0.0.0:1194")

		tempConn, err := net.ListenPacket("udp", "0.0.0.0:12345")
		handleError(err)
		fmt.Println(remoteAddress.String())
		tempConn.WriteTo([]byte{0, 0}, remoteAddress)
		tempConn.Close()

		conn, err := net.DialUDP("udp", localAddress, remoteAddress)
		handleError(err)
		localConn, err := net.ListenUDP("udp", listenAddress)
		handleError(err)

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
				if id, ok := clientConnections[localAppAddress.String()]; ok {
					packet.ID = id
				} else {
					packet.ID = byte(len(clientConnections))
					clientConnections[localAppAddress.String()] = packet.ID
					clientAdresses[packet.ID] = localAppAddress
				}
				packet.Flags = 0
				packet.Payload = buffer[:n]
				encodedPacketBytes = packet.EncodePacket()
				_, err = conn.Write(encodedPacketBytes)
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
				_, err = localConn.WriteTo(packet.Payload, clientAdresses[packet.ID])
				handleError(err)
			}
		}()

		wg.Wait()
	} else if os.Args[1] == "server" {
		listenAddress := resolveAddress("0.0.0.0:23456")
		remoteAddress := resolveAddress(os.Args[2])
		localAddress := resolveAddress("0.0.0.0:1194")

		conn, err := net.DialUDP("udp", listenAddress, remoteAddress)
		handleError(err)

		buffer := make([]byte, 1024*8)
		var packet Packet
		var n int
		for {
			n, err = conn.Read(buffer)
			handleError(err)
			packet.DecodePacket(buffer[:n])
			if connectionToLocalApp, ok := serverConnections[packet.ID]; ok {
				_, err = connectionToLocalApp.Write(packet.Payload)
				handleError(err)
			} else {
				if len(packet.Payload) == 0 {
					continue
				}
				serverConnections[packet.ID], err = net.DialUDP("udp", nil, localAddress)
				handleError(err)
				connectionToLocalApp := serverConnections[packet.ID]
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
}
