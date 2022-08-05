// Copyright (c) 2022, Dell Inc. or its subsidiaries.  All rights reserved.
// Licensed to You under the Apache License, Version 2.0.   See the LICENSE file for more details.

// This code provides a plugin for the DELL iDRAC SDK to support splunk applications.
//
// Code functionality:
//
// func main():
//
// Starts the http client, connects to the known redfish Server Side Events (SSE) endpoint,
// parses the various iDRAC metrics.The metrics are parsed into a buffer as they come in, and once
// a complete metric is detected, it gets sent over to be processed by a go routine.
//
// func processValidBuffer():
//
// This first validates the buffer content by unmarshalling the received JSON string into a map/interface structure.
// I left this code in as it is a good check on the integrity of the parsed metric buffer. Errors here can result in a panic,
// but I left the code to continue and just made a note note of the conditions. Aslo the received metrics are pretty nested
// and I had issues getting individual elements out, so ended up using a set of structures to unmarshall.
// NOTE: This can be used for analysis down the line.
// The code then unmarshalls into a set of structures and that allows individual element access.
// These then get packaged into a set of elements and consumed by the logToSplunk() function.
//
// Helper functions:
//
// func init():  For settting log levels. Happens before main gets called.
// func checkHeader():  For validating the metric header
// func getEnvSettings(): For getting the splunkURL and splunkKEY values from the environment.
// func initConfigParams(): For setting the config parameters.
// NOTE: The above two can be expanded to cover a varitey of methods to configure the system.

package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Use this to set the log level
// Panic, Fatal, Error, Warning, Info, Debug, Trace
func init() {
	// All logging above Error get printed.
	log.SetLevel(log.ErrorLevel)
}

var client = &http.Client{}
var dat map[string]interface{}
var reportCount = 0
var mtx sync.Mutex
var invalidJsonPkts = 0

type SplunkEventFields struct {
	Value      float64 `json:"_value"`
	MetricName string  `json:"metric_name"`
}

type SplunkEvent struct {
	Time   int64             `json:"time"`
	Event  string            `json:"event"`
	Host   string            `json:"host"`
	Fields SplunkEventFields `json:"fields"`
}

var configStringsMu sync.RWMutex
var configStrings = map[string]string{
	"splunkURL": "http://10.244.123.128:8088",
	"splunkKey": "a270b34f-afbd-4d5a-a2c2-520cad1ce2a7",
}

type stagDellData struct {
	ServiceTag string `json:"ServiceTag"`
}

type stagOem struct {
	DELL stagDellData `json:"Dell"`
}

type dellData struct {
	ContextID string `json:"ContextID"`
	Label     string `json:"Label"`
	Source    string `json:"Source"`
	FQDD      string `json:"FQDD"`
}

type oemData struct {
	DELL dellData `json:"Dell"`
}

type MetricValue struct {
	MetricID  string  `json:"Metricid"`
	TimeStamp string  `json:"Timestamp"`
	Value     string  `json:"MetricValue"`
	OEM       oemData `json:"Oem"`
}

type Resp struct {
	MType          string `json:"@odata.type"`
	MContext       string `json:"@odata.context"`
	MId            string `json:"@odata.id"`
	Id             string `json:"Id"`
	Name           string `json:"Name"`
	ReportSequence string `json:"ReportSequence"`
	Timestamp      string `json:"Timestamp"`
	MetricValues   []MetricValue
	Count          int     `json:"MetricValues@odata.count"`
	ServiceTag     stagOem `json:"Oem"`
}

func logToSplunk(events []*SplunkEvent) {
	var builder strings.Builder
	for _, event := range events {
		b, _ := json.Marshal(event)
		builder.Write(b)
		log.Infof("Timestamp = %d ID = %s Value = %f System = %s", event.Time, event.Fields.MetricName, event.Fields.Value, event.Host)
	}

	configStringsMu.RLock()
	url := configStrings["splunkURL"] + "/services/collector"
	key := configStrings["splunkKey"]
	configStringsMu.RUnlock()

	req, err := http.NewRequest("POST", url, strings.NewReader(builder.String()))
	if err != nil {
		log.Printf("Error creating request for %s: %v", url, err)
		return
	}

	req.Header.Add("Authorization", "Splunk "+key)
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error doing http request: ", err)
	}
	if resp.Body != nil {
		resp.Body.Close()
	}
	log.Printf("Sent to Splunk. Got back %d", resp.StatusCode)
}

func processValidBuffer(tmp []byte) {
	var res Resp
	mtx.Lock()
	reportCount++
	err := json.Unmarshal(tmp, &dat)
	mtx.Unlock()
	// NOTE: For the time being I am using the json unmarshall function as a validator for the parsed metric reports
	if err != nil {
		// Don't panic, but keep a count of these invalid packets for analysis
		// panic(err)
		invalidJsonPkts++
		return
	}
	err = json.Unmarshal(tmp, &res)
	if err != nil {
		panic(err)
	}

	log.Info("The size of the received buffer in bytes:  The number of Metric Reports processed:", len(tmp), reportCount)

	events := make([]*SplunkEvent, res.Count)
	for index, value := range res.MetricValues {
		timestamp, err := time.Parse(time.RFC3339, value.TimeStamp)
		if err != nil {
			// For why we do this see https://datatracker.ietf.org/doc/html/rfc3339#section-4.3
			// Go does not handle time properly. See https://github.com/golang/go/issues/20555
			value.TimeStamp = strings.ReplaceAll(value.TimeStamp, "+0000", "Z")
			timestamp, err = time.Parse(time.RFC3339, value.TimeStamp)
			if err != nil {
				log.Printf("Error parsing timestamp for point %s: (%s) %v", value.OEM.DELL.Label, value.TimeStamp, err)
				continue
			}
		}
		event := new(SplunkEvent)
		event.Time = timestamp.Unix()
		event.Event = "metric"
		event.Host = res.ServiceTag.DELL.ServiceTag
		floatVal, _ := strconv.ParseFloat(value.Value, 64)
		event.Fields.Value = floatVal
		event.Fields.MetricName = value.OEM.DELL.Label
		events[index] = event
	}
	logToSplunk(events)
}

func checkHeader(tmp []byte) bool {

	idPresent := bytes.Contains(tmp, []byte("id: "))
	dataPresent := bytes.Contains(tmp, []byte("data: "))
	namePresent := bytes.Contains(tmp, []byte("Name"))

	if idPresent && dataPresent && namePresent {
		return true
	} else {
		return false
	}
}

// getEnvSettings grabs environment variables used to configure splunkpump from the running environment. During normal
// operations these should be defined in a docker file and passed into the container which is running splunkpump
func getEnvSettings() {

	splunkURL := os.Getenv("SPLUNK_URL")
	if len(splunkURL) > 0 {
		configStrings["splunkURL"] = splunkURL
	}
	splunkKey := os.Getenv("SPLUNK_KEY")
	if len(splunkKey) > 0 {
		configStrings["splunkKey"] = splunkKey
	}
}

func initConfigParams() {

	/*
	   NOTE: This code is only focussed on updating the splunkurl and splunkkey parameters from the environment variables
	   and or command line. This will be enhanced to support other configuration approaches.
	*/

	configStringsMu.Lock()
	splunkurl := flag.String("splunkurl", "", "URL of the splunk host")
	splunkkey := flag.String("splunkkey", "", "Splunk key")

	flag.Parse()

	//Gather configuration from environment variables
	// ie. ENV > CONFIG
	getEnvSettings()

	// Override everything with CLI options
	if *splunkurl != "" {
		configStrings["splunkURL"] = *splunkurl
	}
	if *splunkkey != "" {
		configStrings["splunkKey"] = *splunkkey
	}

	configStringsMu.Unlock()
}

func main() {

	// Initialze configuration variables.
	initConfigParams()

	// TODO: This is insecure; use only in dev environments.
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client = &http.Client{Transport: tr, Timeout: 0}

	// Send a http GET request repeatedly.
	var outerloop int

	for {

		req, err := http.NewRequest("GET", "https://10.239.37.136/redfish/v1/SSE?$filter=EventFormatType%20eq%20MetricReport", nil)
		if err != nil {
			fmt.Println("NewRequest GET", req, err)
		}
		fmt.Println("NewRequest GET", req, err)

		req.SetBasicAuth("root", "VMware1!")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Client.Do response", resp, err)
			os.Exit(0)
		}

		defer resp.Body.Close()

		var cnt int
		reportStart := false
		reportEnd := false
		reportChunks := 0
		outerloop++
		validHeader := 0

		byt := make([]byte, 8192*24, 8192*32)

		for {
			buf := make([]byte, 4096)

			n, err := resp.Body.Read(buf)

			log.Info("\n\nThe response size and response error\n\n", n, err)

			if n == 0 {
				log.Info("Got zero lenght when reading body - Don't break, continue for now", n, err)
				break
			}

			reportStart = checkHeader(buf)
			if reportStart {
				validHeader++
			}

			log.Info("The reportStart is", reportStart)
			log.Info("The Name starts at index", bytes.Index(buf, []byte("Name")))
			log.Info("The ReportSequence starts at index", bytes.Index(buf, []byte("ReportSequence")))
			log.Info("The iDRACFirmwareVersion  starts at index", bytes.Index(buf, []byte("iDracFirmwareVersion")))

			if reportStart {
				if validHeader > 1 {
					var index1 int
					log.Info("\n Detected a new header -- process it \n")

					if bytes.Contains(buf, []byte("id: ")) {
						index1 = bytes.Index(buf, []byte("id: "))
						log.Info("The index of the id: is at:", index1)
					}
					log.Info("***  GAP START ***")
					// Check the last 20 bytes of the byt buffer
					bytSize := len(byt)
					log.Info(string(byt[bytSize-20 : bytSize]))
					log.Info("***  GAP END  ***")

					// End the buffer and start a new one
					validHeader = 1
					byt = append(byt, buf[:(index1)]...)
					// Make a copy of the buffer and send it over to be processed.
					newMetricReport := byt
					log.Info("Last chunk and new header", string(buf))
					log.Info("New header detected", string(newMetricReport))
					go processValidBuffer(newMetricReport)
					// Update buffer to point to the next header
					if index1 > 2 {
						buf = buf[index1-2:]
						n = n - (index1 - 2)
					} else {
						buf = buf[index1:]
						n = n - index1
					}
					log.Info("The new size n: and new part buffer :", n, string(buf))
				}

				jsonOpenBraceAt := bytes.IndexByte(buf, byte('{'))

				// Move the data over to a temporary buffer
				byt = buf[jsonOpenBraceAt:n]
				reportChunks++
				if n > 15 {
					log.Info("The init buf start content", string(byt[:10]))
					log.Info("The init buf end content", string(buf[n-15:n]))
					log.Info("The init buf Real end content", string(buf[n-5:n]))
				}
			} else {
				// Append to the buffer
				byt = append(byt, buf[:n]...)
				if n > 15 {
					log.Info("The chunk buf content", string(buf[:10]))
					log.Info("The chunk buf end content", string(buf[n-15:n]))
					log.Info("The chunk buf Real end content", string(buf[n-5:n]))
				}
				reportChunks++
			}

			// Error check the length of the buffer and adjust TheEnd check accordingly.
			TheEnd := false
			if n > 10 {
				log.Info(string(buf))
				TheEnd = (bytes.Contains(buf[n-10:n], []byte(".00")) && bytes.Contains(buf[n-10:n], []byte("}}}")))
			}

			log.Debug(TheEnd, len(buf))

			if TheEnd {
				reportEnd = true
			}
			log.Debug("The reportEnd is", reportEnd)

			if reportEnd {

				// Move the buffer for post processing
				log.Infof("\nExtracted a Metric Report with %d chunks\n", reportChunks)
				// Make a copy of the buffer and send it over to be processed.
				metricReport := byt

				// Validate the buffer -- test
				bytSz := len(metricReport)
				log.Info("The metric report size in bytes", bytSz)
				log.Info("The metric report buf content start", string(byt[:10]))
				log.Info("The metric report buf content end", string(byt[bytSz-10:bytSz]))

				finBuf := string(metricReport)
				log.Debug("FinBuf", finBuf)
				reportChunks = 0
				reportEnd = false
				reportStart = false
				validHeader = 0

				go processValidBuffer(metricReport)

				time.Sleep(1 * time.Second)

				cnt++

				log.Debugf("\nThe outerloop is: %d The inner loop count is: %d \n", outerloop, cnt)

			}
		}
	}
}
