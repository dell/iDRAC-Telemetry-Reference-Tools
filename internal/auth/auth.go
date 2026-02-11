// Licensed to You under the Apache License, Version 2.0.

package auth

import (
	"context"
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
	SHUTDOWNRECEIVED = "Shutdown Received"
	SHUTDOWNFAILED   = "Shutdown Failed"
	POWERSTATEON     = "Power Status On"
	POWERSTATEOFF    = "Power Status Off"
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

func (s Service) String() string {
	return fmt.Sprintf("ServiceType: %d, Ip: %s, AuthType: %d, Auth: *****, State: %s", s.ServiceType, s.Ip, s.AuthType, s.State)
}

type ServiceItem struct {
	Service
	ServiceIP          string `json:"serviceIp"`
	Systemtypedesc     string `json:"systemtypedesc"`
	IsForcefulshutdown bool   `json:"isforcefulshutdown"`
	Timeout            int    `json:"timeout"`
}

func (si ServiceItem) String() string {
	return fmt.Sprintf("ServiceType: %d, Ip: %s, AuthType: %d, Auth: *****, State: %s, ServiceIP: %s, Systemtypedesc: %s, IsForcefulshutdown: %t, Timeout: %d",
		si.ServiceType, si.Ip, si.AuthType, si.State, si.ServiceIP, si.Systemtypedesc, si.IsForcefulshutdown, si.Timeout)
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

	// Message type for services
	SERVICE = "service"
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
	ResendAll()
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

// BroadcastService broadcasts a service update to EventQueue using envelope format
func (as *AuthorizationService) BroadcastService(svc Service) error {
	// TODO see if the type can be reused
	env, err := wire.NewEnvelope(SERVICE, svc)
	if err != nil {
		return err
	}
	return as.SendEnvelope(EventQueue, env)
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

// GetHECConfig sends a fire-and-forget request to get HEC config
func (ac *AuthorizationClient) GetHECConfig() {
	ac.Send(GETHECCONFIG, nil)
}

// ResendAll sends a fire-and-forget request to resend all services
func (ac *AuthorizationClient) ResendAll() {
	ac.Send(RESEND, nil)
}

// SplunkAddHEC sends a fire-and-forget request to add Splunk HEC config
func (ac *AuthorizationClient) SplunkAddHEC(splunkHttp SplunkConfig) error {
	return ac.Send(SPLUNKADDHEC, splunkHttp)
}

// AddService sends a fire-and-forget request to add a service
func (ac *AuthorizationClient) AddService(service Service) error {
	return ac.Send(ADDSERVICE, service)
}

// DeleteService sends a fire-and-forget request to delete a service
func (ac *AuthorizationClient) DeleteService(service Service) error {
	return ac.Send(DELETESERVICE, service)
}

// GetService listens for Service broadcasts on EventQueue.
// The ctx parameter allows cancellation of the listener.
func (ac *AuthorizationClient) GetService(ctx context.Context, services chan<- *Service) {
	envelopes := make(chan wire.Envelope, 10)
	ac.ListenToQueueFiltered(ctx, EventQueue, SERVICE, envelopes)

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

// UpdateService sends a fire-and-forget request to update a service
func (ac *AuthorizationClient) UpdateService(s Service) error {
	return ac.Send(UPDATESERVICE, s)
}

// UpdateLogin sends a fire-and-forget request to update login credentials
func (ac *AuthorizationClient) UpdateLogin(l Login) error {
	return ac.Send(UPDATELOGIN, l)
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

// UpdateValveState sends a fire-and-forget request to update valve state
func (ac *AuthorizationClient) UpdateValveState(ip string, state1 string, state2 string) error {
	return ac.Send(UPDATEVALVESTATE, ValveState{
		Ip:      ip,
		VState1: state1,
		VState2: state2,
	})
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

// AddServiceItem sends a fire-and-forget request to add a service item
func (ac *AuthorizationClient) AddServiceItem(si ServiceItem) error {
	return ac.Send(ADDSERVICEITEM, si)
}

// DeleteServiceItem sends a fire-and-forget request to delete a service item
func (ac *AuthorizationClient) DeleteServiceItem(si ServiceItem) error {
	return ac.Send(DELETESERVICEITEM, si)
}

// UpdateServiceX sends a fire-and-forget request to update a service (extended)
func (ac *AuthorizationClient) UpdateServiceX(s Service) error {
	return ac.Send(UPDATESERVICEX, s)
}

func (ac *AuthorizationClient) UpdateServiceXState(state string, ip string) error {
	return ac.UpdateServiceX(
		Service{
			Ip:    ip,
			State: state,
		},
	)
}

// UpdateServiceItem sends a fire-and-forget request to update a service item
func (ac *AuthorizationClient) UpdateServiceItem(si ServiceItem) error {
	return ac.Send(UPDATESERVICEITEM, si)
}

func (ac *AuthorizationClient) UpdateServiceItemState(state string, siip string) error {
	switch state {
	case SHUTDOWNRECEIVED, SHUTDOWNFAILED, CONNFAILED, RUNNING, POWERSTATEON, POWERSTATEOFF:
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

// AddSystemType sends a fire-and-forget request to add a system type
func (ac *AuthorizationClient) AddSystemType(sysType string) error {
	return ac.Send(ADDSYSTEMTYPE, SystemType(sysType))
}
