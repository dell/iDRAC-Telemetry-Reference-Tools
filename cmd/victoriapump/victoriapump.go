// Licensed to You under the Apache License, Version 2.0.

package main

import (
        "bytes"
        "log"
        "net/http"
        "os"
        "strconv"
        "strings"
        "time"

        "github.com/prometheus/client_golang/prometheus"
        "github.com/prometheus/common/expfmt"

        "github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
        "github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
)

var configStrings = map[string]string{
        "mbhost":        "activemq",
        "mbport":        "61613",
        "victoria_url":  "http://localhost:8428/api/v1/import/prometheus",
        "victoria_user": "",
        "victoria_pass": "",
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
                                "HostName",
                                "FQDD",
                        })
                registry.MustRegister(guage)
                floatVal, err := strconv.ParseFloat(value.Value, 64)
                if err != nil {
                        if value.Value == "Up" || value.Value == "Operational" {
                                floatVal = 1
                        }
                }
                guage.WithLabelValues(value.System, value.HostName, value.Context).Set(floatVal)
                collectors["FQDD"][value.ID] = guage
        } else {
                guage := collectors["FQDD"][value.ID]
                floatVal, err := strconv.ParseFloat(value.Value, 64)
                if err != nil {
                        if value.Value == "Up" || value.Value == "Operational" {
                                floatVal = 1
                        }
                }
                guage.WithLabelValues(value.System, value.HostName, value.Context).Set(floatVal)
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
                                "HostName",
                        })
                registry.MustRegister(guage)
                floatVal, _ := strconv.ParseFloat(value.Value, 64)
                guage.WithLabelValues(value.System, value.HostName).Set(floatVal)
                collectors[value.Context][value.ID] = guage
        } else {
                guage := collectors[value.Context][value.ID]
                floatVal, _ := strconv.ParseFloat(value.Value, 64)
                guage.WithLabelValues(value.System, value.HostName).Set(floatVal)
        }
}

func pushToVictoriaMetrics(registry *prometheus.Registry) {
        var buf bytes.Buffer
        enc := expfmt.NewEncoder(&buf, expfmt.FmtText)
        mfs, err := registry.Gather()
        if err != nil {
                log.Printf("Failed to gather metrics: %v", err)
                return
        }
        for _, mf := range mfs {
                err := enc.Encode(mf)
                if err != nil {
                        log.Printf("Failed to encode metric: %v", err)
                }
        }

        req, err := http.NewRequest("POST", configStrings["victoria_url"], &buf)
        if err != nil {
                log.Printf("Failed to create request: %v", err)
                return
        }
        req.Header.Set("Content-Type", string(expfmt.FmtText))
        if configStrings["victoria_user"] != "" && configStrings["victoria_pass"] != "" {
                req.SetBasicAuth(configStrings["victoria_user"], configStrings["victoria_pass"])
        }

        resp, err := http.DefaultClient.Do(req)
        if err != nil {
                log.Printf("Failed to send metrics: %v", err)
                return
        }
        defer resp.Body.Close()
        if resp.StatusCode >= 300 {
                log.Printf("VictoriaMetrics responded with status: %s", resp.Status)
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
                pushToVictoriaMetrics(registry)
        }
}

func getEnvSettings() {
        if val := os.Getenv("MESSAGEBUS_HOST"); val != "" {
                configStrings["mbhost"] = val
        }
        if val := os.Getenv("MESSAGEBUS_PORT"); val != "" {
                configStrings["mbport"] = val
        }
        if val := os.Getenv("VICTORIA_METRICS_URL"); val != "" {
                configStrings["victoria_url"] = val
        }
        if val := os.Getenv("VICTORIA_USERNAME"); val != "" {
                configStrings["victoria_user"] = val
        }
        if val := os.Getenv("VICTORIA_PASSWORD"); val != "" {
                configStrings["victoria_pass"] = val
        }
}

func main() {
        getEnvSettings()

        dbClient := new(databus.DataBusClient)
        stompPort, _ := strconv.Atoi(configStrings["mbport"])
        maxRetries := 20

        for i := 1; i <= maxRetries; i++ {
                log.Printf("Attempt %d: Connecting to message bus at %s:%d", i, configStrings["mbhost"], stompPort)
                mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
                if err != nil {
                        log.Printf("Connection failed: %s", err)
                        time.Sleep(5 * time.Second)
                } else {
                        dbClient.Bus = mb
                        defer mb.Close()
                        log.Printf("✅ Connected to message bus at %s:%d", configStrings["mbhost"], stompPort)
                        break
                }

                if i == maxRetries {
                        log.Fatalf("❌ Failed to connect to message bus after %d attempts", maxRetries)
                }
        }

        groupsIn := make(chan *databus.DataGroup, 10)
        dbClient.Subscribe("/prometheus")
        dbClient.Get("/prometheus")
        go dbClient.GetGroup(groupsIn, "/prometheus")

        registry := prometheus.NewRegistry()
        handleGroups(groupsIn, registry)
}


