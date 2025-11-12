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
        "github.com/prometheus/client_golang/prometheus/promhttp"
        "github.com/prometheus/client_golang/prometheus"
        "github.com/prometheus/common/expfmt"

        "github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
        "github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
)

var configStrings = map[string]string{
        "mbhost":        "activemq",
        "mbport":        "61613",
        "victoria_url":  "",
        "victoria_user": "",
        "victoria_pass": "",
}

var collectors map[string]map[string]*prometheus.GaugeVec
var loggedMissingVictoria bool = false
// sanitizeMetricName replaces invalid Prometheus metric characters
func sanitizeMetricName(name string) string {
        replacer := strings.NewReplacer(
                ".", "_",
                "-", "_",
                " ", "_",
        )
        return replacer.Replace(name)
}

// parseFloatOrString converts numeric strings or certain status strings to float64
func parseFloatOrString(value string) float64 {
        if f, err := strconv.ParseFloat(value, 64); err == nil {
                return f
        }
        switch value {
        case "Up", "Operational":
                return 1
        case "Down", "Degraded", "Disabled":
                return 0
        default:
                return 0
        }
}

// createOrUpdateGauge handles both FQDD and non-FQDD metrics
func createOrUpdateGauge(value databus.DataValue, registry *prometheus.Registry) {
        isFQDD := strings.Contains(value.Context, ".")
        subsystem := ""
        labels := []string{"ServiceTag", "HostName"}
        contextKey := value.Context

        if isFQDD {
                labels = append(labels, "FQDD")
                contextKey = "FQDD"
        } else {
                value.Context = strings.ReplaceAll(value.Context, " ", "")
                subsystem = value.Context
        }

        if collectors[contextKey] == nil {
                collectors[contextKey] = make(map[string]*prometheus.GaugeVec)
        }

        metricName := sanitizeMetricName(value.ID)
        if isFQDD {
                metricName = sanitizeMetricName("PowerEdge_" + value.ID)
        }

        if collectors[contextKey][metricName] == nil {
                gauge := prometheus.NewGaugeVec(
                        prometheus.GaugeOpts{
                                Namespace: "PowerEdge",
                                Subsystem: subsystem,
                                Name:      metricName,
                        },
                        labels,
                )
                registry.MustRegister(gauge)
                collectors[contextKey][metricName] = gauge
        }

        gauge := collectors[contextKey][metricName]
        if isFQDD {
                gauge.WithLabelValues(value.System, value.HostName, value.Context).Set(parseFloatOrString(value.Value))
        } else {
                gauge.WithLabelValues(value.System, value.HostName).Set(parseFloatOrString(value.Value))
        }
}

// pushToVictoriaMetrics encodes and sends metrics to VictoriaMetrics
func pushToVictoriaMetrics(registry *prometheus.Registry) {
        if configStrings["victoria_url"] == "" {
                if !loggedMissingVictoria {
                        log.Printf("VictoriaMetrics URL not set, skipping push")
                        loggedMissingVictoria = true
                }
                return
        }
        var buf bytes.Buffer
        enc := expfmt.NewEncoder(&buf, expfmt.FmtText)
        mfs, err := registry.Gather()
        if err != nil {
                log.Printf("Failed to gather metrics: %v", err)
                return
        }
        for _, mf := range mfs {
                if err := enc.Encode(mf); err != nil {
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

// handleGroups processes telemetry groups and pushes them to VictoriaMetrics
func handleGroups(groupsChan chan *databus.DataGroup, registry *prometheus.Registry) {
        collectors = make(map[string]map[string]*prometheus.GaugeVec)
        for {
                group := <-groupsChan
                for _, value := range group.Values {
                        createOrUpdateGauge(value, registry)
                }
                pushToVictoriaMetrics(registry)
        }
}

// getEnvSettings loads environment variables for config
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
                        log.Printf("Connected to message bus at %s:%d", configStrings["mbhost"], stompPort)
                        break
                }

                if i == maxRetries {
                        log.Fatalf("Failed to connect to message bus after %d attempts", maxRetries)
                }
        }

        groupsIn := make(chan *databus.DataGroup, 10)
        dbClient.Subscribe("/prometheus")
        dbClient.Get("/prometheus")
        go dbClient.GetGroup(groupsIn, "/prometheus")

        registry := prometheus.NewRegistry()
        // Start HTTP server for /metrics scraping
        go func() {
            httpPort := 2112 // port for vmagent to scrape
            log.Printf("Starting HTTP server for /metrics on :%d", httpPort)
            http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
            if err := http.ListenAndServe("0.0.0.0:"+strconv.Itoa(httpPort), nil); err != nil {
                log.Fatalf("Failed to start /metrics HTTP server: %v", err)
            }
        }()

        handleGroups(groupsIn, registry)
}
