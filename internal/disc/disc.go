// Licensed to You under the Apache License, Version 2.0.

package disc

import (
	"encoding/json"
	"log"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

const (
	UNKNOWN = 0
	MSM     = 1
	EC      = 2
	IDRAC   = 3
)

type Service struct {
	ServiceType int    `json:"serviceType"`
	Ip          string `json:"ip"`
}

const (
	RESEND    = "resend"
	TERMINATE = "terminate"
)

type Command struct {
	Command string `json:"command"`
}

const (
	CommandQueue = "/discovery/command"
	EventQueue   = "/discovery"
)

type DiscoveryService struct {
	Bus messagebus.Messagebus
}
type DiscoveryClient struct {
	Bus messagebus.Messagebus
}

func (d *DiscoveryService) SendService(service Service) error {
	jsonStr, _ := json.Marshal(service)
	err := d.Bus.SendMessage(jsonStr, EventQueue)
	if err != nil {
		log.Print("Error sending message: ", err)
	}
	return err
}

func (d *DiscoveryService) ReceiveCommand(commands chan<- *Command) {
	messages := make(chan string, 10)

	go func() {
		_, err := d.Bus.ReceiveMessage(messages, CommandQueue)
		if err != nil {
			log.Printf("Error receiving messages %v", err)
		}
	}()
	for {
		message := <-messages
		command := new(Command)
		err := json.Unmarshal([]byte(message), command)
		if err != nil {
			log.Print("Error reading command queue: ", err)
		}
		commands <- command
	}
}

func (d *DiscoveryClient) SendCommand(command Command) {
	jsonStr, _ := json.Marshal(command)
	err := d.Bus.SendMessage(jsonStr, CommandQueue)
	if err != nil {
		log.Printf("Failed to send command %v", err)
	}
}

func (d *DiscoveryClient) SendCommandString(command string) {
	c := new(Command)
	c.Command = command
	d.SendCommand(*c)
}

func (d *DiscoveryClient) ResendAll() {
	d.SendCommandString(RESEND)
}

func (d *DiscoveryClient) GetService(services chan<- *Service) {
	messages := make(chan string, 10)

	go func() {
		_, err := d.Bus.ReceiveMessage(messages, EventQueue)
		if err != nil {
			log.Printf("Error receiving messages %v", err)
		}
	}()
	for {
		message := <-messages
		service := new(Service)
		err := json.Unmarshal([]byte(message), service)
		if err != nil {
			log.Print("Error reading discovery queue: ", err)
		}
		services <- service
	}
}
