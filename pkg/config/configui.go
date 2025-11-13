package config

import (
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/config"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
)

type SystemHandler struct {
	AuthClient *auth.AuthorizationClient
	DataBus    *databus.DataBusClient
	ConfigBus  *config.ConfigClient
}

func NewSystemHandler(authClient *auth.AuthorizationClient, dataBus *databus.DataBusClient, configBus *config.ConfigClient) *SystemHandler {
	return &SystemHandler{
		AuthClient: authClient,
		DataBus:    dataBus,
		ConfigBus:  configBus,
	}
}
