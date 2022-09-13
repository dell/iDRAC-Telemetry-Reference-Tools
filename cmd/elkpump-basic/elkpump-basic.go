// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"

	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
)

var configStrings = map[string]string{
	"mbhost": "activemq",
	"mbport": "61613",
}

type DataValueElasticSearch struct {
	ID                string
	Context           string
	Label             string
	Value             string
	System            string
	Timestamp         string
	ValueAggregatable float64
}

func handleGroups(groupsChan chan *databus.DataGroup,
	es *elasticsearch.Client, indexName string) {

	type bulkResponse struct {
		Errors bool `json:"errors"`
		Items  []struct {
			Index struct {
				ID     string `json:"_id"`
				Result string `json:"result"`
				Status int    `json:"status"`
				Error  struct {
					Type   string `json:"type"`
					Reason string `json:"reason"`
					Cause  struct {
						Type   string `json:"type"`
						Reason string `json:"reason"`
					} `json:"caused_by"`
				} `json:"error"`
			} `json:"index"`
		} `json:"items"`
	}

	var (
		buf bytes.Buffer
		res *esapi.Response
		err error
		raw map[string]interface{}
		blk *bulkResponse

		numErrors  int
		numIndexed int
	)

	for {
		group := <-groupsChan
		//		log.Print("group: ", group)
		for _, value := range group.Values {
			log.Print("value: ", value)

			// Prepare the metadata payload
			meta := []byte(fmt.Sprintf(`{ "index" : { "_id" : "%s-%s" } }%s`, value.ID, value.Timestamp, "\n"))

			// Prepare the data payload: encode article to JSON
			if len(value.Value) == 0 {
				continue
			}
			intVal, intErr := strconv.ParseInt(value.Value, 10, 64)
			floatVal, floatErr := strconv.ParseFloat(value.Value, 64)
			esvalue := DataValueElasticSearch{value.ID, value.Context, value.Label, value.Value, value.System, value.Timestamp, 0}
			switch {
			case intErr == nil:
				esvalue.ValueAggregatable = float64(intVal)
			case floatErr == nil && !math.IsNaN(floatVal):
				esvalue.ValueAggregatable = floatVal
			}

			data, err := json.Marshal(esvalue)
			if err != nil {
				log.Fatalf("Cannot encode article %s: %s", value.ID, err)
			}

			// Append newline to the data payload
			data = append(data, "\n"...) // <-- Comment out to trigger failure for batch

			// Append payloads to the buffer (ignoring write errors)
			buf.Grow(len(meta) + len(data))
			buf.Write(meta)
			buf.Write(data)
		}

		res, err = es.Bulk(bytes.NewReader(buf.Bytes()), es.Bulk.WithIndex(indexName))
		if err != nil {
			log.Fatalf("Failure indexing batch : %s", err)
		}
		if res.IsError() {
			if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
				log.Fatalf("Failure to to parse response body: %s", err)
			} else {
				log.Printf("  Error: [%d] %s: %s",
					res.StatusCode,
					raw["error"].(map[string]interface{})["type"],
					raw["error"].(map[string]interface{})["reason"],
				)
			}
			// A successful response might still contain errors for particular documents...
			//
		} else {
			if err := json.NewDecoder(res.Body).Decode(&blk); err != nil {
				log.Fatalf("Failure to to parse response body: %s", err)
			} else {
				for _, d := range blk.Items {
					//so for any HTTP status above 201 ...
					if d.Index.Status > 201 {
						//increment the error counter ...
						numErrors++

						//and print the response status and error information ...
						log.Printf("  Error: [%d]: %s: %s: %s: %s",
							d.Index.Status,
							d.Index.Error.Type,
							d.Index.Error.Reason,
							d.Index.Error.Cause.Type,
							d.Index.Error.Cause.Reason,
						)
					} else {
						//otherwise increase the success counter.
						numIndexed++
					}
				}
			}
		}

		res.Body.Close()
		buf.Reset()
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
	var (
		res *esapi.Response
		err error
	)
	/*
		mapping := `{
		  "mappings": {
			"properties" : {
			  "Context"	: {"type" : "keyword"},
			  "ID" 		: {"type" : "keyword"},
			  "Label" 	: {
					"type" : "text",
					"fields" : {
					  "keyword" : {
						"type" : "keyword",
						"ignore_above" : 256
					  }
					}
				  },
			  "System" : {"type" : "keyword"},
			  "Timestamp" : {"type" : "date"},
			  "Value" : {"type" : "text"}
			}
			}
		  }`*/

	//Gather configuration from environment variables
	getEnvSettings()

	dbClient := new(databus.DataBusClient)
	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			log.Printf("Connected to message bus" )
			dbClient.Bus = mb
			defer mb.Close()
			break
		}
	}

	groupsIn := make(chan *databus.DataGroup, 10)
	dbClient.Subscribe("/elkstack")
	dbClient.Get("/elkstack")
	go dbClient.GetGroup(groupsIn, "/elkstack")

	//Initialize elasticsearch client
	time.Sleep(15 * time.Second)
	retryBackoff := backoff.NewExponentialBackOff()
	es, err := elasticsearch.NewClient(elasticsearch.Config{
		RetryOnStatus: []int{502, 503, 504, 429},

		// Configure the backoff function
		RetryBackoff: func(i int) time.Duration {
			if i == 1 {
				retryBackoff.Reset()
			}
			return retryBackoff.NextBackOff()
		},

		// Retry up to 5 attempts
		MaxRetries: 5,
	})

	if err != nil {
		log.Fatalf("Error creating the client: %s", err)
	}

	indexName := "poweredge_telemetry_metrics"
	isSuccess := false 
	// wait for elastic search server to come up
	for i:= 0; i< 10; i++{
	    res, err = es.Indices.Get([]string{indexName})
	    if err == nil{
	        res.Body.Close()
		isSuccess = true
	    	break
	    }
	    time.Sleep(30 * time.Second)
	}
	
	if !isSuccess{
		log.Fatalf("ELK SErver is not up after 300 seconds")
	}
	log.Printf("ELK Server is up")
	log.Printf("GET successful %s: %v", indexName,res)

	// Re-create the index
	res, err = es.Indices.Delete([]string{indexName})
	if res, err = es.Indices.Delete([]string{indexName}); err != nil {
		log.Fatalf("Cannot delete index:%s %s", indexName, err)
	}
	res.Body.Close()

	//	res, err = es.Indices.Create(indexName,
	//			   es.Indices.Create.WithBody(strings.NewReader(mapping)))
	res, err = es.Indices.Create(indexName)
	if err != nil {
		log.Fatalf("Cannot create index: %s", err)
	}
	if res.IsError() {
		log.Fatalf("Cannot create index: %s", res)
	}
	res.Body.Close()

	go handleGroups(groupsIn, es, indexName)

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Printf("Failed to start webserver %v", err)
	}

}
