// Licensed to You under the Apache License, Version 2.0.

package auth

import (
	"encoding/json"
	"log"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/disc"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus"
)

const (
	AuthTypeUsernamePassword = 1
	AuthTypeXAuthToken       = 2
	AuthTypeBearerToken      = 3
)

const (
	UNKNOWN = disc.UNKNOWN
	MSM     = disc.MSM
	EC      = disc.EC
	IDRAC   = disc.IDRAC
)

type Service struct {
	ServiceType int               `json:"serviceType"`
	Ip          string            `json:"ip"`
	AuthType    int               `json:"authType"`
	Auth        map[string]string `json:"auth"`
}

const (
	RESEND     = "resend"
	ADDSERVICE = "addservice"
	TERMINATE  = "terminate"
)

type Command struct {
	Command string  `json:"command"`
	Service Service `json:"service,omitempty"`
}

const (
	CommandQueue = "/authorization/command"
	EventQueue   = "/authorization"
)

type AuthorizationService struct {
	Bus messagebus.Messagebus
}
type AuthorizationClient struct {
	Bus messagebus.Messagebus
}

func (d *AuthorizationService) SendService(service Service) {
	jsonStr, _ := json.Marshal(service)
	err := d.Bus.SendMessage(jsonStr, EventQueue)
	if err != nil {
		log.Printf("Failed to send service %v", err)
	}
}

func (d *AuthorizationService) RecieveCommand(commands chan<- *Command) {
	messages := make(chan string, 10)

	go func() {
		_, err := d.Bus.ReceiveMessage(messages, CommandQueue)
		if err != nil {
			log.Printf("Error recieving messages %v", err)
		}
	}()
	for {
		message := <-messages
		command := new(Command)
		err := json.Unmarshal([]byte(message), command)
		if err != nil {
			log.Print("Error reading command queue: ", err)
			log.Printf("Message %#v\n", message)
		}
		commands <- command
	}
}

func (d *AuthorizationClient) SendCommand(command Command) {
	jsonStr, _ := json.Marshal(command)
	err := d.Bus.SendMessage(jsonStr, CommandQueue)
	if err != nil {
		log.Printf("Failed to send command %v", err)
	}
}

func (d *AuthorizationClient) SendCommandString(command string) {
	c := new(Command)
	c.Command = command
	d.SendCommand(*c)
}

func (d *AuthorizationClient) ResendAll() {
	d.SendCommandString(RESEND)
}

func (d *AuthorizationClient) AddService(service Service) {
	c := new(Command)
	c.Command = ADDSERVICE
	c.Service = service
	d.SendCommand(*c)
}

func (d *AuthorizationClient) GetService(services chan<- *Service) {
	messages := make(chan string, 10)

	go func() {
		_, err := d.Bus.ReceiveMessage(messages, EventQueue)
		if err != nil {
			log.Printf("Error recieving messages %v", err)
		}
	}()
	for {
		message := <-messages
		service := new(Service)
		err := json.Unmarshal([]byte(message), service)
		if err != nil {
			log.Print("Error reading auth queue: ", err)
		}
		services <- service
	}
}
