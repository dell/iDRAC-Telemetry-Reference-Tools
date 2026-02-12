// Licensed to You under the Apache License, Version 2.0.
// This script is responsible for reading data from Redfish into the ingest pipeline.

package main

import (
	"context"
	//"encoding/json"

	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/service"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/wire"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/redfish"
	prr "github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/redfishread"
)

var configStrings = map[string]string{
	"mbhost":       "activemq",
	"mbport":       "61613",
	"inventoryurl": "/redfish/v1/Chassis/System.Embedded.1",
}

type SystemDetail struct {
	SystemID string
	HostName string
	Model    string
	SKU      string
	FwVer    string
	FQDN     string
	ImgID    string
}

type RedfishDevice struct {
	HasChildren bool
	Redfish     *redfish.RedfishClient
	SystemDetail
	ChildDevices map[int]string
	Events       chan *redfish.RedfishEvent
	Metrics      chan *redfish.RedfishEvent
	State        string
	LastEvent    time.Time
	CtxCancel    context.CancelFunc
	Ctx          context.Context
}

var devices prr.RedfishDevices
var dataGroups map[string]map[string]*databus.DataGroup
var dataGroupsMu sync.RWMutex

// handleAuthServiceChannel Authenticates to the iDRAC and then launches the telemetry monitoring process via
// redfishMonitorStart
func handleAuthServiceChannel(serviceIn chan *auth.Service, dataBusService *databus.DataBusService, devices prr.RedfishDevices, authClient auth.AuthClientInterface) {
	prr.InitNewDataGroupsMap()
	for {
		service := <-serviceIn
		device, err := prr.ValidateAndAddDevice(service, devices)
		if err == nil {
			go prr.RedfishMonitorStart(device, dataBusService, authClient)
		}
	}
}

// getEnvSettings Retrieve settings from the environment. Notice that configStrings has a set of defaults but those
// can be overridden by environment variables via this function.
func getEnvSettings() {
	mbHost := os.Getenv("MESSAGEBUS_HOST")
	if len(mbHost) > 0 {
		configStrings["mbhost"] = mbHost
	}
	mbPort := os.Getenv("MESSAGEBUS_PORT")
	if len(mbPort) > 0 {
		configStrings["mbport"] = mbPort
	}
}

func main() {
	//Gather configuration from environment variables
	getEnvSettings()

	// dataGroups = make(map[string]map[string]*databus.DataGroup)
	var dataBusService *databus.DataBusService
	var authClient *auth.AuthorizationClient

	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			authClient = auth.NewAuthorizationClient(mb, "redfishread")
			dataBusService = databus.NewDataBusService(mb)
			defer mb.Close()
			break
		}
	}

	serviceIn := make(chan *auth.Service, 10)
	envelopes := make(chan service.Envelope)

	// devices is used to answer GETPRODUCERS requests (UI Systems list).
	// It must be initialized once so additions in handleAuthServiceChannel are visible here.
	devices = prr.NewRedfishDevices()

	log.Print("Redfish Telemetry Read Service is initialized")

	authClient.ResendAll()
	go authClient.GetService(context.Background(), serviceIn)
	go handleAuthServiceChannel(serviceIn, dataBusService, devices, authClient) // THIS FUNCTION ADDS SERVICE
	go dataBusService.ReceiveEnvelopes(envelopes)                               //nolint: errcheck
	for {
		env := <-envelopes
		log.Println("Received command in redfishread", "command", env.Type)
		switch env.Type {
		case databus.GET:

			dataGroups := prr.DataGroupsMap.GetDataGroups()
			for _, system := range dataGroups {
				for _, group := range system {
					dataBusService.Reply(env, *group)
				}
			}

		case databus.GETPRODUCERS:
			producers := make([]*databus.DataProducer, len(devices))
			i := 0
			for _, dev := range devices {
				producer := new(databus.DataProducer)
				producer.Hostname = dev.Redfish.GetHostname()
				producer.Username = dev.Redfish.GetUsername()
				producer.State = dev.State
				producer.LastEvent = dev.LastEvent
				producers[i] = producer
				i = i + 1
			}
			dataBusService.Reply(env, producers)
		case databus.DELETEPRODUCER:
			var service auth.Service
			if err := wire.DecodePayload(env.Payload, &service); err != nil {
				log.Printf("Failed decoding DELETEPRODUCER payload: %v", err)
				continue
			}
			devices[service.Ip].CtxCancel()
			log.Printf("service has been cancelled, Ctx = %v", devices[service.Ip].Ctx)
			delete(devices, service.Ip)
		case auth.TERMINATE:
			os.Exit(0)
		}
	}
}
