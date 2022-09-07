// Licensed to You under the Apache License, Version 2.0.

package auth

import (
	"encoding/json"
	"log"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/disc"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
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
	RESEND        = "resend"
	ADDSERVICE    = "addservice"
	DELETESERVICE = "deleteservice"
	TERMINATE     = "terminate"
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

func (d *AuthorizationService) SendService(service Service) error {
	jsonStr, _ := json.Marshal(service)
	err := d.Bus.SendMessage(jsonStr, EventQueue)
	if err != nil {
		log.Printf("Failed to send service %v", err)
	}
	return err
}

func (d *AuthorizationService) ReceiveCommand(commands chan<- *Command) error {
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
			return err
		}
		commands <- command
	}
	return nil
}

func (d *AuthorizationClient) SendCommand(command Command) error {
	jsonStr, _ := json.Marshal(command)
	err := d.Bus.SendMessage(jsonStr, CommandQueue)
	if err != nil {
		log.Printf("Failed to send command %v", err)
	}
	return err
}

func (d *AuthorizationClient) SendCommandString(command string) {
	c := new(Command)
	c.Command = command
	d.SendCommand(*c)
}

func (d *AuthorizationClient) ResendAll() {
	d.SendCommandString(RESEND)
}

func (d *AuthorizationClient) AddService(service Service) error {
	c := new(Command)
	c.Command = ADDSERVICE
	c.Service = service
	return d.SendCommand(*c)
}

func (d *AuthorizationClient) DeleteService(service Service) error {
	c := new(Command)
	c.Command = DELETESERVICE
	c.Service = service
	return d.SendCommand(*c)
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
