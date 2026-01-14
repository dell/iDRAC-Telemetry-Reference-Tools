// Licensed to You under the Apache License, Version 2.0.

package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/disc"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

const (
	AuthTypeUsernamePassword = 1
	AuthTypeXAuthToken       = 2
	AuthTypeBearerToken      = 3
	ReadTimeout              = 5
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
	GETSERVICEITEM    = "getserviceitem"
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
	GetServiceItemWithIP(siip string) ServiceItem
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

// uniqueReplyQueue returns a unique, per-request reply destination suitable
// for request/reply.
//
// The reply destination must be a normal queue/topic name that both the
// requester and responder can use. Avoid broker-managed temporary destination
// semantics here.
func uniqueReplyQueue(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("/authorization/reply/%s-%d", prefix, time.Now().UnixNano())
	}
	return "/authorization/reply/" + prefix + "-" + hex.EncodeToString(b)
}

type AuthorizationClient struct {
	Bus messagebus.Messagebus
}

// ReadOneMessage subscribes to the given queue and waits for a single
// message which is unmarshalled into v.
func (ac *AuthorizationClient) ReadOneMessage(queue string, v any) error {
	messages := make(chan string, 1)
	sub, err := ac.Bus.ReceiveMessage(messages, queue)
	if err != nil {
		log.Println("Error receiving message: ", err)
		return err
	}
	defer sub.Close()

	select {
	case message := <-messages:
		err = json.Unmarshal([]byte(message), v)
		if err != nil {
			log.Print("Error unmarshalling message: ", err)
			return err
		}
		return nil
	case <-time.After(ReadTimeout * time.Second):
		return fmt.Errorf("timeout waiting for message from queue %s", queue)
	}
}

// requestReply performs a request/reply interaction.
//
// It subscribes to cmd.ReceiveQueue, then publishes cmd, then waits for a
// single reply message that is unmarshalled into v.
func (ac *AuthorizationClient) requestReply(cmd Command, v any) error {
	queue := cmd.ReceiveQueue
	if queue == "" {
		return fmt.Errorf("ReceiveQueue must be set")
	}

	messages := make(chan string, 1)
	sub, err := ac.Bus.ReceiveMessage(messages, queue)
	if err != nil {
		log.Println("Error receiving message: ", err)
		return err
	}
	defer sub.Close()

	if err := ac.SendCommand(cmd); err != nil {
		return err
	}

	select {
	case message := <-messages:
		err = json.Unmarshal([]byte(message), v)
		if err != nil {
			log.Print("Error unmarshalling message: ", err)
			return err
		}
		return nil
	case <-time.After(ReadTimeout * time.Second):
		return fmt.Errorf("timeout waiting for message from queue %s", queue)
	}
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

// GetAllServices retrieves all configured services using request/reply.
func (ac *AuthorizationClient) GetAllServices() []Service {
	recvQueue := uniqueReplyQueue("authorization-getallservices")
	fmt.Println("In GetAllServices")
	c := Command{
		Command:      GETALLSERVICES,
		ReceiveQueue: recvQueue,
	}
	services := []Service{}
	err := ac.requestReply(c, &services)
	if err != nil {
		log.Print("Error getting all services: ", err)
		return []Service{}
	}
	return services
}

// GetAllSystemTypes retrieves all configured system types using
// request/reply.
func (ac *AuthorizationClient) GetAllSystemTypes() []SystemType {
	recvQueue := uniqueReplyQueue("authorization-getsystemtypes")
	c := Command{
		Command:      GETSYSTEMTYPES,
		ReceiveQueue: recvQueue,
	}
	systemtype := []SystemType{}
	err := ac.requestReply(c, &systemtype)
	if err != nil {
		log.Print("Error getting all services: ", err)
		return []SystemType{}
	}
	return systemtype
}

// GetValveStatus retrieves the current valve status using request/reply.
func (ac *AuthorizationClient) GetValveStatus() []ValveState {
	recvQueue := uniqueReplyQueue("authorization-getvalvestatus")
	c := Command{
		Command:      GETVALVESTATE,
		ReceiveQueue: recvQueue,
	}
	valvestatus := []ValveState{}
	err := ac.requestReply(c, &valvestatus)
	if err != nil {
		log.Print("Error getting valve status: ", err)
		return []ValveState{}
	}
	log.Print("valvestatus: ", valvestatus)
	return valvestatus
}

// GetServiceWithIP retrieves one service by IP using request/reply.
func (ac *AuthorizationClient) GetServiceWithIP(ip string) Service {
	recvQueue := uniqueReplyQueue("authorization-getservice")
	c := Command{
		Command:      GETSERVICE,
		ReceiveQueue: recvQueue,
		Service: Service{
			Ip: ip,
		},
	}
	service := Service{}
	err := ac.requestReply(c, &service)
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

// GetServiceItems retrieves the associated systems for a service IP using
// request/reply.
func (ac *AuthorizationClient) GetServiceItems(sip string) []ServiceItem {
	recvQueue := uniqueReplyQueue("authorization-getserviceitems")
	command := Command{
		Command:      GETSERVICEITEMS,
		ReceiveQueue: recvQueue,
		ServiceItem: ServiceItem{
			ServiceIP: sip,
		},
	}
	serviceItems := []ServiceItem{}
	err := ac.requestReply(command, &serviceItems)
	if err != nil {
		log.Print("Error reading service items: ", err)
		return nil
	}
	fmt.Println("Get associated systems", serviceItems)
	return serviceItems
}

// GetServiceItemWithIP retrieves exactly one service item by IP using
// request/reply.
func (ac *AuthorizationClient) GetServiceItemWithIP(siip string) ServiceItem {
	recvQueue := uniqueReplyQueue("authorization-getserviceitem")
	command := Command{
		Command:      GETSERVICEITEM,
		ReceiveQueue: recvQueue,
		ServiceItem: ServiceItem{
			Service: Service{
				Ip: siip,
			},
		},
	}
	serviceItems := []ServiceItem{}
	err := ac.requestReply(command, &serviceItems)
	if err != nil {
		log.Print("Error reading service items: ", err)
		return ServiceItem{}
	}
	if len(serviceItems) != 1 {
		log.Print("Error reading service items: Found multiple items for ip ", siip)
		return ServiceItem{}
	}
	return serviceItems[0]
}

func (ac *AuthorizationClient) AddSystemType(sysType string) error {
	c := new(Command)
	c.Command = ADDSYSTEMTYPE
	c.SystemType = SystemType(sysType)
	return ac.SendCommand(*c)
}
