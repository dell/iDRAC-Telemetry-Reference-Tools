// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"flag"
	"gopkg.in/ini.v1"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/disc"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

var configStrings = map[string]string{
	"mbhost": "activemq",
	"mbport": "61613",
}

var services []disc.Service

func getEnvSettings() {
	mbHost := os.Getenv("MESSAGEBUS_HOST")
	if len(mbHost) > 0 {
		configStrings["mbhost"] = mbHost
	}
	mbPort := os.Getenv("MESSAGEBUS_PORT")
	if len(mbPort) > 0 {
		configStrings["mbport"] = mbPort
	}
}

func main() {

	configName := flag.String("config", "config.ini", "The configuration ini file")

	flag.Parse()

	config, err := ini.Load(*configName)
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}

	//Gather configuration from environment variables
	getEnvSettings()

	types := config.Section("Services").Key("Types").Strings(",")
	ips := config.Section("Services").Key("IPs").Strings(",")

	for index, element := range types {
		s := new(disc.Service)
		if strings.EqualFold(element, "MSM") {
			s.ServiceType = disc.MSM
		} else if strings.EqualFold(element, "EC") {
			s.ServiceType = disc.EC
		} else if strings.EqualFold(element, "iDRAC") {
			s.ServiceType = disc.IDRAC
		} else {
			s.ServiceType = disc.UNKNOWN
		}
		s.Ip = ips[index]
		services = append(services, *s)
	}
	log.Print("Services: ", services)

	discoveryService := new(disc.DiscoveryService)
	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			discoveryService.Bus = mb
			defer mb.Close()
			break
		}
	}
	commands := make(chan *disc.Command)

	log.Print("Discovery Service is initialized")

	for _, element := range services {
		go func(elem disc.Service) {
			err := discoveryService.SendService(elem)
			if err != nil {
				log.Printf("Failed sending service %v %v", elem, err)
			}
		}(element)
	}

	go discoveryService.ReceiveCommand(commands)
	for {
		command := <-commands
		log.Printf("in simpledisc Received command: %s", command.Command)
		switch command.Command {
		case disc.RESEND:
			for _, element := range services {
				go func(elem disc.Service) {
					err := discoveryService.SendService(elem)
					if err != nil {
						log.Printf("Failed resending service %v %v", elem, err)
					}
				}(element)
			}
		case disc.TERMINATE:
			os.Exit(0)
		}
	}
}
