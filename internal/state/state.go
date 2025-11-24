package state

import (
	"encoding/json"
	"log"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

const (
	CommandQueue = "/status/command"
	EventQueue   = "/status"

	UPDATESERVICESTATUS     = "updateservicestatus"
	UPDATESERVICEITEMSTATUS = "updateserviceitemstatus"
)

type Command struct {
	Command string `json:"command"`
}

type StateService struct {
	Bus messagebus.Messagebus
}

func (s *StateService) ReceiveCommand(commands chan<- *Command) error {
	messages := make(chan string, 10)

	go func() {
		_, err := s.Bus.ReceiveMessage(messages, CommandQueue)
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
}

type StateClient struct {
	Bus messagebus.Messagebus
}

func (s *StateClient) SendCommand(command Command) error {
	jsonStr, _ := json.Marshal(command)
	err := s.Bus.SendMessage(jsonStr, CommandQueue)
	if err != nil {
		log.Printf("Failed to send command %v", err)
	}
	return err
}

func (s *StateClient) SendCommandString(command string) {
	c := new(Command)
	c.Command = command
	s.SendCommand(*c)
}

func (s *StateClient) UpdateServiceStatus(service auth.Service) error {
	c := new(Command)
	c.Command = UPDATESERVICESTATUS
	return s.SendCommand(*c)
}

func (s *StateClient) UpdateServiceItemStatus(si auth.ServiceItem) error {
	c := new(Command)
	c.Command = UPDATESERVICEITEMSTATUS
	return s.SendCommand(*c)
}
