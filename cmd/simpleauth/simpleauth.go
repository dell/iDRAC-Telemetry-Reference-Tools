// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"gopkg.in/ini.v1"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/disc"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
)

var configStrings = map[string]string{
	"mbhost": "activemq",
	"mbport": "61613",
}

var authServices map[string]auth.Service

func handleDiscServiceChannel(serviceIn chan *disc.Service, config *ini.File, authorizationService *auth.AuthorizationService) {
	for {
		service := <-serviceIn
		//log.Print("Service = ", service)
		devconfig, err := config.GetSection(service.Ip)
		if err != nil {
			log.Print(err)
			continue
		}
		authService := new(auth.Service)
		authService.ServiceType = service.ServiceType
		authService.Ip = service.Ip
		if authService.ServiceType == auth.EC {
			sshconfig := &ssh.ClientConfig{
				User: devconfig.Key("username").MustString(""),
				Auth: []ssh.AuthMethod{
					ssh.Password(devconfig.Key("password").MustString("")),
				},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			}
			serviceName := service.Ip
			if strings.Contains(service.Ip, ":") {
				split := strings.Split(service.Ip, ":")
				serviceName = split[0]
			}
			client, err := ssh.Dial("tcp", serviceName+":22", sshconfig)
			if err != nil {
				log.Print("Failed to dial: ", err)
				continue
			}
			session, err := client.NewSession()
			if err != nil {
				log.Print("Failed to create session: ", err)
				continue
			}
			var b bytes.Buffer
			session.Stdout = &b
			if err := session.Run("/usr/bin/hapitest -e"); err != nil {
				log.Print("Failed to run: " + err.Error())
				session.Close()
				continue
			}
			session.Close()
			str := b.String()
			if !strings.Contains(str, "Local  EC Active State   = 1") {
				log.Printf("EC at %s is not active. Skipping...\n", service.Ip)
				continue
			}
			session, err = client.NewSession()
			if err != nil {
				log.Print("Failed to create session: ", err)
				continue
			}
			session.Stdout = &b
			if err := session.Run("/usr/bin/oauthtest token"); err != nil {
				log.Print("Failed to run: " + err.Error())
				session.Close()
				continue
			}
			session.Close()
			str = b.String()
			parts := strings.Split(str, "Local device token : ")
			authService.AuthType = auth.AuthTypeBearerToken
			authService.Auth = make(map[string]string)
			authService.Auth["token"] = strings.TrimSpace(parts[1])
		} else {
			key, err := devconfig.GetKey("username")
			if err != nil {
				//TODO get token
			} else {
				authService.AuthType = auth.AuthTypeUsernamePassword
				authService.Auth = make(map[string]string)
				authService.Auth["username"] = key.String()
				authService.Auth["password"] = devconfig.Key("password").MustString("")
			}
		}
		//log.Print("Got Service = ", *authService)
		_ = authorizationService.SendService(*authService)
		if authServices == nil {
			authServices = make(map[string]auth.Service)
		}
		authServices[service.Ip] = *authService
	}
}

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

	discoveryClient := new(disc.DiscoveryClient)
	authorizationService := new(auth.AuthorizationService)

	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			discoveryClient.Bus = mb
			authorizationService.Bus = mb
			defer mb.Close()
			break
		}
	}
	serviceIn := make(chan *disc.Service, 10)
	commands := make(chan *auth.Command)

	log.Print("Auth Service is initialized")

	discoveryClient.ResendAll()
	go discoveryClient.GetService(serviceIn)
	go handleDiscServiceChannel(serviceIn, config, authorizationService)
	go authorizationService.ReceiveCommand(commands) //nolint: errcheck
	for {
		command := <-commands
		log.Printf("in simpleauth, Received command: %s", command.Command)
		switch command.Command {
		case auth.RESEND:
			for _, element := range authServices {
				go authorizationService.SendService(element) //nolint: errcheck
			}
		case auth.TERMINATE:
			os.Exit(0)
		}
	}
}
