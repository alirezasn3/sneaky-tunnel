package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
)

var config Config
var logFile *os.File

type Config struct {
	Role        string   `json:"role"`
	ListenOn    string   `json:"listenOn"`
	ConnectTo   string   `json:"connectTo"`
	Server      string   `json:"server"`
	ClientPort  string   `json:"clientPort"`
	Negotiators []string `json:"negotiators"`
}

func handleError(e error) {
	if e != nil {
		panic(e)
	}
}

func Log(message string) {
	log.Print(message)
	fmt.Print(message)
}

func resolveAddress(adress string) *net.UDPAddr {
	a, err := net.ResolveUDPAddr("udp", adress)
	handleError(err)
	return a
}

func init() {
	cPath := "config.json"
	if len(os.Args) > 1 {
		cPath = os.Args[1]
	}
	bytes, err := os.ReadFile(cPath)
	handleError(err)
	err = json.Unmarshal(bytes, &config)
	handleError(err)

	lPath := "logs.txt"
	if len(os.Args) > 2 {
		lPath = os.Args[2]
	}
	logFile, err = os.OpenFile(lPath, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	handleError(err)
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Lshortfile)
}

func main() {
	defer logFile.Close()

	if config.Role == "client" {
		var c Client
		c.GetPublicIP()
		c.SelectNegotiator()
		c.NegotiatePorts()
		c.OpenPortAndSendDummyPacket()
		c.Start()
	} else if config.Role == "server" {
		var s Server
		s.ListenForNegotiationRequests()
	}
}
