// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/auth"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/databus"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/redfish"
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
}

var devices map[string]*RedfishDevice
var dataGroups map[string]map[string]*databus.DataGroup

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
			log.Printf("Unable to get mertric report MetricValue %d: %v", j, err)
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
	if dataGroups[systemid] == nil {
		dataGroups[systemid] = make(map[string]*databus.DataGroup)
	}
	dataGroups[systemid][group.ID] = group
}

func (r *RedfishDevice) RestartEventListener() {
	go r.Redfish.ListenForEvents(r.Events)
}

func (r *RedfishDevice) StartEventListener(dataBusService *databus.DataBusService) {
	if r.Events == nil {
		r.Events = make(chan *redfish.RedfishEvent, 10)
	}
	timer := time.AfterFunc(time.Minute*5, r.RestartEventListener)
	log.Printf("%s: Starting event listener...\n", r.SystemID)
	go r.Redfish.ListenForEvents(r.Events)
	for {
		event := <-r.Events
		timer.Reset(time.Minute * 5)
		r.LastEvent = time.Now()
		if event != nil && event.Payload != nil &&
			event.Payload.Object["@odata.id"] != nil {
			log.Printf("%s: Got new report for %s\n", r.SystemID, event.Payload.Object["@odata.id"].(string))
			parseReport(event.Payload, r.SystemID, dataBusService)
		} else {
			log.Printf("%s: Got unknown SSE event %v\n", r.SystemID, event.Payload)
		}
	}
}

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
			log.Printf("Unable to get mertric report %d: %v", i, err)
			continue
		}
		parseReport(metricReport, r.SystemID, dataBusService)
	}
	r.State = databus.RUNNING
	r.StartEventListener(dataBusService)
}

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
		log.Println("TODO: Fake some basic telemetry...")
		r.State = databus.STOPPED
	} else {
		log.Printf("%s: Using Telemetry Service...\n", r.Redfish.Hostname)
		getTelemetry(r, telemetryService, dataBusService)
	}
}

func handleAuthServiceChannel(serviceIn chan *auth.Service, dataBusService *databus.DataBusService) {
	for {
		service := <-serviceIn
		if devices[service.Ip] != nil {
			continue
		}
		log.Print("Got new service = ", service.Ip)
		var r *redfish.RedfishClient
		var err error
		log.Println(service)
		if service.AuthType == auth.AuthTypeUsernamePassword {
			r, err = redfish.Init(service.Ip, service.Auth["username"], service.Auth["password"])
		} else if service.AuthType == auth.AuthTypeBearerToken {
			r, err = redfish.InitBearer(service.Ip, service.Auth["token"])
		}
		if err != nil {
			log.Printf("%s: Failed to instantiate redfish client %v", service.Ip, err)
			continue
		}
		log.Print(r)
		device := new(RedfishDevice)
		device.Redfish = r
		device.State = databus.STARTING
		device.HasChildren = service.ServiceType == auth.MSM
		if devices == nil {
			devices = make(map[string]*RedfishDevice)
		}
		devices[service.Ip] = device
		go redfishMonitorStart(device, dataBusService)
	}
}

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
			log.Printf("Could not connect to message bus: ", err)
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

	log.Print("Refish Telemetry Read Service is initialized")

	authClient.ResendAll()
	go authClient.GetService(serviceIn)
	go handleAuthServiceChannel(serviceIn, dataBusService)
	go dataBusService.RecieveCommand(commands)
	for {
		command := <-commands
		log.Printf("Recieved command: %s", command.Command)
		switch command.Command {
		case databus.GET:
			for _, system := range dataGroups {
				for _, group := range system {
					dataBusService.SendGroupToQueue(*group, command.RecieveQueue)
				}
			}
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
			dataBusService.SendProducersToQueue(producers, command.RecieveQueue)
		case auth.TERMINATE:
			os.Exit(0)
		}
	}
}
