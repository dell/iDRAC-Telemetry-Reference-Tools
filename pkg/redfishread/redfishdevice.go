package redfishread

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/redfish"
)

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
	Redfish     redfish.RedfishClientInterface
	SystemDetail
	ChildDevices map[int]string
	Events       chan *redfish.RedfishEvent
	Metrics      chan *redfish.RedfishEvent
	State        string
	LastEvent    time.Time
	CtxCancel    context.CancelFunc
	Ctx          context.Context
}

func (r *RedfishDevice) RestartAlertListener() {
	go r.Redfish.ListenForAlerts(r.Ctx, r.Events)
}

func (r *RedfishDevice) RestartMetricListener() {
	go r.Redfish.ListenForMetricReports(r.Ctx, r.Metrics)
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

func parseRedfishEvents(events *redfish.RedfishPayload, r *RedfishDevice, dataBusService *databus.DataBusService) {
	id := r.SystemID
	eventData, err := events.GetPropertyByName("Events")
	if err != nil {
		log.Printf("%s: Unable to get eventData: %v", id, err)
		return
	}
	log.Printf("RedFish Events Found for parsing: %v\n", eventData)

	group := new(databus.DataGroup)
	group.HostID = r.Redfish.GetHostname()
	group.HostName = r.HostName
	group.FQDN = r.FQDN
	group.System = r.SystemID
	group.Model = r.Model
	group.SKU = r.SKU
	group.FwVer = r.FwVer
	group.ImgID = r.ImgID

	groupId, _ := events.Object["Id"].(string)
	group.ID = groupId
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
			var ok bool
			data.EventId, ok = eventData.Object["EventId"].(string)
			if !ok {
				log.Printf("Unable to get property EventId")
			}
			data.EventType, ok = eventData.Object["EventType"].(string)
			if !ok {
				log.Printf("Unable to get property EventType")
			}
			data.EventTimestamp, ok = eventData.Object["EventTimestamp"].(string)
			if !ok {
				log.Printf("Unable to get property EventTimestamp")
			}
			data.MemberId, ok = eventData.Object["MemberId"].(string)
			if !ok {
				log.Printf("Unable to get property MemberId")
			}
			data.MessageSeverity, ok = eventData.Object["MessageSeverity"].(string)
			if !ok {
				log.Printf("Unable to get property MessageSeverity")
			}
			data.Message, ok = eventData.Object["Message"].(string)
			if !ok {
				log.Printf("Unable to get property Message")
			}
			data.MessageId, ok = eventData.Object["MessageId"].(string)
			if !ok {
				log.Printf("Unable to get property MessageId")
			}
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
	DataGroupsMap.AddDataGroup(r.SystemID, group)
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
	DataGroupsMap.AddDataGroup(r.SystemID, group)
}
