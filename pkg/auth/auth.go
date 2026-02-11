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
	UPDATELOGIN   = auth.UPDATELOGIN

	UPDATESERVICE     = auth.UPDATESERVICE
	GETALLSERVICES    = auth.GETALLSERVICES
	ADDSERVICEITEM    = auth.ADDSERVICEITEM
	DELETESERVICEITEM = auth.DELETESERVICEITEM
	GETSERVICEITEMS   = auth.GETSERVICEITEMS
	GETSERVICEITEM    = auth.GETSERVICEITEM
	UPDATESERVICEITEM = auth.UPDATESERVICEITEM
	GETVALVESTATE     = auth.GETVALVESTATE
	UPDATEVALVESTATE  = auth.UPDATEVALVESTATE
	ADDSYSTEMTYPE     = auth.ADDSYSTEMTYPE
	GETSYSTEMTYPES    = auth.GETSYSTEMTYPES
	GETLOGIN          = auth.GETLOGIN

	UPDATESERVICEX = auth.UPDATESERVICEX
)

func NewAuthClientWithBus(mb messagebus.Messagebus, clientName string) *auth.AuthorizationClient {
	return auth.NewAuthorizationClient(mb, clientName)
}

func NewAuthServiceWithBus(mb messagebus.Messagebus) *auth.AuthorizationService {
	return auth.NewAuthorizationService(mb)
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
type ValveState = auth.ValveState
type SystemType = auth.SystemType
type Login = auth.Login

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
	SHUTDOWNRECEIVED = auth.SHUTDOWNRECEIVED
	SHUTDOWNFAILED   = auth.SHUTDOWNFAILED
	POWERSTATEON     = auth.POWERSTATEON
	POWERSTATEOFF    = auth.POWERSTATEOFF
)
