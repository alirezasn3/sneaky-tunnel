package main

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var config Config
var logFile *os.File

type Config struct {
	Role           string   `json:"role"`
	ListeningPorts []uint16 `json:"listeningPorts"`
	ClientDelay    int      `json:"clientDelay"`
	ServerIP       string   `json:"serverIP"`
	Negotiators    []string `json:"negotiators"`
}

func handleError(e error) {
	if e != nil {
		panic(e)
	}
}

func resolveAddress(adress string) *net.UDPAddr {
	a, err := net.ResolveUDPAddr("udp4", adress)
	handleError(err)
	return a
}

func getPortFromAddress(address string) string {
	parts := strings.Split(address, ":")
	return parts[len(parts)-1]
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

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func(*Client) {
			<-sigs
			c.CleanUp()
			os.Exit(0)
		}(&c)

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
