// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	influx "github.com/influxdata/influxdb1-client/v2"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/databus"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

var configStrings = map[string]string{
	"mbhost":       "activemq",
	"mbport":       "61613",
	"influxDBHost": "http://localhost:8086",
	"influxDBName": "poweredge_telemetry_metrics",
}

var db influx.Client

func createDB() {
	q := influx.Query{
		Command:  fmt.Sprintf("create database %s", configStrings["influxDBName"]),
		Database: configStrings["influxDBName"],
	}

	_, err := db.Query(q)
	if err != nil {
		log.Print("Error creating database: ", err)
	}
}

func handleGroups(groupsChan chan *databus.DataGroup) {
	for {
		group := <-groupsChan
		var points = make([]*influx.Point, len(group.Values))
		for index, value := range group.Values {
			floatVal, _ := strconv.ParseFloat(value.Value, 64)

			fields := make(map[string]interface{})
			fields["value"] = floatVal

			timestamp, err := time.Parse(time.RFC3339, value.Timestamp)
			if err != nil {
				log.Printf("Error parsing timestamp for  point %s: (%s) %v", value.Context+"_"+value.ID, value.Timestamp, err)
			}
			//log.Printf("%s: %s\n", value.Context+"_"+value.ID, value.Timestamp)
			if strings.Contains(value.Context, ".") {
				points[index], err = influx.NewPoint(value.ID, map[string]string{"ServiceTag": value.System, "FQDD": value.Context}, fields, timestamp)
			} else {
				points[index], err = influx.NewPoint(value.Context+"_"+value.ID, map[string]string{"ServiceTag": value.System}, fields, timestamp)
			}
			if err != nil {
				log.Printf("Error creating point %s: %v", value.Context+"_"+value.ID, err)
			}
		}
		bps, err := influx.NewBatchPoints(influx.BatchPointsConfig{Database: configStrings["influxDBName"]})
		if err != nil {
			log.Print("Error creating batch points: ", err)
			continue
		}
		bps.AddPoints(points)
		err = db.Write(bps)
		if err != nil {
			log.Print("Error logging to influx: ", err)
		}
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
	configStrings["influxDBHost"] = os.Getenv("INFLUXDB_SERVER")
	configStrings["influxDBName"] = os.Getenv("INFLUXDB_DB")

}

func main() {
	var err error

	//Gather configuration from environment variables
	getEnvSettings()

	dbClient := new(databus.DataBusClient)
	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: ", err)
			time.Sleep(5 * time.Second)
		} else {
			dbClient.Bus = mb
			defer mb.Close()
			break
		}
	}

	groupsIn := make(chan *databus.DataGroup, 10)
	dbClient.Subscribe("/influx")
	dbClient.Get("/influx")
	go dbClient.GetGroup(groupsIn, "/influx")

	time.Sleep(5 * time.Second)
	db, err = influx.NewHTTPClient(influx.HTTPConfig{
		Addr: fmt.Sprintf("http://%s:8086", configStrings["influxDBHost"]),
	})
	if err != nil {
		log.Println("Cannot connect to influx: ", err)

	} else {
		defer db.Close()
		createDB()

		handleGroups(groupsIn)
	}
}
