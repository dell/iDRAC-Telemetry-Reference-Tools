package auth

import (
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

const (
	TERMINATE     = auth.TERMINATE
	RESEND        = auth.RESEND
	ADDSERVICE    = auth.ADDSERVICE
	DELETESERVICE = auth.DELETESERVICE
	GETSERVICE    = auth.GETSERVICE

	UPDATESERVICE     = auth.UPDATESERVICE
	GETALLSERVICES    = auth.GETALLSERVICES
	ADDSERVICEITEM    = auth.ADDSERVICEITEM
	DELETESERVICEITEM = auth.DELETESERVICEITEM
	GETSERVICEITEMS   = auth.GETSERVICEITEMS
	UPDATESERVICEITEM = auth.UPDATESERVICEITEM
	GETVALVESTATE     = auth.GETVALVESTATE
	UPDATEVALVESTATE  = auth.UPDATEVALVESTATE
	ADDSYSTEMTYPE     = auth.ADDSYSTEMTYPE
)

func NewAuthClientWithBus(mb messagebus.Messagebus) *auth.AuthorizationClient {
	return &auth.AuthorizationClient{
		Bus: mb,
	}
}

func NewAuthServiceWithBus(mb messagebus.Messagebus) *auth.AuthorizationService {
	return &auth.AuthorizationService{
		Bus: mb,
	}
}

func NewServiceChan(n int) chan *auth.Service {
	return make(chan *auth.Service, n)
}

func NewCommandChan() chan *auth.Command {
	return make(chan *auth.Command)
}

type Service = auth.Service
type ServiceItem = auth.ServiceItem
type Command = auth.Command
type SplunkConfig = auth.SplunkConfig
type Valvestate = auth.ValveState

type AuthorizationClient = auth.AuthorizationClient
type AuthorizationService = auth.AuthorizationService
type AuthClientInterface = auth.AuthClientInterface

const (
	SYSTEM_TYPE_IDRAC        = auth.IDRAC
	SYSTEM_TYPE_IRC          = auth.IRC
	SYSTEM_TYPE_NVLINK       = auth.NVLINK
	AuthTypeUsernamePassword = auth.AuthTypeUsernamePassword

	// Service states
	STARTING     = auth.STARTING
	RUNNING      = auth.RUNNING
	RUNNINGWOTEL = auth.RUNNINGWOTEL
	TELNOTFOUND  = auth.TELNOTFOUND
	CONNFAILED   = auth.CONNFAILED
	LEAKED       = auth.LEAKED
	MONITORING   = auth.MONITORING

	// ServiceItem states
	SHUTDOWNSENT   = auth.SHUTDOWNSENT
	SHUTDOWNFAILED = auth.SHUTDOWNFAILED
)
