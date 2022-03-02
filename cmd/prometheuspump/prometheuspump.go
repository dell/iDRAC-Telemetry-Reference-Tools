// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/databus"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

var configStrings = map[string]string{
	"mbhost": "activemq",
	"mbport": "61613",
}

var collectors map[string]map[string]*prometheus.GaugeVec

func doFQDDGuage(value databus.DataValue, registry *prometheus.Registry) {
	if collectors["FQDD"] == nil {
		collectors["FQDD"] = make(map[string]*prometheus.GaugeVec)
	}
	if collectors["FQDD"][value.ID] == nil {
		guage := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "PowerEdge",
				Name:      value.ID,
			},
			[]string{
				"ServiceTag",
				"FQDD",
			})
		registry.MustRegister(guage)
		floatVal, err := strconv.ParseFloat(value.Value, 64)
		if err != nil {
			if value.Value == "Up" || value.Value == "Operational" {
				floatVal = 1
			}
		}
		guage.WithLabelValues(value.System, value.Context).Set(floatVal)
		collectors["FQDD"][value.ID] = guage
	} else {
		guage := collectors["FQDD"][value.ID]
		floatVal, err := strconv.ParseFloat(value.Value, 64)
		if err != nil {
			if value.Value == "Up" || value.Value == "Operational" {
				floatVal = 1
			}
		}
		guage.WithLabelValues(value.System, value.Context).Set(floatVal)
	}
}

func doNonFQDDGuage(value databus.DataValue, registry *prometheus.Registry) {
	value.Context = strings.Replace(value.Context, " ", "", -1)
	if collectors[value.Context] == nil {
		collectors[value.Context] = make(map[string]*prometheus.GaugeVec)
	}
	if collectors[value.Context][value.ID] == nil {
		guage := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "PowerEdge",
				Subsystem: value.Context,
				Name:      value.ID,
			},
			[]string{
				"ServiceTag",
			})
		registry.MustRegister(guage)
		floatVal, _ := strconv.ParseFloat(value.Value, 64)
		guage.WithLabelValues(value.System).Set(floatVal)
		collectors[value.Context][value.ID] = guage
	} else {
		guage := collectors[value.Context][value.ID]
		floatVal, _ := strconv.ParseFloat(value.Value, 64)
		guage.WithLabelValues(value.System).Set(floatVal)
	}
}

func handleGroups(groupsChan chan *databus.DataGroup, registry *prometheus.Registry) {
	collectors = make(map[string]map[string]*prometheus.GaugeVec)
	for {
		group := <-groupsChan
		for _, value := range group.Values {
			log.Print("value: ", value)
			if strings.Contains(value.Context, ".") {
				doFQDDGuage(value, registry)
			} else {
				doNonFQDDGuage(value, registry)
			}
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
}

func main() {

	//Gather configuration from environment variables
	getEnvSettings()

	dbClient := new(databus.DataBusClient)
	//Initialize messagebus
	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			dbClient.Bus = mb
			defer mb.Close()
			break
		}
	}

	groupsIn := make(chan *databus.DataGroup, 10)
	dbClient.Subscribe("/prometheus")
	dbClient.Get("/prometheus")
	go dbClient.GetGroup(groupsIn, "/prometheus")

	registry := prometheus.NewRegistry()
	go handleGroups(groupsIn, registry)

	gatherer := prometheus.Gatherer(registry)
	http.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	err := http.ListenAndServe(":2112", nil)
	if err != nil {
		log.Printf("Failed to start webserver %v", err)
	}
}
