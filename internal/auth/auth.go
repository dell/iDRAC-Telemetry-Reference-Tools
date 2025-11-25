// Licensed to You under the Apache License, Version 2.0.

package auth

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/disc"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

const (
	AuthTypeUsernamePassword = 1
	AuthTypeXAuthToken       = 2
	AuthTypeBearerToken      = 3
)

// Service states
const (
	STARTING     = "Starting"
	RUNNING      = "Running"
	RUNNINGWOTEL = "Running Only Alerts"
	TELNOTFOUND  = "Telemetry Service Not Found"
	CONNFAILED   = "Connection Failed"
	LEAKED       = "Leaked"
)

// ServiceItem states
const (
	SHUTDOWNSENT   = "Shutdown Sent"
	SHUTDOWNFAILED = "Shutdown Failed"
)

const (
	UNKNOWN = disc.UNKNOWN
	MSM     = disc.MSM
	EC      = disc.EC
	IDRAC   = disc.IDRAC
	IRC     = disc.IRC
)

type Service struct {
	ServiceType int               `json:"serviceType"`
	Ip          string            `json:"ip"`
	AuthType    int               `json:"authType"`
	Auth        map[string]string `json:"auth"`
	State       string            `json:"state"`
}

type ServiceItem struct {
	Service
	ServiceIP string `json:"serviceIp"` // IP of the service this item belongs to
}

const (
	RESEND        = "resend"
	ADDSERVICE    = "addservice"
	DELETESERVICE = "deleteservice"
	UPDATESERVICE = "updateservice"
	TERMINATE     = "terminate"
	SPLUNKADDHEC  = "splunkaddhec"
	GETHECCONFIG  = "gethecconfig"

	ADDSERVICEITEM    = "addserviceitem"
	DELETESERVICEITEM = "deleteserviceitem"
	GETSERVICEITEMS   = "getserviceitems"
	UPDATESERVICEITEM = "updateserviceitem"
)

type SplunkConfig struct {
	Url   string `json:"url,omitempty"`
	Key   string `json:"key,omitempty"`
	Index string `json:"index,omitempty"`
}

type Command struct {
	Command      string       `json:"command"`
	SplunkConfig SplunkConfig `json:"Splunkconfig,omitempty"`
	Service      Service      `json:"service,omitempty"`
	ServiceItem  ServiceItem  `json:"serviceitem,omitempty"`
	ReceiveQueue string       `json:"receivequeue,omitempty"`
}

type response struct {
	Command  string      `json:"command"`
	DataType string      `json:"dataType"`
	Data     interface{} `json:"data"`
}

const (
	CommandQueue = "/authorization/command"
	EventQueue   = "/authorization"
)

type AuthClientInterface interface {
	AddService(service Service) error
	AddServiceItem(si ServiceItem) error
	DeleteService(service Service) error
	DeleteServiceItem(si ServiceItem) error
	GetHECConfig()
	GetService(services chan<- *Service)
	GetServiceItems(sip string) []ServiceItem
	GetServiceWithIP(ip string) Service
	ReadOneMessage(queue string, v any) error
	ResendAll()
	SendCommand(command Command) error
	SendCommandString(command string)
	SplunkAddHEC(SplunkHttp SplunkConfig) error
	UpdateService(s Service) error
	UpdateServiceItem(si ServiceItem) error
	UpdateServiceItemState(state string, siip string) error
	UpdateServiceState(state string, sip string) error
}

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

func (a *AuthorizationClient) GetHECConfig() {
	a.SendCommandString(GETHECCONFIG)
}

func (d *AuthorizationService) Sendconfig(config SplunkConfig) error {
	jsonStr, _ := json.Marshal(config)
	err := d.Bus.SendMessage(jsonStr, EventQueue)
	if err != nil {
		log.Printf("Failed to send service %v", err)
	}
	return err

}

func (a *AuthorizationClient) SendCommand(command Command) error {
	jsonStr, _ := json.Marshal(command)
	err := a.Bus.SendMessage(jsonStr, CommandQueue)
	if err != nil {
		log.Printf("Failed to send command %v", err)
	}
	return err
}

func (a *AuthorizationClient) SendCommandString(command string) {
	c := new(Command)
	c.Command = command
	a.SendCommand(*c)
}

func (a *AuthorizationClient) ResendAll() {
	a.SendCommandString(RESEND)
}

func (a *AuthorizationClient) SplunkAddHEC(SplunkHttp SplunkConfig) error {
	c := new(Command)
	c.Command = SPLUNKADDHEC
	c.SplunkConfig = SplunkHttp
	return a.SendCommand(*c)
}

func (a *AuthorizationClient) AddService(service Service) error {
	c := new(Command)
	c.Command = ADDSERVICE
	c.Service = service
	return a.SendCommand(*c)
}

func (a *AuthorizationClient) DeleteService(service Service) error {
	c := new(Command)
	c.Command = DELETESERVICE
	c.Service = service
	return a.SendCommand(*c)
}

func (a *AuthorizationClient) GetService(services chan<- *Service) {
	messages := make(chan string, 10)

	go func() {
		_, err := a.Bus.ReceiveMessage(messages, EventQueue)
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

func (a *AuthorizationClient) UpdateService(s Service) error {
	c := new(Command)
	c.Command = UPDATESERVICE
	c.Service = s
	return a.SendCommand(*c)
}

func (a *AuthorizationClient) UpdateServiceState(state string, sip string) error {
	switch state {
	case CONNFAILED, STARTING, RUNNING, TELNOTFOUND, RUNNINGWOTEL:
		a.UpdateService(
			Service{
				Ip:    sip,
				State: state,
			},
		)
	default:
		return fmt.Errorf("invalid state %s", state)
	}
	return nil
}

func (a *AuthorizationClient) GetServiceWithIP(ip string) Service {
	recvQueue := "/authorization/services/" + ip
	c := Command{
		Command:      GETSERVICEITEMS,
		ReceiveQueue: recvQueue,
		Service: Service{
			Ip: ip,
		},
	}
	a.SendCommand(c)

	service := Service{}
	err := a.ReadOneMessage(recvQueue, &service)
	if err != nil {
		log.Print("Error getting service with ip: ", ip, " err: ", err)
		return Service{}
	}
	return service
}

func (a *AuthorizationClient) AddServiceItem(si ServiceItem) error {
	c := new(Command)
	c.Command = ADDSERVICEITEM
	c.ServiceItem = si
	return a.SendCommand(*c)
}

func (a *AuthorizationClient) DeleteServiceItem(si ServiceItem) error {
	c := new(Command)
	c.Command = DELETESERVICEITEM
	c.ServiceItem = si
	return a.SendCommand(*c)
}

func (a *AuthorizationClient) UpdateServiceItem(si ServiceItem) error {
	c := new(Command)
	c.Command = UPDATESERVICEITEM
	c.ServiceItem = si
	return a.SendCommand(*c)
}

func (a *AuthorizationClient) UpdateServiceItemState(state string, siip string) error {
	switch state {
	case SHUTDOWNSENT, SHUTDOWNFAILED:
		a.UpdateServiceItem(
			ServiceItem{
				Service: Service{
					Ip:    siip,
					State: state,
				},
			},
		)
	default:
		return fmt.Errorf("invalid state %s", state)
	}
	return nil
}

func (a *AuthorizationClient) GetServiceItems(sip string) []ServiceItem {
	recvQueue := "/authorization/serviceitems/" + sip
	command := Command{
		Command:      GETSERVICEITEMS,
		ReceiveQueue: recvQueue,
		ServiceItem: ServiceItem{
			ServiceIP: sip,
		},
	}
	a.SendCommand(command)

	serviceItems := []ServiceItem{}
	err := a.ReadOneMessage(recvQueue, &serviceItems)
	if err != nil {
		log.Print("Error reading service items: ", err)
		return nil
	}
	return serviceItems
}

func (a *AuthorizationClient) ReadOneMessage(queue string, v any) error {
	messages := make(chan string)
	sub, err := a.Bus.ReceiveMessage(messages, queue)
	if err != nil {
		log.Println("Error receiving message: ", err)
		return err
	}
	message := <-messages

	sub.Close()
	err = json.Unmarshal([]byte(message), v)
	if err != nil {
		log.Print("Error unmarshalling message: ", err)
		return err
	}
	return nil
}
