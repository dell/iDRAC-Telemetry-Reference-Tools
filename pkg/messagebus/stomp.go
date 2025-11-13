package messagebus

import (
	"log"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
)

const (
	mbPort = 61613
	mbHost = "activemq"
)

func InitializeStompMB() messagebus.Messagebus {
	stompPort := mbPort
	for {
		mb, err := stomp.NewStompMessageBus(mbHost, stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			return mb
		}
	}
}
