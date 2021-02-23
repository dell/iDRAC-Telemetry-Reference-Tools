// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/ini.v1"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/databus"
	//"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/amqp"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

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
			if strings.Contains(value.Context, ".") {
				doFQDDGuage(value, registry)
			} else {
				doNonFQDDGuage(value, registry)
			}
		}
	}
}

func main() {
	configName := flag.String("config", "config.ini", "The configuration ini file")

	flag.Parse()

	config, err := ini.Load(*configName)
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}

	//amqpHost := config.Section("General").Key("AmpqHost").MustString("0.0.0.0")
	//amqpPort := config.Section("General").Key("AmqpPort").MustInt(5672)
	stompHost := config.Section("General").Key("StompHost").MustString("0.0.0.0")
	stompPort := config.Section("General").Key("StompPort").MustInt(61613)

	//mb, err := amqp.NewAmqpMessageBus(amqpHost, amqpPort)
	mb, err := stomp.NewStompMessageBus(stompHost, stompPort)
	if err != nil {
		log.Fatal("Could not connect to message bus: ", err)
	}
	defer mb.Close()

	dbClient := new(databus.DataBusClient)
	dbClient.Bus = mb
	groupsIn := make(chan *databus.DataGroup, 10)
	dbClient.Subscribe("/prometheus")
	dbClient.Get("/prometheus")
	go dbClient.GetGroup(groupsIn, "/prometheus")

	registry := prometheus.NewRegistry()
	go handleGroups(groupsIn, registry)

	gatherer := prometheus.Gatherer(registry)
	http.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	err = http.ListenAndServe(":2112", nil)
	if err != nil {
		log.Printf("Failed to start webserver %v", err)
	}
}
