// Licensed to You under the Apache License, Version 2.0.

// DMTF Redfish Telemetry simulator in Golang.
//     $ go run source-simulator.go
//	   $ client side command
//        - curl -s -k -u testuser:testpassword -X GET http://127.0.0.1:8080/redfish/v1/SSE?$filter=EventFormatType%20eq%20MetricReport
//        - curl -s -k -u testuser:testpassword -X GET http://127.0.0.1:8080/redfish/v1/SSE

// TODO - support TLS/https

package main

import (
	"io"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	mr "gitlab.pgre.dell.com/enterprise/telemetryservice/utilities/source-simulator/metricreport"
	br "gitlab.pgre.dell.com/enterprise/telemetryservice/utilities/source-simulator/ssebroker"
)

var configStrings = map[string]string{
	"user":     "testuser",
	"password": "testpassword",
}

func getEnvSettings() {
	username := os.Getenv("SIMULATOR_USERNAME")
	if len(username) > 0 {
		configStrings["user"] = username
	}
	userpassword := os.Getenv("SIMULATOR_USERPASSWORD")
	if len(userpassword) > 0 {
		configStrings["password"] = userpassword
	}
}

func main() {
	//Gather configuration from environment variables
	getEnvSettings()

	router := gin.Default()

	// Add event-streaming headers
	router.Use(br.HeadersMiddleware())

	// Initialize new streaming server
	broker := br.NewSSEBroker()
	router.Use(broker.ServeHTTP())

	// Basic Authentication
	authorized := router.Group("/", gin.BasicAuth(gin.Accounts{
		configStrings["user"]: configStrings["password"],
	}))

	// Authorized client can stream the event
	authorized.GET("/redfish/v1/SSE", func(c *gin.Context) {
		// We are streaming current time to clients in the interval 10 seconds
		go func() {
			for {
				time.Sleep(time.Second * 10)
				// Send Report to clients message channel
				broker.Message <- mr.GetMetricReport()
			}
		}()

		c.Stream(func(w io.Writer) bool {
			// Stream message to client from message channel
			if msg, ok := <-broker.Message; ok {
				c.SSEvent("MetricReport", msg)
				return true
			}
			return false
		})
	})

	log.Print("Starting to listen on port 8080...")
	router.Run(":8080")
}
