// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"flag"
	"log"
	"os"
	"strings"
	"time"
	"gopkg.in/ini.v1"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/disc"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

var services []disc.Service

func main() {

	configName := flag.String("config", "config.ini", "The configuration ini file")

	flag.Parse()

	config, err := ini.Load(*configName)
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}

	stompHost := config.Section("General").Key("StompHost").MustString("0.0.0.0")
	stompPort := config.Section("General").Key("StompPort").MustInt(61613)

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
	log.Print("StompHost: ", stompHost)
	log.Print("StompPort: ", stompPort)
	log.Print("Services: ", services)

	discoveryService := new(disc.DiscoveryService)
	for { 	
		mb, err := stomp.NewStompMessageBus(stompHost, stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: ", err)
		        time.Sleep(5 * time.Second)
		} else {
			discoveryService.Bus = mb
			defer mb.Close()
			break
		}
	}
	commands := make(chan *disc.Command)

	for _, element := range services {
		go func(elem disc.Service) {
			err := discoveryService.SendService(elem)
			if err != nil {
				log.Printf("Failed sending service %v %v", elem, err)
			}
		}(element)
	}

	go discoveryService.RecieveCommand(commands)
	for {
		command := <-commands
		log.Printf("Recieved command: %s", command.Command)
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
