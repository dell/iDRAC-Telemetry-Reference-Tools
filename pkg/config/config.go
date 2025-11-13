package config

import (
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/config"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

func NewConfigClientWithBus(mb messagebus.Messagebus) *config.ConfigClient {
	return &config.ConfigClient{
		Bus: mb,
	}
}
