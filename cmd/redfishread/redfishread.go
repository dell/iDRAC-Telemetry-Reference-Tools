// Licensed to You under the Apache License, Version 2.0.
// This script is responsible for reading data from Redfish into the ingest pipeline.

package main

import (
	"context"
	//"encoding/json"
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
		if id == "" && value.Object["MetricProperty"] != nil {
			id = value.Object["MetricProperty"].(string)
			//get last part of MP, /abc/def#ghi => def_ghi
			li := strings.LastIndex(id, "/")
			if li != -1 {
				id = id[li+1:]
			}
			id = strings.ReplaceAll(id, "#", "_")
		}
	}
	if id == "" {
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
func parseReport(metricReport *redfish.RedfishPayload, r *RedfishDevice, dataBusService *databus.DataBusService) {
	metricValues, err := metricReport.GetPropertyByName("MetricValues")
	if err != nil {
		log.Printf("%s: Unable to get metric report's MetricValues: %v %v", r.SystemID, err, metricReport)
		return
	}
	group := new(databus.DataGroup)

	group.HostName = r.HostName
	group.FQDN = r.FQDN
	group.System = r.SystemID
	group.Model = r.Model
	group.SKU = r.SKU
	group.FwVer = r.FwVer
	group.ImgID = r.ImgID
	group.ID = metricReport.Object["Id"].(string)
	group.Label = metricReport.Object["Name"].(string)
	group.Timestamp = metricReport.Object["Timestamp"].(string)
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
			data.System = r.SystemID
			data.HostName = r.HostName
			group.Values = append(group.Values, *data)
		}
	}
	dataBusService.SendGroup(*group)

	dataGroupsMu.Lock()
	if dataGroups[r.SystemID] == nil {
		dataGroups[r.SystemID] = make(map[string]*databus.DataGroup)
	}
	dataGroups[r.SystemID][group.ID] = group
	dataGroupsMu.Unlock()
}

func parseRedfishEvents(events *redfish.RedfishPayload, r *RedfishDevice, dataBusService *databus.DataBusService) {
	id := r.SystemID
	eventData, err := events.GetPropertyByName("Events")
	if err != nil {
		log.Printf("%s: Unable to get eventData: %v", id, err)
		return
	}
	log.Printf("RedFish Events Found for parsing: %v\n", eventData)

	group := new(databus.DataGroup)
	group.HostName = r.HostName
	group.FQDN = r.FQDN
	group.System = r.SystemID
	group.Model = r.Model
	group.SKU = r.SKU
	group.FwVer = r.FwVer
	group.ImgID = r.ImgID

	group.ID = events.Object["Id"].(string)
	//group.Label = events.Object["Name"].(string)
	size := eventData.GetArraySize()
	for j := 0; j < size; j++ {
		eventData, err := events.GetEventByIndex(j)
		if err != nil {
			log.Printf("Unable to retrieve the redfish events\n")
			return
		}
		if eventData.Object["EventId"] != nil {
			data := new(databus.EventValue)

			originCondition, err := eventData.GetPropertyByName("OriginOfCondition")
			if err != nil {
				log.Printf("Unable to get property %v\n", err)
			} else {
				data.OriginOfCondition = originCondition.Object["@odata.id"].(string)
			}
			data.EventId = eventData.Object["EventId"].(string)
			data.EventType = eventData.Object["EventType"].(string)
			data.EventTimestamp = eventData.Object["EventTimestamp"].(string)
			data.MemberId = eventData.Object["MemberId"].(string)
			data.MessageSeverity = eventData.Object["MessageSeverity"].(string)
			data.Message = eventData.Object["Message"].(string)
			data.MessageId = eventData.Object["MessageId"].(string)
			if args, ok := eventData.Object["MessageArgs"]; ok {
				if args != nil {
					for _, a := range args.([]interface{}) {
						data.MessageArgs = append(data.MessageArgs, a.(string))
					}
				}
			}
			group.Events = append(group.Events, *data)
		}
	}
	dataBusService.SendGroup(*group)

	dataGroupsMu.Lock()
	if dataGroups[r.SystemID] == nil {
		dataGroups[r.SystemID] = make(map[string]*databus.DataGroup)
	}
	dataGroups[r.SystemID][group.ID] = group
	dataGroupsMu.Unlock()
}

/*
// Responsible for taking the lifecycle events received from SSE, getting its events, and then sending it along the
// data bus
func parseRedfishLce(lceevents *redfish.RedfishPayload, r *RedfishDevice, dataBusService *databus.DataBusService) {
	id := r.SystemID
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
			return
		}
		if eventData.Object["EventId"] != nil {
			data := new(databus.DataValue)
			originCondition, err := eventData.GetPropertyByName("OriginOfCondition")
			if err != nil {
				log.Printf("Unable to get property %v\n", err)
			}
			data.Value = fmt.Sprint(originCondition)
			data.MessageId = eventData.Object["MessageId"].(string)
			data.EventType = eventData.Object["EventType"].(string)
			if eventData.Object["EventTimestamp"] == nil {
				t := time.Now()
				data.Timestamp = t.Format("2006-01-02T15:04:05-0700")
			} else {
				data.Timestamp = eventData.Object["EventTimestamp"].(string)
			}
			if originCondition != nil {
				if strings.Contains(originCondition.Object["@odata.id"].(string), configStrings["inventoryurl"]) {
					map_oc, err := r.Redfish.GetUri(originCondition.Object["@odata.id"].(string))
					if err != nil {
						log.Printf("ERROR: %s\n", err)
					}
					inv_oem, err := map_oc.GetPropertyByName("Oem")
					if err != nil {
						log.Printf("Unable to get OEM %s\n", err)
					}
					if inv_oem != nil {
						inv_DellOem, err := inv_oem.GetPropertyByName("Dell")
						if err != nil {
							log.Printf("Unable to get DELL metrics %s\n", err)
						}
						if inv_DellOem != nil {
							inv_DellNIC, err := inv_DellOem.GetPropertyByName("DellNIC")
							if err != nil {
								log.Printf("Unable to get DellNIC metrics %s\n", err)
							} else {
								data.MaxBandwidthPercent = inv_DellNIC.Object["MaxBandwidthPercent"].(float64)
								data.MinBandwidthPercent = inv_DellNIC.Object["MinBandwidthPercent"].(float64)
							}
							inv_DellNICPortMetrics, err := inv_DellOem.GetPropertyByName("DellNICPortMetrics")
							if err != nil {
								log.Printf("Unable to get NICPortMetrics%s\n", err)
							} else {
								data.Context = inv_DellNICPortMetrics.Object["@odata.context"].(string)
								data.Label = inv_DellNICPortMetrics.Object["@odata.type"].(string)
								data.ID = inv_DellNICPortMetrics.Object["@odata.id"].(string)
								data.DiscardedPkts = inv_DellNICPortMetrics.Object["DiscardedPkts"].(float64)
								broadcast := inv_DellNICPortMetrics.Object["RxBroadcast"]
								data.RxBroadcast = broadcast.(float64)
								data.RxBytes = inv_DellNICPortMetrics.Object["RxBytes"].(float64)
								data.RxErrorPktAlignmentErrors = inv_DellNICPortMetrics.Object["RxErrorPktAlignmentErrors"].(float64)
								data.RxMulticastPackets = inv_DellNICPortMetrics.Object["RxMutlicastPackets"].(float64)
								data.RxUnicastPackets = inv_DellNICPortMetrics.Object["RxUnicastPackets"].(float64)
								data.TxBroadcast = inv_DellNICPortMetrics.Object["TxBroadcast"].(float64)
								data.TxBytes = inv_DellNICPortMetrics.Object["TxBytes"].(float64)
								data.TxMutlicastPackets = inv_DellNICPortMetrics.Object["TxMutlicastPackets"].(float64)
								data.TxUnicastPackets = inv_DellNICPortMetrics.Object["TxUnicastPackets"].(float64)
							}
						}
					}
				}
			}
			data.System = r.SystemID
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
*/

func (r *RedfishDevice) RestartMetricListener() {
	go r.Redfish.ListenForMetricReports(r.Ctx, r.Metrics)
}

func (r *RedfishDevice) RestartAlertListener() {
	go r.Redfish.ListenForAlerts(r.Ctx, r.Events)
}

func (r *RedfishDevice) RestartLceEventListener() {
	go r.Redfish.ListenForLceEvents(r.Ctx, r.Events)
}

// StartMetricListener Directly responsible for receiving SSE events from iDRAC. Will parse received reports or issue a
// message in the log indicating it received an unknown SSE event.
func (r *RedfishDevice) StartMetricListener(dataBusService *databus.DataBusService) {
	if r.Metrics == nil {
		r.Metrics = make(chan *redfish.RedfishEvent, 10)
	}
	//timer := time.AfterFunc(time.Minute*5, r.RestartAlertListener)
	log.Printf("%s: Starting metric listener...\n", r.SystemID)
	go r.Redfish.ListenForMetricReports(r.Ctx, r.Metrics)
	for {
		event := <-r.Metrics
		if event == nil {
			log.Printf("%s: Got SSE nil event \n", r.SystemID)
			continue
		}
		if event.Err != nil { // SSE connect failure , retry connection
			log.Printf("%s: Got SSE error %s\n", r.SystemID, event.Err)
			if strings.Contains(event.Err.Error(), "connection error") {
				// Wait for 5 minutes before restarting, so that the iDRAC can be rebooted
				// and SSE connection can be re-established

				log.Printf("Sleep 5 minutes before restarting SSE connection for %s\n", r.SystemID)
				time.Sleep(time.Minute * 5)
			}
			r.RestartMetricListener()
			continue
		}
		r.LastEvent = time.Now()
		if event.Payload != nil {
			if ot, ok := event.Payload.Object["@odata.type"].(string); ok {
				switch {
				case strings.Contains(ot, ".MetricReport"):
					log.Printf("%s: Got new report for %s\n", r.SystemID, event.Payload.Object["@odata.id"].(string))
					parseReport(event.Payload, r, dataBusService)
					continue
				default:
					log.Printf("%s: Got unknown event type %s\n", r.SystemID, ot)
				}
			}
		}
		//log.Printf("%s: Got unknown SSE event %v\n", r.SystemID, event.Payload)
		log.Printf("%s: Got bad SSE event \n", r.SystemID)
	}
}

func (r *RedfishDevice) StartAlertListener(dataBusService *databus.DataBusService) {
	if r.Events == nil {
		r.Events = make(chan *redfish.RedfishEvent, 10)
	}
	//timer := time.AfterFunc(time.Minute*5, r.RestartAlertListener)
	log.Printf("%s: Starting event listener...\n", r.SystemID)
	go r.Redfish.ListenForAlerts(r.Ctx, r.Events)
	for {
		event := <-r.Events
		if event == nil {
			log.Printf("%s: Got SSE nil event \n", r.SystemID)
			continue
		}
		if event.Err != nil { // SSE connect failure , retry connection
			log.Printf("%s: Got SSE error %s\n", r.SystemID, event.Err)
			if strings.Contains(event.Err.Error(), "connection error") {
				// Wait for 5 minutes before restarting, so that the iDRAC can be rebooted
				// and SSE connection can be re-established

				log.Printf("Sleep 5 minutes before restarting SSE connection for %s\n", r.SystemID)
				time.Sleep(time.Minute * 5)
			}
			r.RestartAlertListener()
			continue
		}
		r.LastEvent = time.Now()
		if event.Payload != nil {
			if ot, ok := event.Payload.Object["@odata.type"].(string); ok {
				switch {
				case strings.Contains(ot, ".Event"):
					log.Printf("%s: Got new event\n", r.SystemID)
					parseRedfishEvents(event.Payload, r, dataBusService)
					continue
				default:
					log.Printf("%s: Got unknown event type %s\n", r.SystemID, ot)
				}
			}
		}
		//log.Printf("%s: Got unknown SSE event %v\n", r.SystemID, event.Payload)
		log.Printf("%s: Got bad SSE event \n", r.SystemID)
	}
}

/*
// StartLceEventListener Directly responsible for receiving Redfish LifeCycleEvents from iDRAC. Will parse received reports or issue a
// message in the log indicating it received an unknown event.
func (r *RedfishDevice) StartLceEventListener(dataBusService *databus.DataBusService) {
	if r.Events == nil {
		r.Events = make(chan *redfish.RedfishEvent, 10)
	}
	timer := time.AfterFunc(time.Minute*5, r.RestartLceEventListener)
	log.Printf("%s: Starting event listener...\n", r.SystemID)
	go r.Redfish.ListenForLceEvents(r.Ctx, r.Events)
	for {
		lceevent := <-r.Events
		timer.Reset(time.Minute * 5)
		r.LastEvent = time.Now()
		if lceevent != nil {
			log.Printf("%s: Got new event with id %s\n", r.SystemID, lceevent.Payload.Object["Id"].(string))
			parseRedfishLce(lceevent.Payload, r, dataBusService)
		} else {
			log.Printf("%s: Got unknown LCE event %v\n", r.SystemID, lceevent.Payload)
		}
	}
}
*/
// getTelemetry Starts the service which will listen for SSE reports from the iDRAC
func getTelemetry(r *RedfishDevice, telemetryService *redfish.RedfishPayload, dataBusService *databus.DataBusService) {
	r.State = databus.RUNNING
	inclAlerts := os.Getenv("INCLUDE_ALERTS")
	if inclAlerts == "true" {
		go r.StartAlertListener(dataBusService)
	}
	go r.StartMetricListener(dataBusService)

}

/*
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
		parseRedfishLce(eventSvc, r, dataBusService)
	}
	r.State = databus.RUNNING
	r.StartLceEventListener(dataBusService)
}
*/
// Take an instance of a Redfish device, get its system ID, get any child devices if it is a chassis, and then start
// listening for SSE events. NOTE: This expects that someone has enabled Telemetry reports and started the telemetry
// service externally.
func redfishMonitorStart(r *RedfishDevice, dataBusService *databus.DataBusService) {
	systemID, err := r.Redfish.GetSystemId()
	if err != nil || systemID == "" {
		log.Printf("%s: Failed to get system id! %v\n", r.Redfish.Hostname, err)
		return
	}
	hostName, sku, model, fwver, fqdn, imgid, err := r.Redfish.GetSysInfo()
	if err != nil || hostName == "" {
		log.Printf("%s: Failed to get hostName id! %v\n", r.Redfish.Hostname, err)
		// assume same as system id, host name cannot be empty if used as key
		hostName = systemID
	}
	log.Printf("%s: Got System ID %s, Hostname %s\n", r.Redfish.Hostname, systemID, hostName)
	r.SystemID = systemID
	r.HostName = hostName
	r.SKU = sku
	r.Model = model
	r.FwVer = fwver
	r.FQDN = fqdn
	r.ImgID = imgid

	r.Redfish.FwVer = fwver

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
		r.State = databus.TELNOTFOUND
	} else {
		log.Printf("%s: Using Telemetry Service...\n", r.Redfish.Hostname)
		//go getRedfishLce(r, telemetryService, dataBusService)
		getTelemetry(r, telemetryService, dataBusService)
	}
}

// handleAuthServiceChannel Authenticates to the iDRAC and then launches the telemetry monitoring process via
// redfishMonitorStart
func handleAuthServiceChannel(serviceIn chan *auth.Service, dataBusService *databus.DataBusService) {
	for {
		service := <-serviceIn
		if service.Ip == "" {
			log.Println("Service IP is empty")
			continue
		}
		if devices[service.Ip] != nil {
			log.Printf("Device with IP %s already exists", service.Ip)
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
		//log.Print(r)
		device := new(RedfishDevice)
		if err != nil {
			log.Printf("%s: Failed to instantiate redfish client %v", service.Ip, err)
			// Creating device for failed password so that it will show up on GUI
			r = new(redfish.RedfishClient)
			r.Hostname = service.Ip
			r.Username = service.Auth["username"]
			r.Password = service.Auth["password"]
			device.State = databus.CONNFAILED
		} else {
			device.State = databus.STARTING
		}
		device.Redfish = r
		device.HasChildren = service.ServiceType == auth.MSM
		ctx, cancel := context.WithCancel(context.Background())
		device.Ctx = ctx
		device.CtxCancel = cancel
		if devices == nil {
			devices = make(map[string]*RedfishDevice)
		}
		devices[service.Ip] = device
		// Only want validated devices to be started
		if err == nil {
			go redfishMonitorStart(device, dataBusService)
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
			log.Printf("service has been cancelled, Ctx = %v", devices[command.ServiceIP].Ctx)
			time.Sleep(2 * time.Second)
			delete(devices, command.ServiceIP)
		case auth.TERMINATE:
			os.Exit(0)
		}
	}
}
