// Licensed to You under the Apache License, Version 2.0.

package config

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

type SetFunc func(name string, value interface{}) error
type GetFunc func(name string) (interface{}, error)

type ConfigEntry struct {
	Set     SetFunc
	Get     GetFunc
	Default interface{}
}

const (
	GETPROPS = "getprops"
	GET      = "get"
	SET      = "set"
	RESET    = "reset"
)

type Command struct {
	Command       string      `json:"command"`
	ResponseQueue string      `json:"ReceiveQueue"`
	Property      string      `json:"property,omitempty"`
	Value         interface{} `json:"value,omitempty"`
}

type Response struct {
	Command  string      `json:"command"`
	Property string      `json:"property,omitempty"`
	Value    interface{} `json:"value,omitempty"`
	Error    error       `json:"error,omitempty"`
}

type ConfigService struct {
	Entries      map[string]*ConfigEntry
	CommandQueue string
	Bus          messagebus.Messagebus
}

type ConfigClient struct {
	Bus           messagebus.Messagebus
	CommandQueue  string
	ResponseQueue string
}

func NewConfigService(bus messagebus.Messagebus, commandQueue string, entries map[string]*ConfigEntry) *ConfigService {
	ret := new(ConfigService)
	ret.Entries = entries
	ret.CommandQueue = commandQueue
	ret.Bus = bus
	return ret
}

func NewConfigClient(bus messagebus.Messagebus, commandQueue string, responseQueue string) *ConfigClient {
	ret := new(ConfigClient)
	ret.CommandQueue = commandQueue
	ret.ResponseQueue = responseQueue
	ret.Bus = bus
	return ret
}

func (d *ConfigService) Run() {
	messages := make(chan string, 10)

	go func() {
		_, err := d.Bus.ReceiveMessage(messages, d.CommandQueue)
		if err != nil {
			log.Printf("Error recieving messages %v", err)
		}
	}()
	for {
		message := <-messages
		//log.Printf("Got message: %s\n", message)
		command := new(Command)
		err := json.Unmarshal([]byte(message), command)
		if err != nil {
			log.Print("Error reading config command queue: ", err)
		}
		switch command.Command {
		default:
			log.Print("Received unknown config command: ", command.Command)
		case GETPROPS:
			d.GetProperties(command)
		case GET:
			d.Get(command)
		case SET:
			d.Set(command)
		case RESET:
			d.Reset(command)
		}
	}
}

func (d *ConfigService) GetProperties(command *Command) {
	keys := make([]string, 0, len(d.Entries))
	for k := range d.Entries {
		keys = append(keys, k)
	}
	jsonStr, _ := json.Marshal(keys)
	err := d.Bus.SendMessage(jsonStr, command.ResponseQueue)
	log.Printf("Failed to send response %v", err)
}

func (d *ConfigService) Get(command *Command) {
	resp := new(Response)
	resp.Command = command.Command
	entry, ok := d.Entries[command.Property]
	if !ok {
		resp.Error = fmt.Errorf("Could not find property named %s", command.Property)
	} else {
		value, err := entry.Get(command.Property)
		if err != nil {
			resp.Error = err
		} else {
			resp.Value = value
		}
	}
	jsonStr, _ := json.Marshal(resp)
	//log.Println(string(jsonStr))
	err := d.Bus.SendMessage(jsonStr, command.ResponseQueue)
	if err != nil {
		log.Printf("Failed to send response %v", err)
	}
}

func (d *ConfigService) Set(command *Command) {
	resp := new(Response)
	resp.Command = command.Command
	entry, ok := d.Entries[command.Property]
	if !ok {
		resp.Error = fmt.Errorf("Could not find property named %s", command.Property)
	} else {
		err := entry.Set(command.Property, command.Value)
		if err != nil {
			resp.Error = err
		} else {
			resp.Value = command.Value
		}
	}
	jsonStr, _ := json.Marshal(resp)
	err := d.Bus.SendMessage(jsonStr, command.ResponseQueue)
	if err != nil {
		log.Printf("Failed to send response %v", err)
	}
}

func (d *ConfigService) Reset(command *Command) {
	resp := new(Response)
	resp.Command = command.Command
	entry, ok := d.Entries[command.Property]
	if !ok {
		resp.Error = fmt.Errorf("Could not find property named %s", command.Property)
	} else {
		err := entry.Set(command.Property, entry.Default)
		if err != nil {
			resp.Error = err
		} else {
			resp.Value = entry.Default
		}
	}
	jsonStr, _ := json.Marshal(resp)
	err := d.Bus.SendMessage(jsonStr, command.ResponseQueue)
	if err != nil {
		log.Printf("Failed to send response %v", err)
	}
}

func (d *ConfigClient) SendCommand(command Command) error {
	jsonStr, _ := json.Marshal(command)
	return d.Bus.SendMessage(jsonStr, d.CommandQueue)
}

func (d *ConfigClient) ReadOneMessage() string {
	messages := make(chan string)
	sub, err := d.Bus.ReceiveMessage(messages, d.ResponseQueue)
	if err != nil {
		log.Println("Error receiving message: ", err)
		return ""
	}
	message := <-messages
	//log.Println("Got message: ", message)
	sub.Close()
	return message
}

func (d *ConfigClient) GetProperties() ([]string, error) {
	var command Command
	command.Command = GETPROPS
	command.ResponseQueue = d.ResponseQueue
	err := d.SendCommand(command)
	if err != nil {
		return nil, err
	}
	message := d.ReadOneMessage()
	var props []string
	err = json.Unmarshal([]byte(message), &props)
	if err != nil {
		return nil, err
	}
	return props, nil
}

func (d *ConfigClient) Get(name string) (*Response, error) {
	var command Command
	command.Command = GET
	command.ResponseQueue = d.ResponseQueue
	command.Property = name
	err := d.SendCommand(command)
	if err != nil {
		return nil, err
	}
	message := d.ReadOneMessage()
	prop := new(Response)
	err = json.Unmarshal([]byte(message), prop)
	if err != nil {
		return nil, err
	}
	return prop, nil
}

func (d *ConfigClient) Set(name string, value interface{}) (*Response, error) {
	var command Command
	command.Command = SET
	command.ResponseQueue = d.ResponseQueue
	command.Property = name
	command.Value = value
	err := d.SendCommand(command)
	if err != nil {
		return nil, err
	}
	message := d.ReadOneMessage()
	prop := new(Response)
	err = json.Unmarshal([]byte(message), prop)
	if err != nil {
		return nil, err
	}
	return prop, nil
}

func (d *ConfigClient) Reset(name string) (*Response, error) {
	var command Command
	command.Command = RESET
	command.ResponseQueue = d.ResponseQueue
	command.Property = name
	err := d.SendCommand(command)
	if err != nil {
		return nil, err
	}
	message := d.ReadOneMessage()
	prop := new(Response)
	err = json.Unmarshal([]byte(message), prop)
	if err != nil {
		return nil, err
	}
	return prop, nil
}
