// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

//	"strconv"
//	"strings"

	"gopkg.in/ini.v1"
	"github.com/cenkalti/backoff/v4"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/elastic/go-elasticsearch/v8/esutil"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/databus"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

var (
	countSuccessful uint64
)	
	
func handleGroups(groupsChan chan *databus.DataGroup, 
						bi esutil.BulkIndexer) {
	for {
		group := <-groupsChan
//		log.Print("group: ", group)
		for _, value := range group.Values {
			log.Print("value: ", value)

			data, err := json.Marshal(value)
			if err != nil {
				log.Fatalf("Cannot encode metric:  %s - %s", value.ID, value.Context)
			}
	
			// Add an item to the BulkIndexer
			err = bi.Add(
				context.Background(),
				esutil.BulkIndexerItem{
					// Action field configures the operation to perform (index, create, delete, update)
					Action: "index",
	
					// DocumentID is the (optional) document ID
					DocumentID: value.ID,
	
					// Body is an `io.Reader` with the payload
					Body: bytes.NewReader(data),

					// OnSuccess is called for each successful operation
					OnSuccess: func(ctx context.Context, item esutil.BulkIndexerItem, res esutil.BulkIndexerResponseItem) {
						atomic.AddUint64(&countSuccessful, 1)
					},

					// OnFailure is called for each failed operation
					OnFailure: func(ctx context.Context, item esutil.BulkIndexerItem, res esutil.BulkIndexerResponseItem, err error) {
						if err != nil {
							log.Printf("ERROR: %s", err)
						} else {
							log.Printf("ERROR: %s: %s", res.Error.Type, res.Error.Reason)
						}
					},
				},
			)
			if err != nil {
				log.Fatalf("Unexpected error: %s", err)
			}
		}
	}
}

func main() {
	var (
//countSuccessful uint64

		res *esapi.Response
		err error
	)

	configName := flag.String("config", "config.ini", "The configuration ini file")


	flag.Parse()

	config, err := ini.Load(*configName)
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}

	stompHost := config.Section("General").Key("StompHost").MustString("0.0.0.0")
	stompPort := config.Section("General").Key("StompPort").MustInt(61613)

	mb, err := stomp.NewStompMessageBus(stompHost, stompPort)
	if err != nil {
		log.Fatal("Could not connect to message bus: ", err)
	}
	defer mb.Close()

	dbClient := new(databus.DataBusClient)
	dbClient.Bus = mb
	groupsIn := make(chan *databus.DataGroup, 10)
	dbClient.Subscribe("/elkstack")
	dbClient.Get("/elkstack")
	go dbClient.GetGroup(groupsIn, "/elkstack")

	//Initialize elasticsearch client
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
		log.Fatalf("Error creating the elasticsearch client: %s", err)
	}

	indexName := "idrac_telemetry_metrics"
	// Create the BulkIndexer
	bi, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:         indexName,        // The default index name
		Client:        es,               // The Elasticsearch client
		NumWorkers:    runtime.NumCPU(), // The number of worker goroutines
		FlushBytes:    10000,           // The flush threshold in bytes
		FlushInterval: 1 * time.Second,  // The periodic flush interval
	})
	if err != nil {
		log.Fatalf("Error creating the indexer: %s", err)
	}

	// Re-create the index
	//
	if res, err = es.Indices.Delete([]string{indexName}, es.Indices.Delete.WithIgnoreUnavailable(true)); err != nil || res.IsError() {
		log.Fatalf("Cannot delete index: %s", err)
	}
	res.Body.Close()
	res, err = es.Indices.Create(indexName)
	if err != nil {
		log.Fatalf("Cannot create index: %s", err)
	}
	if res.IsError() {
		log.Fatalf("Cannot create index: %s", res)
	}
	res.Body.Close()


	go handleGroups(groupsIn, bi)

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Printf("Failed to start webserver %v", err)
	}

	// Close the indexer
	if err := bi.Close(context.Background()); err != nil {
		log.Fatalf("Unexpected error: %s", err)
	}

}
