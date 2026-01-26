// Licensed to You under the Apache License, Version 2.0.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/disc"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/service"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/wire"
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
	UPDATELOGIN   = "updatelogin"
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
	GETLOGIN          = "getlogin"

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
	Login        Login        `json:"login,omitempty"`
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
type Login struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	JwtVersion int    `json:"jwtVersion"`
}

const (
	CommandQueue = "/queue/authorization/command"
	EventQueue   = "/queue/authorization"
	ReplyPrefix  = "/queue/authorization/reply."
)

type AuthClientInterface interface {
	AddService(service Service) error
	AddServiceItem(si ServiceItem) error
	DeleteService(service Service) error
	DeleteServiceItem(si ServiceItem) error
	GetHECConfig()
	GetAllServices() []Service
	GetService(ctx context.Context, services chan<- *Service)
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
	UpdateLogin(l Login) error
	AddSystemType(sysType string) error
	GetAllSystemTypes() []SystemType
	GetLogin() Login
}

type AuthorizationService struct {
	*service.BaseService
}

// NewAuthorizationService creates a new AuthorizationService with the given message bus
func NewAuthorizationService(bus messagebus.Messagebus) *AuthorizationService {
	return &AuthorizationService{
		BaseService: service.NewBaseService(bus, CommandQueue),
	}
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

func (as *AuthorizationService) SendLogin(login Login, queue string) error {
	jsonStr, _ := json.Marshal(login)
	err := as.Bus.SendMessage(jsonStr, queue)
	if err != nil {
		log.Printf("Failed to send login %v", err)
	}
	return err
}

// BroadcastService broadcasts a service update to EventQueue using envelope format
func (as *AuthorizationService) BroadcastService(svc Service) error {
	env, err := wire.NewEnvelope("service", svc)
	if err != nil {
		return err
	}
	return as.SendEnvelope(EventQueue, env)
}

// ReceiveEnvelope receives commands as envelopes from the command queue
func (as *AuthorizationService) ReceiveEnvelope(envelopes chan<- wire.Envelope) error {
	return as.BaseService.ReceiveCommand(envelopes)
}

// ReceiveCommand receives commands from the command queue (legacy format)
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

type AuthorizationClient struct {
	*service.BaseClient
}

// NewAuthorizationClient creates a new AuthorizationClient with the given message bus
func NewAuthorizationClient(bus messagebus.Messagebus, clientName string) *AuthorizationClient {
	return &AuthorizationClient{
		BaseClient: service.NewBaseClient(bus, CommandQueue, ReplyPrefix, clientName, ReadTimeout*time.Second),
	}
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

// GetService listens for Service broadcasts on EventQueue.
// The ctx parameter allows cancellation of the listener.
func (ac *AuthorizationClient) GetService(ctx context.Context, services chan<- *Service) {
	envelopes := make(chan wire.Envelope, 10)
	ac.ListenToQueueFiltered(ctx, EventQueue, "service", envelopes)

	go func() {
		defer close(services)
		for env := range envelopes {
			var svc Service
			if err := wire.DecodePayload(env.Payload, &svc); err != nil {
				log.Printf("Error decoding service: %v", err)
				continue
			}
			select {
			case services <- &svc:
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (ac *AuthorizationClient) UpdateService(s Service) error {
	c := new(Command)
	c.Command = UPDATESERVICE
	c.Service = s
	return ac.SendCommand(*c)
}

func (ac *AuthorizationClient) UpdateLogin(l Login) error {
	c := new(Command)
	c.Command = UPDATELOGIN
	c.Login = l
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
	services := []Service{}
	err := ac.Call(GETALLSERVICES, nil, &services)
	if err != nil {
		log.Print("Error getting all services: ", err)
		return []Service{}
	}
	return services
}

// GetAllSystemTypes retrieves all configured system types using
// request/reply.
func (ac *AuthorizationClient) GetAllSystemTypes() []SystemType {
	systemtypes := []SystemType{}
	err := ac.Call(GETSYSTEMTYPES, nil, &systemtypes)
	if err != nil {
		log.Print("Error getting all system types: ", err)
		return []SystemType{}
	}
	return systemtypes
}

// GetValveStatus retrieves the current valve status using request/reply.
func (ac *AuthorizationClient) GetValveStatus() []ValveState {
	valvestatus := []ValveState{}
	err := ac.Call(GETVALVESTATE, nil, &valvestatus)
	if err != nil {
		log.Print("Error getting valve status: ", err)
		return []ValveState{}
	}
	return valvestatus
}

// GetServiceWithIP retrieves one service by IP using request/reply.
func (ac *AuthorizationClient) GetServiceWithIP(ip string) Service {
	svc := Service{}
	err := ac.Call(GETSERVICE, Service{Ip: ip}, &svc)
	if err != nil {
		log.Print("Error getting service with ip: ", ip, " err: ", err)
		return Service{}
	}
	return svc
}

func (ac *AuthorizationClient) GetLogin() Login {
	login := Login{}
	err := ac.Call(GETLOGIN, nil, &login)
	if err != nil {
		log.Print("Error getting login: ", err)
		return Login{}
	}
	return login
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
	serviceItems := []ServiceItem{}
	err := ac.Call(GETSERVICEITEMS, ServiceItem{ServiceIP: sip}, &serviceItems)
	if err != nil {
		log.Print("Error reading service items: ", err)
		return nil
	}
	return serviceItems
}

// GetServiceItemWithIP retrieves exactly one service item by IP using
// request/reply.
func (ac *AuthorizationClient) GetServiceItemWithIP(siip string) ServiceItem {
	serviceItems := []ServiceItem{}
	err := ac.Call(GETSERVICEITEM, ServiceItem{Service: Service{Ip: siip}}, &serviceItems)
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
