// Licensed to You under the Apache License, Version 2.0.
// This script is responsible for reading data from Redfish into the ingest pipeline.

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/redfish"
)

var configStrings = map[string]string{
	"mbhost": "activemq",
	"mbport": "61613",
}

type RedfishDevice struct {
	HasChildren  bool
	Redfish      *redfish.RedfishClient
	SystemID     string
	ChildDevices map[int]string
	Events       chan *redfish.RedfishEvent
	State        string
	LastEvent    time.Time
	CtxCancel    context.CancelFunc
	Ctx          context.Context
}

var devices map[string]*RedfishDevice
var dataGroups map[string]map[string]*databus.DataGroup
var dataGroupsMu sync.RWMutex

// populateChildChassis If the device is a chassis, we also have to obtain IDs / info for all children in that chassis
// and pull telemetry on them. This function will expand the chassis information and obtain the necessary information.
func populateChildChassis(r *RedfishDevice, serviceRoot *redfish.RedfishPayload) {
	chassisCollection, err := serviceRoot.GetPropertyByName("Chassis")
	if err != nil {
		log.Println(err)
		return
	}
	size := chassisCollection.GetCollectionSize()
	for i := 0; i < size; i++ {
		chassis, err := chassisCollection.GetPropertyByIndex(i)
		if err != nil {
			continue
		}
		if chassis.Object["ChassisType"].(string) != "Enclosure" && chassis.Object["SKU"] != nil {
			name := chassis.Object["Name"].(string)
			if strings.HasPrefix(name, "Sled-") {
				split := strings.Split(name, "-")
				i, _ := strconv.Atoi(split[1])
				r.ChildDevices[i] = chassis.Object["SKU"].(string)
			}
		}
	}
}

func getValueIdContextAndLabel(value *redfish.RedfishPayload, i int) (string, string, string) {
	id := ""
	if value.Object["MetricId"] != nil {
		id = value.Object["MetricId"].(string)
	} else {
		id = fmt.Sprintf("Metric%d", i)
	}
	if value.Object["Oem"] != nil {
		oem := value.Object["Oem"].(map[string]interface{})
		if oem["Dell"] != nil {
			dell := oem["Dell"].(map[string]interface{})
			if dell["ContextID"] != nil && dell["Label"] != nil {
				return id, dell["ContextID"].(string), dell["Label"].(string)
			}
		}
	}
	return id, "", id
}

// Responsible for taking the report received from SSE, getting its component parts, and then sending it along the
// data bus
func parseReport(metricReport *redfish.RedfishPayload, systemid string, dataBusService *databus.DataBusService) {
	metricValues, err := metricReport.GetPropertyByName("MetricValues")
	if err != nil {
		log.Printf("%s: Unable to get metric report's MetricValues: %v %v", systemid, err, metricReport)
		return
	}
	group := new(databus.DataGroup)

	group.ID = metricReport.Object["Id"].(string)
	group.Label = metricReport.Object["Name"].(string)
	valuesSize := metricValues.GetArraySize()
	for j := 0; j < valuesSize; j++ {
		metricValue, err := metricValues.GetPropertyByIndex(j)
		if err != nil {
			log.Printf("Unable to get metric report MetricValue %d: %v", j, err)
			continue
		}
		if metricValue.Object["MetricValue"] != nil {
			data := new(databus.DataValue)
			data.ID, data.Context, data.Label = getValueIdContextAndLabel(metricValue, j)
			data.Value = metricValue.Object["MetricValue"].(string)
			if metricValue.Object["Timestamp"] == nil {
				t := time.Now()
				data.Timestamp = t.Format("2006-01-02T15:04:05-0700")
			} else {
				data.Timestamp = metricValue.Object["Timestamp"].(string)
			}
			data.System = systemid
			group.Values = append(group.Values, *data)
		}
	}
	dataBusService.SendGroup(*group)

	dataGroupsMu.Lock()
	if dataGroups[systemid] == nil {
		dataGroups[systemid] = make(map[string]*databus.DataGroup)
	}
	dataGroups[systemid][group.ID] = group
	dataGroupsMu.Unlock()
}

// Responsible for taking the lifecycle events received from SSE, getting its events, and then sending it along the
// data bus
func parseRedfishLce(lceevents *redfish.RedfishPayload, id string, dataBusService *databus.DataBusService) {
	eventData, err := lceevents.GetPropertyByName("Events")
	if err != nil {
		log.Printf("%s: Unable to get eventData: %v %v", id, err, lceevents)
		return
	}
	log.Printf("RedFish LifeCycle Events Found for parsing: %v\n", eventData)

	group := new(databus.DataGroup)

	group.ID = lceevents.Object["Id"].(string)
	group.Label = lceevents.Object["Name"].(string)
	size := lceevents.GetEventSize()
	for j := 0; j < size; j++ {
		eventData, err := lceevents.GetEventByIndex(j)
		if err != nil {
			log.Printf("Unable to retrieve the redfish lifecycle events\n")
		}
		if eventData.Object["EventId"] != nil {
			data := new(databus.DataValue)
			data.ID = eventData.Object["EventId"].(string)
			data.Context = ""
			data.Label = ""
			data.Value = eventData.Object["EventType"].(string)
			if eventData.Object["EventTimestamp"] == nil {
				t := time.Now()
				data.Timestamp = t.Format("2006-01-02T15:04:05-0700")
			} else {
				data.Timestamp = eventData.Object["EventTimestamp"].(string)
			}
			data.System = id
			group.Values = append(group.Values, *data)
		}
	}

	dataBusService.SendGroup(*group)

	dataGroupsMu.Lock()
	if dataGroups[id] == nil {
		dataGroups[id] = make(map[string]*databus.DataGroup)
	}
	dataGroups[id][group.ID] = group
	dataGroupsMu.Unlock()
}

func (r *RedfishDevice) RestartEventListener() {
	go r.Redfish.ListenForEvents(r.Ctx, r.Events)
}

// StartEventListener Directly responsible for receiving SSE events from iDRAC. Will parse received reports or issue a
// message in the log indicating it received an unknown SSE event.
func (r *RedfishDevice) StartEventListener(dataBusService *databus.DataBusService) {
	if r.Events == nil {
		r.Events = make(chan *redfish.RedfishEvent, 10)
	}
	timer := time.AfterFunc(time.Minute*5, r.RestartEventListener)
	log.Printf("%s: Starting event listener...\n", r.SystemID)
	go r.Redfish.ListenForEvents(r.Ctx, r.Events)
	for {
		event := <-r.Events
		timer.Reset(time.Minute * 5)
		r.LastEvent = time.Now()
		if event != nil && event.Payload != nil &&
			event.Payload.Object["@odata.id"] != nil {
			log.Printf("%s: Got new report for %s\n", r.SystemID, event.Payload.Object["@odata.id"].(string))
			parseReport(event.Payload, r.SystemID, dataBusService)
		} else {
			//log.Printf("%s: Got unknown SSE event %v\n", r.SystemID, event.Payload)
			log.Printf("%s: Got unknown SSE event \n", r.SystemID)
		}
	}
}

// getTelemetry Starts the service which will listen for SSE reports from the iDRAC
func getTelemetry(r *RedfishDevice, telemetryService *redfish.RedfishPayload, dataBusService *databus.DataBusService) {
	metricReports, err := telemetryService.GetPropertyByName("MetricReports")
	if err != nil {
		log.Printf("Error retrieving metric reports for %s: %v\n", r.SystemID, err)
		return
	}
	size := metricReports.GetCollectionSize()
	if size == 0 {
		log.Printf("%s: No metric reports!\n", r.SystemID)
	}
	log.Printf("%s: Found %d Metric Reports\n", r.Redfish.Hostname, size)
	for i := 0; i < size; i++ {
		metricReport, err := metricReports.GetPropertyByIndex(i)
		if err != nil {
			log.Printf("Unable to get metric report %d: %v", i, err)
			continue
		}
		parseReport(metricReport, r.SystemID, dataBusService)
	}
	r.State = databus.RUNNING
	r.StartEventListener(dataBusService)
}

// getRedfishLce Starts the service which will listens for Redfish LifeCycle Events from the iDRAC
func getRedfishLce(r *RedfishDevice, eventService *redfish.RedfishPayload, dataBusService *databus.DataBusService) {
	eventsIn := make(chan *redfish.RedfishEvent, 10)
	//contxt := r.Ctx
	go r.Redfish.GetLceSSE(r.Ctx, eventsIn, "https://"+r.Redfish.Hostname+"/redfish/v1/SSE") //nolint: errcheck
	//eventsOut := new(redfish.RedfishEvent)
	eventsOut := <-eventsIn
	eventSvc := eventsOut.Payload
	size := eventSvc.GetEventSize()
	if size == 0 {
		log.Printf("%s: No Redfish LifeCycle Events!\n", r.SystemID)
	}
	log.Printf("%s: Found %d Redfish LifeCycle Events\n", r.Redfish.Hostname, size)
	for i := 0; i < size; i++ {
		parseRedfishLce(eventSvc, r.SystemID, dataBusService)
	}
	r.State = databus.RUNNING
	r.StartEventListener(dataBusService)
}

// Take an instance of a Redfish device, get its system ID, get any child devices if it is a chassis, and then start
// listening for SSE events. NOTE: This expects that someone has enabled Telemetry reports and started the telemetry
// service externally.
func redfishMonitorStart(r *RedfishDevice, dataBusService *databus.DataBusService) {
	systemID, err := r.Redfish.GetSystemId()
	if err != nil {
		log.Printf("%s: Failed to get system id! %v\n", r.Redfish.Hostname, err)
		return
	}
	log.Printf("%s: Got System ID %s\n", r.Redfish.Hostname, systemID)
	r.SystemID = systemID
	serviceRoot, err := r.Redfish.GetUri("/redfish/v1")
	if err != nil {
		log.Println(err)
		return
	}
	if r.HasChildren {
		r.ChildDevices = make(map[int]string)
		populateChildChassis(r, serviceRoot)
	}
	//Does this system support Telemetry?
	telemetryService, err := serviceRoot.GetPropertyByName("TelemetryService")
	if err != nil {
		log.Println("TODO: Fake some basic telemetry...") // TODO
		r.State = databus.STOPPED
	} else {
		log.Printf("%s: Using Telemetry Service...\n", r.Redfish.Hostname)
		go getTelemetry(r, telemetryService, dataBusService)
	}

	//Checking for EventService support
	eventService, err := serviceRoot.GetPropertyByName("EventService")
	if err != nil {
		log.Println("EventService not supported...")
		r.State = databus.STOPPED
	} else {
		log.Printf("%s: Event Service consumption loading...\n", r.Redfish.Hostname)
		go getRedfishLce(r, eventService, dataBusService)
	}
}

// handleAuthServiceChannel Authenticates to the iDRAC and then launches the telemetry monitoring process via
// redfishMonitorStart
func handleAuthServiceChannel(serviceIn chan *auth.Service, dataBusService *databus.DataBusService) {
	for {
		service := <-serviceIn
		if devices[service.Ip] != nil {
			continue
		}
		log.Print("Got new service = ", service.Ip)
		var r *redfish.RedfishClient
		var err error
		//log.Println(service)
		if service.AuthType == auth.AuthTypeUsernamePassword {
			r, err = redfish.Init(service.Ip, service.Auth["username"], service.Auth["password"])
		} else if service.AuthType == auth.AuthTypeBearerToken {
			r, err = redfish.InitBearer(service.Ip, service.Auth["token"])
		}
		if err != nil {
			log.Printf("%s: Failed to instantiate redfish client %v", service.Ip, err)
			continue
		}
		//log.Print(r)
		device := new(RedfishDevice)
		device.Redfish = r
		device.State = databus.STARTING
		device.HasChildren = service.ServiceType == auth.MSM
		ctx, cancel := context.WithCancel(context.Background())
		device.Ctx = ctx
		device.CtxCancel = cancel

		if devices == nil {
			devices = make(map[string]*RedfishDevice)
		}
		devices[service.Ip] = device
		go redfishMonitorStart(device, dataBusService)
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

	dataGroups = make(map[string]map[string]*databus.DataGroup)
	authClient := new(auth.AuthorizationClient)
	dataBusService := new(databus.DataBusService)

	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			authClient.Bus = mb
			dataBusService.Bus = mb
			defer mb.Close()
			break
		}
	}

	serviceIn := make(chan *auth.Service, 10)
	commands := make(chan *databus.Command)

	log.Print("Redfish Telemetry Read Service is initialized")

	authClient.ResendAll()
	go authClient.GetService(serviceIn)
	go handleAuthServiceChannel(serviceIn, dataBusService) // THIS FUNCTION ADDS SERVICE
	go dataBusService.ReceiveCommand(commands)             //nolint: errcheck
	for {
		command := <-commands
		log.Printf("Received command in redfishread: %s", command.Command)
		switch command.Command {
		case databus.GET:
			dataGroupsMu.Lock()
			for _, system := range dataGroups {
				for _, group := range system {
					dataBusService.SendGroupToQueue(*group, command.ReceiveQueue)
				}
			}
			dataGroupsMu.Unlock()
		case databus.GETPRODUCERS:
			producers := make([]*databus.DataProducer, len(devices))
			i := 0
			for _, dev := range devices {
				producer := new(databus.DataProducer)
				producer.Hostname = dev.Redfish.Hostname
				producer.Username = dev.Redfish.Username
				producer.State = dev.State
				producer.LastEvent = dev.LastEvent
				producers[i] = producer
				i = i + 1
			}
			err := dataBusService.SendProducersToQueue(producers, command.ReceiveQueue)
			if err != nil {
				log.Printf("aft SendProducersToQueue got error,so continue")
			}
		case databus.DELETEPRODUCER:
			devices[command.ServiceIP].CtxCancel()
			log.Printf("service has been cancelled, Ctx = ", devices[command.ServiceIP].Ctx)
			time.Sleep(2 * time.Second)
			delete(devices, command.ServiceIP)
		case auth.TERMINATE:
			os.Exit(0)
		}
	}
}
