// Licensed to You under the Apache License, Version 2.0.

package redfish

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/sse"
)

const (
	mrSSEFilter     = "?$filter=EventFormatType%20eq%20MetricReport"
	mrSSEFilter17G  = "?$filter=EventFormatType%20eq%20%27MetricReport%27"
	evtSSEFilter    = "?$filter=EventFormatType%20eq%20Event"
	evtSSEFilter17G = "?$filter=EventType%20eq%20%27Alert%27"
)

type RedfishClient struct {
	Hostname    string
	Username    string
	Password    string
	BearerToken string
	HttpClient  *http.Client
	IsIPv6      int
	FwVer       string
}

type RedfishEvent struct {
	Err     error
	ID      string
	Payload *RedfishPayload
}

func Init(hostname string, username string, password string) (*RedfishClient, error) {
	ret := new(RedfishClient)
	ret.Hostname = hostname
	ret.Username = username
	ret.Password = password
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	//Allow a max of 5 connections in the http client connection pool
	//tr.MaxIdleConns = 5
	//tr.MaxIdleConnsPerHost = 5
	ret.HttpClient = &http.Client{Transport: tr}
	_, err := ret.GetUri("/redfish/v1")
	if err != nil {
		log.Print("Failed to init redfish client: ", err)
		return nil, err
	}
	return ret, nil
}

func InitBearer(hostname string, token string) (*RedfishClient, error) {
	ret := new(RedfishClient)
	ret.Hostname = hostname
	ret.BearerToken = token
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	ret.HttpClient = &http.Client{Transport: tr}
	_, err := ret.GetUri("/redfish/v1")
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (r *RedfishClient) addAuthToRequest(req *http.Request) {
	if r.BearerToken != "" {
		req.Header.Add("Authorization", "Bearer "+r.BearerToken)
	} else {
		req.SetBasicAuth(r.Username, r.Password)
	}
}

func (r *RedfishClient) GetUri(uri string) (*RedfishPayload, error) {
	if r.IsIPv6 == 0 {
		split := strings.Split(r.Hostname, ":")
		if len(split) > 2 {
			r.IsIPv6 = 1
			r.Hostname = "[" + r.Hostname + "]"
		} else {
			r.IsIPv6 = 2
		}
	}
	req, err := http.NewRequest("GET", "https://"+r.Hostname+uri, nil)
	if err != nil {
		return nil, err
	}
	r.addAuthToRequest(req)
	req.Header.Add("Accept", "application/json")
	resp, err := r.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetUri failed for %s with Error code %d",
			r.Hostname+uri, resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ret := new(RedfishPayload)
	err = json.Unmarshal(body, &ret.Object)
	if err != nil {
		return nil, fmt.Errorf("Body %s could not parse to JSON %w", body, err)
	}
	ret.Client = r
	return ret, nil
}

func (r *RedfishClient) Walk() map[string]*RedfishPayload {
	res := make(map[string]*RedfishPayload)
	r.walkUri("/redfish/v1", &res)
	return res
}

func (r *RedfishClient) walkUri(uri string, res *map[string]*RedfishPayload) {
	payload, err := r.GetUri(uri)
	if err != nil {
		return
	}
	(*res)[uri] = payload
	payload.walk(res)
}

func (r *RedfishClient) GetSysInfo() (hostname, sku, model, fwver, fqdn, imgid string, err error) {
	serviceRoot, err := r.GetUri("/redfish/v1/Systems/System.Embedded.1?$select=HostName,SKU,Model")
	if err != nil {
		return
	}
	//iDRAC
	if serviceRoot.Object["HostName"] != nil {
		hostname = serviceRoot.Object["HostName"].(string)
	}
	if serviceRoot.Object["SKU"] != nil {
		sku = serviceRoot.Object["SKU"].(string)
	}
	if serviceRoot.Object["Model"] != nil {
		model = serviceRoot.Object["Model"].(string)
	}

	serviceRoot, err = r.GetUri("/redfish/v1/Managers/iDRAC.Embedded.1/EthernetInterfaces/NIC.1?$select=FQDN")
	if err != nil {
		return
	}
	//iDRAC
	if serviceRoot.Object["FQDN"] != nil {
		fqdn = serviceRoot.Object["FQDN"].(string)
	}

	serviceRoot, err = r.GetUri("/redfish/v1/Managers/iDRAC.Embedded.1?$select=FirmwareVersion,Links")
	if err != nil {
		return
	}
	//iDRAC
	if serviceRoot.Object["FirmwareVersion"] != nil {
		fwver = serviceRoot.Object["FirmwareVersion"].(string)
	}

	if serviceRoot.Object["Links"] != nil {
		act, ok := serviceRoot.Object["Links"].(map[string]interface{})["ActiveSoftwareImage"]
		if ok {
			imgid = act.(map[string]interface{})["@odata.id"].(string)
			if imgid != "" {
				imgid = imgid[strings.LastIndex(imgid, "/")+1:]
			}
		}
	}
	return
}

func (r *RedfishClient) GetSystemId() (string, error) {
	serviceRoot, err := r.GetUri("/redfish/v1")
	if err != nil {
		return "", err
	}
	//iDRAC
	if serviceRoot.Object["Oem"] != nil {
		//log.Printf("%s: Has Oem elem!", r.Hostname)
		oem := valueToPayload(r, serviceRoot.Object["Oem"])
		if oem.Object["Dell"] != nil {
			dell := valueToPayload(r, oem.Object["Dell"])
			//log.Printf("%s: Has Oem/Dell elem! %v", r.Hostname, dell)
			if dell.Object["ServiceTag"] != nil {
				return dell.Object["ServiceTag"].(string), nil
			}
		}
	}
	//log.Printf("%s: No Oem/Dell/ServiceTag elem!", r.Hostname)
	chassisCollection, err := serviceRoot.GetPropertyByName("Chassis")
	if err != nil {
		return "", err
	}
	size := chassisCollection.GetCollectionSize()
	for i := 0; i < size; i++ {
		chassis, err := chassisCollection.GetPropertyByIndex(i)
		if err != nil {
			continue
		}
		if chassis.Object["ChassisType"].(string) == "Enclosure" {
			if chassis.Object["Name"].(string) == "Blade Chassis" {
				//EC case...
				if chassis.Object["SKU"].(string) != "" {
					return chassis.Object["SKU"].(string), nil
				}
				oemChassis, err := r.GetUri(chassis.Object["@odata.id"].(string) + "/Attributes")
				if err != nil {
					return "", err
				}
				attributes := valueToPayload(r, oemChassis.Object["Attributes"])
				return attributes.Object["NIC.1.MACAddress"].(string), nil
			}
			return chassis.Object["Name"].(string), nil
		}
	}
	return "", errors.New("Unable to determine System ID")
}

func (r *RedfishClient) ListenForAlerts(Ctx context.Context, event chan<- *RedfishEvent) {
	ret := new(RedfishEvent)
	serviceRoot, err := r.GetUri("/redfish/v1")
	if err == nil {
		eventService, err := serviceRoot.GetPropertyByName("EventService")
		if err == nil {
			if eventService.Object["ServerSentEventUri"] != nil {
				sseUri := "https://" + r.Hostname + eventService.Object["ServerSentEventUri"].(string)
				filter := evtSSEFilter
				if strings.Compare(r.FwVer, "4.00.00.00") < 0 {
					filter = evtSSEFilter17G
				}
				ret.Err = r.StartSSE(Ctx, event, sseUri+filter)

			} else {
				log.Println("Don't support POST back yet!")
				ret.Err = errors.New("Don't support POST back yet!")
			}
		} else {
			ret.Err = err
		}
	} else {
		log.Println("Unable to get service root!", err)
		ret.Err = err
	}
	if ret.Err != nil {
		event <- ret
	}
}
func (r *RedfishClient) ListenForMetricReports(Ctx context.Context, event chan<- *RedfishEvent) {
	ret := new(RedfishEvent)
	serviceRoot, err := r.GetUri("/redfish/v1")
	if err == nil {
		eventService, err := serviceRoot.GetPropertyByName("EventService")
		if err == nil {
			if eventService.Object["ServerSentEventUri"] != nil {
				sseUri := "https://" + r.Hostname + eventService.Object["ServerSentEventUri"].(string)
				filter := mrSSEFilter
				if strings.Compare(r.FwVer, "4.00.00.00") < 0 {
					filter = mrSSEFilter17G
				}
				ret.Err = r.StartSSE(Ctx, event, sseUri+filter)
			} else {
				log.Println("Don't support POST back yet!")
				ret.Err = errors.New("Don't support POST back yet!")
			}
		} else {
			ret.Err = err
		}
	} else {
		log.Println("Unable to get service root!", err)
		ret.Err = err
	}
	if ret.Err != nil {
		event <- ret
	}
}

func (r *RedfishClient) ListenForLceEvents(Ctx context.Context, event chan<- *RedfishEvent) {
	ret := new(RedfishEvent)
	serviceRoot, err := r.GetUri("/redfish/v1")
	if err == nil {
		eventService, err := serviceRoot.GetPropertyByName("EventService")
		sseUri := "https://" + r.Hostname + eventService.Object["ServerSentEventUri"].(string)
		if err == nil {
			if eventService.Object["ServerSentEventUri"] != nil {
				ret.Err = r.GetLceSSE(Ctx, event, sseUri)

			} else {
				log.Println("Don't support POST back yet!")
				ret.Err = errors.New("Don't support POST back yet!")
			}
		} else {
			ret.Err = err
		}
	} else {
		log.Println("Unable to get service root!", err)
		ret.Err = err
	}
	if ret.Err != nil {
		event <- ret
	}
}

func (r *RedfishClient) StartSSE(Ctx context.Context, event chan<- *RedfishEvent, sseURI string) error {
	sseConfig := new(sse.Config)
	sseConfig.Client = r.HttpClient
	//iDRAC version
	filter := mrSSEFilter
	serviceRoot, err := r.GetUri("/redfish/v1/Managers/iDRAC.Embedded.1?$select=FirmwareVersion")
	if err == nil {
		if serviceRoot.Object["FirmwareVersion"] != nil {
			fwver, ok := serviceRoot.Object["FirmwareVersion"].(string)
			if ok && strings.Compare(fwver, "4.00.00.00") < 0 {
				filter = mrSSEFilter17G
			}
		}
	}
	log.Println("SSE Metric Report Filter: ", filter)

	lastTS := time.Now() // Variable to hold the latest event timestamp
	sseConfig.RequestCreator = func() *http.Request {
		req, err := http.NewRequest("GET", sseURI, nil)
		if err != nil {
			return nil
		}
		r.addAuthToRequest(req)
		//req.Header.Add("Accept", "text/event-stream")
		req.Header.Add("Accept", "*/*")
		return req
	}
	sseSource, err := sseConfig.Connect()
	if err != nil {
		log.Println("Error connecting! ", err)
		err = errors.New("connection error")
		return err
	}
	for {
		select {
		case <-Ctx.Done():
			sseSource.Close()
			return nil
		default:
			sseEvent, err := sseSource.Next()
			redfishEvent := new(RedfishEvent)
			redfishEvent.ID = sseEvent.ID
			if err != nil {
				redfishEvent.Err = err
				if strings.Contains(err.Error(), "EOF") {
					// EOF denotes a terminated SSE connection.
					if lastTS.Before(time.Now().Add(-time.Minute * 60)) {
						// SSE connection times out if no events have been sent in around 60 minutes.
						err = errors.New("sse idle timeout")
						redfishEvent.Err = err
					} else {
						err = errors.New("connection error")
						redfishEvent.Err = err
					}
				}
				redfishEvent.Payload = nil
				event <- redfishEvent
				// Sending an error event triggers a reconnect to the SSE source (RestartEventListener).
				// Hence, closing the SSE source here gracefully.
				sseSource.Close()
				return nil
			}

			lastTS = time.Now() // Update the latest event timestamp
			ret := new(RedfishPayload)
			err = json.Unmarshal(sseEvent.Data, &ret.Object)
			if err != nil {
				log.Printf("Failed to parse message %v", err)
				continue
			}
			ret.Client = r
			redfishEvent.Payload = ret
			redfishEvent.Err = nil
			event <- redfishEvent
		}
	}
}

func (r *RedfishClient) GetEventsSSE(Ctx context.Context, event chan<- *RedfishEvent, sseURI string) error {
	sseConfig := new(sse.Config)
	sseConfig.Client = r.HttpClient
	sseConfig.RequestCreator = func() *http.Request {
		filter := evtSSEFilter
		if strings.Compare(r.FwVer, "4.00.00.00") < 0 {
			filter = evtSSEFilter17G
		}
		req, err := http.NewRequest("GET", sseURI+filter, nil)
		if err != nil {
			return nil
		}
		r.addAuthToRequest(req)
		//req.Header.Add("Accept", "text/event-stream")
		req.Header.Add("Accept", "*/*")
		return req
	}
	sseSource, err := sseConfig.Connect()
	if err != nil {
		return err
	}
	for {
		select {
		case <-Ctx.Done():
			sseSource.Close()
			return nil
		default:
			sseEvent, err := sseSource.Next()
			if err != nil {
				break
			}
			redfishEvent := new(RedfishEvent)
			redfishEvent.ID = sseEvent.ID
			ret := new(RedfishPayload)
			err = json.Unmarshal(sseEvent.Data, &ret.Object)
			if err != nil {
				log.Printf("Failed to parse message %v", err)
				continue
			}
			ret.Client = r
			redfishEvent.Payload = ret
			event <- redfishEvent
		}
	}
}

func (r *RedfishClient) GetLceSSE(Ctx context.Context, event chan<- *RedfishEvent, sseURI string) error {
	sseConfig := new(sse.Config)
	sseConfig.Client = r.HttpClient
	sseConfig.RequestCreator = func() *http.Request {
		req, err := http.NewRequest("GET", sseURI+"?$filter=EventType%20eq%20%27Other%27", nil)
		if err != nil {
			return nil
		}
		r.addAuthToRequest(req)
		req.Header.Add("Accept", "*/*")
		return req
	}
	sseConfig.RetryParams.MaxRetries = 5
	sseSource, err := sseConfig.Connect()
	if err != nil {
		return err
	}
	// waiting for context to be cancelled by listening to Ctx.Done() channel
	// if someone cancelled it, it will close the connection
	for {
		select {
		case <-Ctx.Done():
			sseSource.Close()
			return nil
		default:
			sseEvent, err := sseSource.Next()
			if err != nil {
				fmt.Println("SSESource.Next() Errored, breaking from loop")
				fmt.Println(err)
				break
			}
			redfishEvent := new(RedfishEvent)
			redfishEvent.ID = sseEvent.ID
			ret := new(RedfishPayload)
			err = json.Unmarshal(sseEvent.Data, &ret.Object)
			if err != nil {
				log.Printf("Failed to parse message %v", err)
				continue
			}
			ret.Client = r
			redfishEvent.Payload = ret
			event <- redfishEvent
		}
	}
}

func (r *RedfishClient) GetSSEByUri(event chan<- *RedfishEvent, sseURI string) {
	errEvent := new(RedfishEvent)
	sseConfig := new(sse.Config)
	sseConfig.Client = r.HttpClient
	sseConfig.RequestCreator = func() *http.Request {
		req, err := http.NewRequest("GET", "https://"+r.Hostname+sseURI, nil)
		if err != nil {
			return nil
		}
		r.addAuthToRequest(req)
		//req.Header.Add("Accept", "text/event-stream")
		req.Header.Add("Accept", "*/*")
		return req
	}
	sseSource, err := sseConfig.Connect()
	if err != nil {

		errEvent.Err = err
		event <- errEvent
		return
	}
	for {
		sseEvent, err := sseSource.Next()
		if err != nil {
			errEvent.Err = err
			event <- errEvent
			break
		}
		redfishEvent := new(RedfishEvent)
		redfishEvent.ID = sseEvent.ID
		ret := new(RedfishPayload)
		err = json.Unmarshal(sseEvent.Data, &ret.Object)
		if err != nil {
			log.Printf("Failed to parse message %v", err)
			continue
		}
		ret.Client = r
		redfishEvent.Payload = ret
		event <- redfishEvent
	}

}

func (r *RedfishClient) GetInventoryByUri(sseURI string) (*RedfishPayload, error) {
	sseConfig := new(sse.Config)
	sseConfig.Client = r.HttpClient
	sseConfig.RequestCreator = func() *http.Request {
		req, err := http.NewRequest("GET", "https://"+r.Hostname+sseURI, nil)
		if err != nil {
			return nil
		}
		r.addAuthToRequest(req)
		//req.Header.Add("Accept", "text/event-stream")
		req.Header.Add("Accept", "*/*")
		return req
	}
	sseSource, err := sseConfig.Connect()
	if err != nil {
		log.Printf("Error %s while seconfigConnect while GetInventoryByUri\n", err)
		return nil, err
	}
	sseEvent, err := sseSource.Next()
	if err != nil {
		log.Printf("Error %s while sseSourceNext while GetInventoryByUri\n", err)
		log.Printf("^^^^^^%s^^^^^^^^^^^^^\n", sseEvent)
	}
	ret := new(RedfishPayload)
	err = json.Unmarshal(sseEvent.Data, &ret.Object)
	if err != nil {
		log.Printf("Failed to parse message %v", err)
		return nil, err
	}
	ret.Client = r
	return ret, nil
}
