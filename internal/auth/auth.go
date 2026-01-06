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
	LEAKED       = "Leak Detected"
	MONITORING   = "Monitoring"
)

// ServiceItem states
const (
	SHUTDOWNSENT   = "Shutdown Sent"
	SHUTDOWNFAILED = "Shutdown Failed"
	POWERSTATEON   = "Power Status On"
	POWERSTATEOFF  = "Power Status Off"
)

const (
	UNKNOWN = disc.UNKNOWN
	MSM     = disc.MSM
	EC      = disc.EC
	IDRAC   = disc.IDRAC
	IRC     = disc.IRC
	NVLINK  = disc.NVLINK
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
	ServiceIP          string `json:"serviceIp"`
	Systemtypedesc     string `json:"systemtypedesc"`
	IsForcefulshutdown bool   `json:"isforcefulshutdown"`
	Timeout            int    `json:"timeout"`
}

const (
	RESEND        = "resend"
	ADDSERVICE    = "addservice"
	DELETESERVICE = "deleteservice"
	UPDATESERVICE = "updateservice"
	GETSERVICE    = "getservice"
	TERMINATE     = "terminate"
	SPLUNKADDHEC  = "splunkaddhec"
	GETHECCONFIG  = "gethecconfig"

	GETALLSERVICES    = "getallservices"
	ADDSERVICEITEM    = "addserviceitem"
	DELETESERVICEITEM = "deleteserviceitem"
	GETSERVICEITEMS   = "getserviceitems"
	UPDATESERVICEITEM = "updateserviceitem"
	UPDATEVALVESTATE  = "updatevalvestate"
	GETVALVESTATE     = "getvalvestatus"
	ADDSYSTEMTYPE     = "addsystemtype"
	GETSYSTEMTYPES    = "getsystemtypes"

	UPDATESERVICEX = "updateservicex"
)

type SplunkConfig struct {
	Url   string `json:"url,omitempty"`
	Key   string `json:"key,omitempty"`
	Index string `json:"index,omitempty"`
}

type SystemType string

type Command struct {
	Command      string       `json:"command"`
	SplunkConfig SplunkConfig `json:"Splunkconfig,omitempty"`
	Service      Service      `json:"service,omitempty"`
	ServiceItem  ServiceItem  `json:"serviceitem,omitempty"`
	ReceiveQueue string       `json:"receivequeue,omitempty"`
	SystemType   SystemType   `json:"systemtype,omitempty"`
	ValveState   ValveState   `json:"valvestate,omitempty"`
}

type ValveState struct {
	Ip      string `json:"ip"`
	VState1 string `json:"state1"`
	VState2 string `json:"state2"`
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
	GetAllServices() []Service
	GetService(services chan<- *Service)
	GetServiceItems(sip string) []ServiceItem
	GetServiceWithIP(ip string) Service
	ReadOneMessage(queue string, v any) error
	ResendAll()
	SendCommand(command Command) error
	SendCommandString(command string)
	SplunkAddHEC(SplunkHttp SplunkConfig) error
	UpdateServiceX(s Service) error
	UpdateService(s Service) error
	UpdateServiceItem(si ServiceItem) error
	UpdateServiceItemState(state string, siip string) error
	UpdateServiceState(state string, sip string) error
	UpdateServiceXState(state string, ip string) error
	UpdateValveState(ip string, state1 string, state2 string) error
	AddSystemType(sysType string) error
	GetAllSystemTypes() []SystemType
}

type AuthorizationService struct {
	Bus messagebus.Messagebus
}
type AuthorizationClient struct {
	Bus messagebus.Messagebus
}

func (as *AuthorizationService) SendValveState(valvestatus []ValveState, rcvQueue string) error {

	jsonStr, _ := json.Marshal(valvestatus)
	err := as.Bus.SendMessage(jsonStr, rcvQueue)
	if err != nil {
		log.Printf("Failed to send valve status to queue %s: %v", rcvQueue, err)
	}
	return err

}

func (as *AuthorizationService) SendSystemTypes(systemtypes []SystemType, rcvQueue string) error {

	jsonStr, _ := json.Marshal(systemtypes)
	err := as.Bus.SendMessage(jsonStr, rcvQueue)
	if err != nil {
		log.Printf("Failed to send system types to queue %s: %v", rcvQueue, err)
	}
	return err
}

func (as *AuthorizationService) SendAllServices(services []Service, rcvQueue string) error {
	// Convert the slice of services to JSON
	jsonStr, err := json.Marshal(services)
	if err != nil {
		log.Printf("Failed to marshal services: %v", err)
		return err
	}

	// Send the JSON message to the queue
	err = as.Bus.SendMessage(jsonStr, rcvQueue)
	if err != nil {
		log.Printf("Failed to send services to queue %s: %v", rcvQueue, err)
		return err
	}
	return nil
}

func (as *AuthorizationService) SendService(service Service) error {
	return as.SendServiceWithQ(service, EventQueue)
}

func (as *AuthorizationService) SendServiceWithQ(service Service, queue string) error {
	jsonStr, _ := json.Marshal(service)
	err := as.Bus.SendMessage(jsonStr, queue)
	if err != nil {
		log.Printf("Failed to send service %v", err)
	}
	return err
}

func (as *AuthorizationService) SendServiceItems(sis []ServiceItem, rcvQueue string) error {
	return as.SendServiceItemsWithQ(sis, rcvQueue)
}

func (as *AuthorizationService) SendServiceItemsWithQ(sis []ServiceItem, queue string) error {
	jsonStr, _ := json.Marshal(sis)
	err := as.Bus.SendMessage(jsonStr, queue)
	if err != nil {
		log.Printf("Failed to send service %v", err)
	}
	return err
}

func (as *AuthorizationService) ReceiveCommand(commands chan<- *Command) error {
	messages := make(chan string, 10)

	go func() {
		_, err := as.Bus.ReceiveMessage(messages, CommandQueue)
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

func (ac *AuthorizationClient) GetHECConfig() {
	ac.SendCommandString(GETHECCONFIG)
}

func (as *AuthorizationService) Sendconfig(config SplunkConfig) error {
	jsonStr, _ := json.Marshal(config)
	err := as.Bus.SendMessage(jsonStr, EventQueue)
	if err != nil {
		log.Printf("Failed to send service %v", err)
	}
	return err

}

func (ac *AuthorizationClient) SendCommand(command Command) error {
	jsonStr, _ := json.Marshal(command)
	err := ac.Bus.SendMessage(jsonStr, CommandQueue)
	if err != nil {
		log.Printf("Failed to send command %v", err)
	}
	return err
}

func (ac *AuthorizationClient) SendCommandString(command string) {
	c := new(Command)
	c.Command = command
	ac.SendCommand(*c)
}

func (ac *AuthorizationClient) ResendAll() {
	ac.SendCommandString(RESEND)
}

func (ac *AuthorizationClient) SplunkAddHEC(SplunkHttp SplunkConfig) error {
	c := new(Command)
	c.Command = SPLUNKADDHEC
	c.SplunkConfig = SplunkHttp
	return ac.SendCommand(*c)
}

func (ac *AuthorizationClient) AddService(service Service) error {
	c := new(Command)
	c.Command = ADDSERVICE
	c.Service = service
	return ac.SendCommand(*c)
}

func (ac *AuthorizationClient) DeleteService(service Service) error {
	c := new(Command)
	c.Command = DELETESERVICE
	c.Service = service
	return ac.SendCommand(*c)
}

func (ac *AuthorizationClient) GetService(services chan<- *Service) {
	messages := make(chan string, 10)

	go func() {
		_, err := ac.Bus.ReceiveMessage(messages, EventQueue)
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

func (ac *AuthorizationClient) UpdateService(s Service) error {
	c := new(Command)
	c.Command = UPDATESERVICE
	c.Service = s
	return ac.SendCommand(*c)
}

func (ac *AuthorizationClient) UpdateServiceState(state string, sip string) error {
	switch state {
	case CONNFAILED, STARTING, RUNNING, TELNOTFOUND, RUNNINGWOTEL, LEAKED, MONITORING:
		ac.UpdateService(
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

func (ac *AuthorizationClient) UpdateValveState(ip string, state1 string, state2 string) error {
	c := new(Command)
	c.Command = UPDATEVALVESTATE
	c.ValveState = ValveState{
		Ip:      ip,
		VState1: state1,
		VState2: state2,
	}
	return ac.SendCommand(*c)
}
func (ac *AuthorizationClient) GetAllServices() []Service {
	recvQueue := "/authorization/services/all"
	fmt.Println("In GetAllServices")
	c := Command{
		Command:      GETALLSERVICES,
		ReceiveQueue: recvQueue,
	}
	ac.SendCommand(c)

	services := []Service{}
	err := ac.ReadOneMessage(recvQueue, &services)
	if err != nil {
		log.Print("Error getting all services: ", err)
		return []Service{}
	}
	return services
}

func (ac *AuthorizationClient) GetAllSystemTypes() []SystemType {
	recvQueue := "/authorization/SystemTypes/all"
	c := Command{
		Command:      GETSYSTEMTYPES,
		ReceiveQueue: recvQueue,
	}
	ac.SendCommand(c)

	systemtype := []SystemType{}
	err := ac.ReadOneMessage(recvQueue, &systemtype)
	if err != nil {
		log.Print("Error getting all services: ", err)
		return []SystemType{}
	}
	return systemtype

}
func (ac *AuthorizationClient) GetValveStatus() []ValveState {
	recvQueue := "/authorization/ValveStatus/all"
	c := Command{
		Command:      GETVALVESTATE,
		ReceiveQueue: recvQueue,
	}
	ac.SendCommand(c)

	valvestatus := []ValveState{}
	err := ac.ReadOneMessage(recvQueue, &valvestatus)
	if err != nil {
		log.Print("Error getting valve status: ", err)
		return []ValveState{}

	}
	log.Print("valvestatus: ", valvestatus)
	return valvestatus
}

func (ac *AuthorizationClient) GetServiceWithIP(ip string) Service {
	recvQueue := "/authorization/services/" + ip
	c := Command{
		Command:      GETSERVICE,
		ReceiveQueue: recvQueue,
		Service: Service{
			Ip: ip,
		},
	}
	ac.SendCommand(c)

	service := Service{}
	err := ac.ReadOneMessage(recvQueue, &service)
	if err != nil {
		log.Print("Error getting service with ip: ", ip, " err: ", err)
		return Service{}
	}
	return service
}

func (ac *AuthorizationClient) AddServiceItem(si ServiceItem) error {
	c := new(Command)
	c.Command = ADDSERVICEITEM
	c.ServiceItem = si
	return ac.SendCommand(*c)
}

func (ac *AuthorizationClient) DeleteServiceItem(si ServiceItem) error {
	c := new(Command)
	c.Command = DELETESERVICEITEM
	c.ServiceItem = si
	return ac.SendCommand(*c)
}

func (ac *AuthorizationClient) UpdateServiceX(s Service) error {
	c := new(Command)
	c.Command = UPDATESERVICEX
	c.Service = s
	return ac.SendCommand(*c)
}

func (ac *AuthorizationClient) UpdateServiceXState(state string, ip string) error {
	return ac.UpdateServiceX(
		Service{
			Ip:    ip,
			State: state,
		},
	)
}

func (ac *AuthorizationClient) UpdateServiceItem(si ServiceItem) error {
	c := new(Command)
	c.Command = UPDATESERVICEITEM
	c.ServiceItem = si
	return ac.SendCommand(*c)
}

func (ac *AuthorizationClient) UpdateServiceItemState(state string, siip string) error {
	switch state {
	case SHUTDOWNSENT, SHUTDOWNFAILED, CONNFAILED, RUNNING, POWERSTATEON, POWERSTATEOFF:
		ac.UpdateServiceItem(
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

func (ac *AuthorizationClient) GetServiceItems(sip string) []ServiceItem {
	recvQueue := "/authorization/serviceitems/" + sip
	command := Command{
		Command:      GETSERVICEITEMS,
		ReceiveQueue: recvQueue,
		ServiceItem: ServiceItem{
			ServiceIP: sip,
		},
	}
	ac.SendCommand(command)

	serviceItems := []ServiceItem{}

	err := ac.ReadOneMessage(recvQueue, &serviceItems)
	if err != nil {
		log.Print("Error reading service items: ", err)
		return nil
	}
	fmt.Println("Get associated systems", serviceItems)
	return serviceItems
}

func (ac *AuthorizationClient) ReadOneMessage(queue string, v any) error {
	messages := make(chan string)
	sub, err := ac.Bus.ReceiveMessage(messages, queue)
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

func (ac *AuthorizationClient) AddSystemType(sysType string) error {
	c := new(Command)
	c.Command = ADDSYSTEMTYPE
	c.SystemType = SystemType(sysType)
	return ac.SendCommand(*c)
}
