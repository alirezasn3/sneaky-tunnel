package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var config Config
var logFile *os.File
var httpClient *http.Client

type Config struct {
	Role           string   `json:"role"`
	ListeningPorts []uint16 `json:"listeningPorts"`
	ServerIP       string   `json:"serverIP"`
	Negotiators    []string `json:"negotiators"`
	Resolver       string   `json:"resolver"`
}

func resolveAddress(adress string) *net.UDPAddr {
	a, err := net.ResolveUDPAddr("udp4", adress)
	if err != nil {
		log.Panic(err)
	}
	return a
}

func getPortFromAddress(address string) string {
	parts := strings.Split(address, ":")
	return parts[len(parts)-1]
}

func getIPFromAddress(address string) string {
	parts := strings.Split(address, ":")
	return parts[0]
}

func isValidAddress(address string) bool {
	if len(address) > 21 {
		return false
	}
	addressParts := strings.Split(address, ":")
	if len(addressParts) != 2 {
		return false
	}
	ipParts := strings.Split(addressParts[0], ".")
	if len(ipParts) != 4 {
		return false
	}
	_, err := strconv.ParseUint(addressParts[1], 10, 16)
	if err != nil {
		return false
	}
	ip := net.ParseIP(addressParts[0])
	return ip != nil
}

func init() {
	cPath := "config.json"
	if len(os.Args) > 1 {
		cPath = os.Args[1]
	}
	bytes, err := os.ReadFile(cPath)
	if err != nil {
		log.Panic(err)
	}
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		log.Panic(err)
	}

	lPath := "logs.txt"
	if len(os.Args) > 2 {
		lPath = os.Args[2]
	}
	logFile, err = os.OpenFile(lPath, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Panic(err)
	}
	log.SetOutput(logFile)
	log.SetFlags(log.Ltime | log.Lshortfile)

	dialer := &net.Dialer{
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Second * 10,
				}
				return d.DialContext(ctx, "udp", config.Resolver+":53")
			},
		},
	}
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, addr)
	}
	http.DefaultTransport.(*http.Transport).DialContext = dialContext
	httpClient = &http.Client{
		Timeout:   time.Second * 5,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
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
		c.SelectNegotiator()
		c.NegotiatePorts()
		c.OpenPortAndSendDummyPacket()
		c.Start()
	} else if config.Role == "server" {
		(&Server{}).ListenForNegotiationRequests()
	}
}
