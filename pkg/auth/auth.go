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

	ADDSERVICEITEM    = auth.ADDSERVICEITEM
	DELETESERVICEITEM = auth.DELETESERVICEITEM
	GETSERVICEITEMS   = auth.GETSERVICEITEMS
	UPDATESERVICEITEM = auth.UPDATESERVICEITEM
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

type AuthorizationClient = auth.AuthorizationClient

const SYSTEM_TYPE_IDRAC = auth.IDRAC

const SYSTEM_TYPE_IRC = auth.IRC

const AuthTypeUsernamePassword = auth.AuthTypeUsernamePassword
