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

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/sse"
)

type RedfishClient struct {
	Hostname    string
	Username    string
	Password    string
	BearerToken string
	HttpClient  *http.Client
	IsIPv6      int
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

func (r *RedfishClient) ListenForEvents(Ctx context.Context, event chan<- *RedfishEvent) {
	ret := new(RedfishEvent)
	serviceRoot, err := r.GetUri("/redfish/v1")
	if err == nil {
		eventService, err := serviceRoot.GetPropertyByName("EventService")
		if err == nil {
			if eventService.Object["ServerSentEventUri"] != nil {
				ret.Err = r.GetSSE(Ctx, event, eventService)

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

func (r *RedfishClient) GetMetricReportsSSE(Ctx context.Context, event chan<- *RedfishEvent, sseURI string) error {
	sseConfig := new(sse.Config)
	sseConfig.Client = r.HttpClient
	sseConfig.RequestCreator = func() *http.Request {
		req, err := http.NewRequest("GET", sseURI+"?$filter=EventFormatType%20eq%20MetricReport", nil)
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
				log.Println("Error reading! ", err)
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

func (r *RedfishClient) GetEventsSSE(event chan<- *RedfishEvent, sseURI string) error {
	sseConfig := new(sse.Config)
	sseConfig.Client = r.HttpClient
	sseConfig.RequestCreator = func() *http.Request {
		req, err := http.NewRequest("GET", sseURI+"?$filter=EventFormatType%20eq%Event", nil)
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
	return nil
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
	fmt.Println("GSR: GetLceSSE connection made. Ctx = ", Ctx)
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

func (r *RedfishClient) GetSSE(Ctx context.Context, event chan<- *RedfishEvent, eventService *RedfishPayload) error {
	sseUri := "https://" + r.Hostname + eventService.Object["ServerSentEventUri"].(string)
	return r.GetMetricReportsSSE(Ctx, event, sseUri)
	//return r.GetEventsSSE(event, sseUri)
}
