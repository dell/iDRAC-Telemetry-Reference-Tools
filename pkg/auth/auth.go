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

const SYSTEM_TYPE_IDRAC = auth.IDRAC

const AuthTypeUsernamePassword = auth.AuthTypeUsernamePassword
