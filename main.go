package main

import (
	"encoding/json"
	"net"
	"os"
)

var config Config

type Config struct {
	Role        string   `json:"role"`
	ListenOn    string   `json:"listenOn"`
	ConnectTo   string   `json:"connectTo"`
	Server      string   `json:"server"`
	Negotiators []string `json:"negotiators"`
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
	if config.Role == "client" {
		var c Client
		c.GetPublicIP()
		c.SelectNegotiator()
		c.FindUnusedPort()
		c.NegotiatePorts()
		c.OpenPortAndSendDummyPacket()
		c.Start()
	} else if config.Role == "server" {
		var s Server
		s.ListenForNegotiationRequests()
	}
}
