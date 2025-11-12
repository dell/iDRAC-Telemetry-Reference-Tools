package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/config"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
	"github.com/spf13/viper"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
)

const (
	joinChar = ":"
)

// Intermediate struct to hold metric data points before conversion to OTLP
type dp struct {
	value any // float64 or string (for enums)
	time  int64
	attr  []*commonv1.KeyValue
}

type otelMeta struct {
	name        string
	description string
	unit        string
	valueType   interface{}
	attributes  map[string]string
	Time        int64
	Value       interface{}
	MetricName  string
	enum        map[string]interface{}
}

var idrac2Otel = map[string]otelMeta{}

var configStringsMu sync.RWMutex
var configStrings = map[string]string{
	"mbhost":         "activemq",
	"mbport":         "61613",
	"otelCollector":  "",
	"otelCACert":     "",
	"otelClientCert": "",
	"otelClientKey":  "",
	"otelSkipVerify": "",
}

var configItems = map[string]*config.ConfigEntry{
	"otelCollector": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"otelCACert": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"otelClientCert": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"otelClientKey": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"otelSkipVerify": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
}

// configSet updates a configuration property in the in‑memory config map.
//
// Parameters:
//
//	name string - the name of the configuration property to set.
//	value interface{} - the new value for the property (must be a string).
//
// Returns:
//
//	error - nil on success, or an error if the property name is unknown.
func configSet(name string, value interface{}) error {
	configStringsMu.Lock()
	defer configStringsMu.Unlock()
	switch name {
	case "otelCollector", "otelCACert", "otelClientCert", "otelClientKey", "otelSkipVerify":
		configStrings[name] = value.(string)
	default:
		return fmt.Errorf("unknown property %s", name)
	}
	return nil
}

// configGet retrieves a configuration property's current value from the in‑memory config map.
//
// Parameters:
//
//	name string - the name of the configuration property to get.
//
// Returns:
//
//	(interface{}, error) - the property value as an interface{} and nil on success, or an error if the property name is unknown.
func configGet(name string) (interface{}, error) {
	switch name {
	case "otelCollector", "otelCACert", "otelClientCert", "otelClientKey", "otelSkipVerify":
		configStringsMu.RLock()
		ret := configStrings[name]
		configStringsMu.RUnlock()
		return ret, nil
	default:
		return nil, fmt.Errorf("unknown property %s", name)
	}
}

type rf2Otel struct {
	ScopeAttr map[string]string
	Metric    []map[string]interface{}
}

// getEnvSettings grabs environment variables used to configure otelpump from the running environment. During normal
// operations these should be defined in a docker file and passed into the container which is running otelpump
func getEnvSettings() {
	// already locked on entrance
	mbHost := os.Getenv("MESSAGEBUS_HOST")
	if len(mbHost) > 0 {
		configStrings["mbhost"] = mbHost
	}
	mbPort := os.Getenv("MESSAGEBUS_PORT")
	if len(mbPort) > 0 {
		configStrings["mbport"] = mbPort
	}
	otelCollector := os.Getenv("OTEL_COLLECTOR")
	if len(otelCollector) > 0 {
		configStrings["otelCollector"] = otelCollector
	}

	otelCert := os.Getenv("OTEL_CACERT")
	if len(otelCert) > 0 {
		configStrings["otelCACert"] = otelCert
	}
	otelClientCert := os.Getenv("OTEL_CLIENT_CERT")
	if len(otelClientCert) > 0 {
		configStrings["otelClientCert"] = otelClientCert
	}
	otelClientKey := os.Getenv("OTEL_CLIENT_KEY")
	if len(otelClientKey) > 0 {
		configStrings["otelClientKey"] = otelClientKey
	}
	otelSkipVerify := os.Getenv("OTEL_SKIP_VERIFY")
	if len(otelSkipVerify) > 0 {
		configStrings["otelSkipVerify"] = otelSkipVerify
	}

}

func containsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

// readOtelMeta loads the redfish‑to‑OTel mapping configuration from a YAML file and populates internal structures used for metric conversion.
//
// Parameters:
//
//	configFile string - path to the YAML configuration file.
//
// Returns:
//
//	(none) - exits the program on fatal errors; otherwise populates global variables.
func readOtelMeta(configFile string) {
	slog.Info("readOtelMeta: using config file", "configFile", configFile)
	cfg := viper.New()
	cfg.SetConfigFile(configFile)
	err := cfg.ReadInConfig()
	if err != nil {
		slog.Error("error reading redfishToOtel.yaml", "error", err)
		os.Exit(-1)
	}
	var repeatedMetricIds []string
	if cfg.IsSet("repeatedMetricIds") {
		err = cfg.UnmarshalKey("repeatedMetricIds", &repeatedMetricIds)
		if err != nil {
			slog.Error("error unmarshalling repeatedMetricIds", "error", err)
			os.Exit(-1)
		}
	}

	subcfg := cfg.Sub("ScopeAttrDefault")
	if subcfg == nil {
		slog.Error("ScopeAttrDefault not found in redfishToOtel.yaml ")
		os.Exit(-1)
	}
	var scopeDef = map[string]string{}
	err = subcfg.Unmarshal(&scopeDef)
	if err != nil {
		slog.Error("Unmarshal error", "error", err)
		os.Exit(-1)
	}
	// replace _ with . in keys
	for sa, v := range scopeDef {
		if strings.Contains(sa, "_") {
			delete(scopeDef, sa)
			scopeDef[strings.Replace(sa, "_", ".", -1)] = v
		}
	}

	subcfg = cfg.Sub("MetricReport")
	if subcfg == nil {
		slog.Error("MetricReport not found in redfishToOtel.yaml ")
		os.Exit(-1)
	}

	for k := range subcfg.AllSettings() {
		metricReport := k
		//logTrace(DEBUG, " Process metricReport ", metricReport)
		subcfg2 := subcfg.Sub(k)
		if subcfg2 == nil {
			slog.Warn("nil subcfg for section", "section", k)
			continue
		}

		var r2o rf2Otel
		err := subcfg2.Unmarshal(&r2o)
		if err != nil {
			slog.Warn("Unmarshal error", "error", err)
			continue
		}

		for sa, v := range r2o.ScopeAttr {
			if strings.Contains(sa, "_") {
				delete(r2o.ScopeAttr, sa)
				r2o.ScopeAttr[strings.Replace(sa, "_", ".", -1)] = v
			}
		}
		slog.Debug("r2o: scopeattr", "scopeattr", r2o.ScopeAttr)
		slog.Debug("r2o: Metric ", "Metric", r2o.Metric)

		for _, m := range r2o.Metric {
			var om otelMeta
			onm, ok := m["otelmetricname"].(string)
			if !ok {
				slog.Warn("otelMetricName not found!")
				continue
			}
			om.name = onm
			otyp, ok := m["oteltype"].(string)
			if !ok {
				slog.Warn("otelType not found!")
				continue
			}
			om.valueType = otyp
			ou, ok := m["unit"].(string)
			if ok {
				om.unit = ou
			}
			od, ok := m["description"].(string)
			if ok {
				om.description = od
			}
			rnm, ok := m["redfishname"].(string)
			if !ok {
				slog.Warn("redfishName not found!")
				continue
			}
			om.attributes = make(map[string]string)
			// default scope
			for k, v := range scopeDef {
				om.attributes[k] = v
			}
			// override local scope
			for k, v := range r2o.ScopeAttr {
				om.attributes[k] = v
			}

			// m["attr"] , append to scope attributes / replace specific attr .e.g hw.type
			am, ok := m["attr"].(map[string]interface{})
			if ok {
				for k, v := range am {
					om.attributes[k] = v.(string)
				}
			}

			// Handle repeatedMetricIds by prefixing with metric report
			if containsString(repeatedMetricIds, rnm) {
				rnm = metricReport + joinChar + rnm
			}

			// Add the enum map to the meta if it exists
			if enum, ok := m["enum"]; ok {
				om.enum = enum.(map[string]interface{})
			}
			idrac2Otel[rnm] = om
			// logTrace(DEBUG, "otelMeta ", om)
		}
	}
}

// parseFloat attempts to parse a string as a float64.
//
// Parameters:
//
//	value string - the string to convert.
//
// Returns:
//
//	(float64, bool) - the parsed float and true on success, or 0 and false on failure.
func parseFloat(value string) (float64, bool) {
	val, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

// parseRFC3339ToNanos parses an RFC3339 timestamp string and returns the Unix nanosecond representation.
//
// Parameters:
//
//	s string - timestamp in RFC3339 format.
//
// Returns:
//
//	(int64, error) - nanoseconds since epoch and an error if parsing fails.
func parseRFC3339ToNanos(s string) (int64, error) {
	if strings.TrimSpace(s) == "" {
		return 0, errors.New("empty timestamp")
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, fmt.Errorf("parse timestamp %q: %w", s, err)
	}
	return t.UnixNano(), nil
}

// kv creates a OpenTelemetry KeyValue attribute from a string key and string value.
//
// Parameters:
//
//	k string - attribute key.
//	v string - attribute value.
//
// Returns:
//
//	*commonv1.KeyValue - pointer to the constructed KeyValue protobuf message.
func kv(k, v string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key:   k,
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v}},
	}
}

// getHostAttributes builds a slice of host‑level attributes for a DataGroup, used in OTLP resources.
//
// Parameters:
//
//	group *databus.DataGroup - the data group containing host information.
//
// Returns:
//
//	[]*commonv1.KeyValue - slice of KeyValue attributes describing the host.
func getHostAttributes(group *databus.DataGroup) []*commonv1.KeyValue {
	return []*commonv1.KeyValue{
		kv("host.type", "PowerEdge"),
		kv("host.id", group.SKU),
		kv("host.name", group.FQDN),
		kv("host.model", group.Model),
	}
}

// addScopeAttributes populates the instrumentation scope attributes for a metric based on the otelMeta definition, FQDD, and timestamp.
//
// Parameters:
//
//	otelM otelMeta - metadata describing the metric.
//	fqdd string - the Fully Qualified Device Descriptor identifying the source.
//	timestamp string - string representation of the report timestamp.
//	scope *commonv1.InstrumentationScope - the scope object to which attributes will be added.
//
// Returns:
//
//	(none) - modifies the provided scope in place.
func addScopeAttributes(otelM otelMeta, fqdd, timestamp string, scope *commonv1.InstrumentationScope) {
	scope.Attributes = []*commonv1.KeyValue{}
	for attr, val := range otelM.attributes {
		switch val {
		case "var-FQDD":
			val = fqdd
		case "var-Timestamp":
			val = timestamp
		default:
			// use the value as is
		}
		scope.Attributes = append(scope.Attributes, kv(attr, val))
	}
}

// groupMetricsByFQDDAndMetricId groups metric data points from a
// DataGroup by their FQDD and metric ID.
//
// Parameters:
//
//	group *databus.DataGroup - the input data group containing metric values.
//
// Returns:
//
//	map[string]map[string][]dp - a nested map keyed first by FQDD then by metric ID, each containing a slice of data points.
func groupMetricsByFQDDAndMetricId(group *databus.DataGroup) map[string]map[string][]dp {

	metricsByFQDDMetricId := map[string]map[string][]dp{}
	for _, value := range group.Values {
		var val any
		val, ok := parseFloat(value.Value)
		if !ok {
			val = value.Value // keep original string value for enum conversion
		}
		nanos, err := parseRFC3339ToNanos(value.Timestamp)
		if err != nil {
			slog.Warn("skip record with bad timestamp", "timestamp", value.Timestamp, "error", err)
			continue
		}
		if _, ok := metricsByFQDDMetricId[value.Context]; !ok {
			metricsByFQDDMetricId[value.Context] = map[string][]dp{}
		}

		if _, ok := metricsByFQDDMetricId[value.Context][value.ID]; !ok {
			metricsByFQDDMetricId[value.Context][value.ID] = []dp{}
		}
		metricsByFQDDMetricId[value.Context][value.ID] = append(metricsByFQDDMetricId[value.Context][value.ID], dp{value: val, time: nanos})
	}
	return metricsByFQDDMetricId
}

// toOTLPMetrics converts a DataGroup containing metric values into an
// OTLP ResourceMetrics protobuf message.
//
// Parameters:
//
//	group *databus.DataGroup - the data group with metric information.
//
// Returns:
//
//	(*metricsv1.ResourceMetrics, error) - the constructed ResourceMetrics and any error encountered during conversion.
func toOTLPMetrics(group *databus.DataGroup) (*metricsv1.ResourceMetrics, error) {
	metricsByFQDDMetricId := groupMetricsByFQDDAndMetricId(group)
	// Top level object for every group. Here a DataGroup is a MetricReport
	rm := &metricsv1.ResourceMetrics{
		Resource: &resourcev1.Resource{
			Attributes: getHostAttributes(group),
		},
	}

	for fqdd, metricsByMetricId := range metricsByFQDDMetricId {
		metrics := make([]*metricsv1.Metric, 0, len(metricsByMetricId))
		scope := &commonv1.InstrumentationScope{}
		for metricId, dps := range metricsByMetricId {
			var otelM otelMeta
			otelM, ok := idrac2Otel[metricId]
			if !ok || otelM.name == "" {

				// If the metricId is not found, check if it is a
				// repeatedMetricId and try prefixing with the group ID (metric report ID)
				otelM, ok = idrac2Otel[strings.ToLower(group.ID)+joinChar+metricId]
				if !ok || otelM.name == "" {
					slog.Warn("OtelMeta not found for the redfish MetricId", "metricId", metricId)
					continue
				}
			}
			if scope.Attributes == nil {

				//  Get the report time stamp from the DataGroup for scope attribute
				reportTime, err := parseRFC3339ToNanos(group.Timestamp)
				if err != nil {
					slog.Warn("skip record with bad timestamp", "timestamp", group.Timestamp, "error", err)
					continue
				}
				addScopeAttributes(otelM, fqdd, strconv.FormatInt(reportTime, 10), scope)
			}
			m := &metricsv1.Metric{
				Name:        otelM.name,
				Description: otelM.description,
				Unit:        otelM.unit,
				Data: &metricsv1.Metric_Gauge{
					Gauge: &metricsv1.Gauge{DataPoints: make([]*metricsv1.NumberDataPoint, 0)},
				},
			}
			if otelM.enum != nil {
				m.Metadata = make([]*commonv1.KeyValue, 0)
				for k, v := range otelM.enum {
					m.Metadata = append(m.Metadata, kv("enum."+k, strconv.Itoa(v.(int))))
				}
			}
			for _, p := range dps {
				switch v := p.value.(type) {
				case float64:
					// no conversion needed
					m.GetGauge().DataPoints = append(m.GetGauge().DataPoints, &metricsv1.NumberDataPoint{
						TimeUnixNano: uint64(p.time),
						Attributes:   p.attr,
						Value:        &metricsv1.NumberDataPoint_AsDouble{AsDouble: v},
					})
				case string:
					// enum string to int conversion
					if otelM.enum == nil {
						slog.Warn("no enum map. cannot convert value", "metricId", metricId, "value", v)
						continue
					}
					ev, ok := otelM.enum[strings.ToLower(v)]
					if !ok {
						slog.Warn("no enum mapping for value", "metricId", metricId, "value", v)
						continue
					}
					m.GetGauge().DataPoints = append(m.GetGauge().DataPoints, &metricsv1.NumberDataPoint{
						TimeUnixNano: uint64(p.time),
						Attributes:   p.attr,
						Value:        &metricsv1.NumberDataPoint_AsInt{AsInt: int64(ev.(int))},
					})
				default:
					slog.Warn("unexpected value type for metric id", "metricId", metricId, "value", v, "type", fmt.Sprintf("%T", v))
					continue
				}
			}
			metrics = append(metrics, m)
		}
		rm.ScopeMetrics = append(rm.ScopeMetrics, &metricsv1.ScopeMetrics{
			Scope:   scope,
			Metrics: metrics,
		})
	}
	return rm, nil
}

// mapSeverity maps a textual severity string to the corresponding OpenTelemetry SeverityNumber.
//
// Parameters:
//
//	s string - severity text (e.g., "ok", "warning", "critical").
//
// Returns:
//
//	logsv1.SeverityNumber - the corresponding severity enum value.
func mapSeverity(s string) logsv1.SeverityNumber {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ok":
		return logsv1.SeverityNumber_SEVERITY_NUMBER_INFO
	case "warning":
		return logsv1.SeverityNumber_SEVERITY_NUMBER_WARN
	case "critical":
		return logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR
	default:
		return logsv1.SeverityNumber_SEVERITY_NUMBER_UNSPECIFIED
	}
}

// toOTLPLogs converts a DataGroup containing events into an
// OTLP ResourceLogs protobuf message.
//
// Parameters:
//
//	group *databus.DataGroup - the data group with event information.
//
// Returns:
//
//	(*logsv1.ResourceLogs, error) - the constructed ResourceLogs and any error encountered during conversion.
func toOTLPLogs(group *databus.DataGroup) (*logsv1.ResourceLogs, error) {
	var records []*logsv1.LogRecord

	for _, event := range group.Events {
		etime, err := parseRFC3339ToNanos(event.EventTimestamp)
		if err != nil {
			slog.Warn("error formatting timestamp", "error", err, "event id", event.EventId)
			continue
		}
		attrs := []*commonv1.KeyValue{
			kv("event.data.type", "telemetry"),
			kv("event.object.type", event.EventType),
			kv("event.object.id", event.EventId),
		}
		jstr, err := json.Marshal(event)
		if err != nil {
			slog.Warn("error marshaling event data", "error", err, "event id", event.EventId)
			continue
		}
		lr := &logsv1.LogRecord{
			TimeUnixNano:         uint64(etime),
			ObservedTimeUnixNano: uint64(etime),
			SeverityText:         event.MessageSeverity,
			SeverityNumber:       mapSeverity(event.MessageSeverity),
			Attributes:           attrs,
			Body:                 &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: string(jstr)}},
		}
		records = append(records, lr)
	}

	if len(records) == 0 {
		return &logsv1.ResourceLogs{
			ScopeLogs: []*logsv1.ScopeLogs{},
		}, nil
	}

	rl := &logsv1.ResourceLogs{
		Resource: &resourcev1.Resource{
			Attributes: getHostAttributes(group),
		},
		ScopeLogs: []*logsv1.ScopeLogs{
			{
				Scope: &commonv1.InstrumentationScope{
					Name:    "Lifecycle Logs",
					Version: "1.0.0",
				},
				LogRecords: records,
			},
		},
	}
	return rl, nil
}

func convertAndSendOtelMetrics(ctx context.Context, groupsChan chan *databus.DataGroup, exp *httpExporter) {
	for {
		group := <-groupsChan
		if group.ID == "MemoryMetrics" {
			continue
		}
		if len(group.Values) > 0 {
			rm, err := toOTLPMetrics(group)
			if err != nil {
				slog.Error("error converting metrics to OTLP", "error", err)
				continue
			}
			if len(rm.ScopeMetrics) == 0 {
				slog.Warn("no metrics converted", "group id", group.ID)
				continue
			}
			if err := exp.exportMetrics(ctx, rm); err != nil {
				slog.Error("error exporting metrics", "error", err)
			}
		} else if len(group.Events) > 0 {
			rl, err := toOTLPLogs(group)
			if err != nil {
				slog.Error("error converting logs to OTLP", "error", err)
				continue
			}
			if len(rl.ScopeLogs) == 0 {
				slog.Warn("no logs converted", "group id", group.ID)
				continue
			}
			if err := exp.exportLogs(ctx, rl); err != nil {
				slog.Error("error exporting logs", "error", err)
			}
		}

	}

}

func main() {
	slog.SetLogLoggerLevel(slog.LevelWarn)
	slog.SetDefault(slog.Default().With("pump", "otelpump"))

	getEnvSettings()
	configStringsMu.RLock()
	host := configStrings["mbhost"]
	port, _ := strconv.Atoi(configStrings["mbport"])
	configStringsMu.RUnlock()

	// internal message bus
	var mb messagebus.Messagebus
	for {
		smb, err := stomp.NewStompMessageBus(host, port)
		if err == nil {
			defer smb.Close()
			mb = smb
			break
		}
		slog.Warn("Could not connect to message bus ", "host", host, "port", port, "error", err)
		time.Sleep(time.Minute)
	}

	dbClient := new(databus.DataBusClient)
	dbClient.Bus = mb
	configService := config.NewConfigService(mb, "/otelpump/config", configItems)

	dbClient.Subscribe("/otel")
	dbClient.Get("/otel")
	groupsIn := make(chan *databus.DataGroup, 10)
	go dbClient.GetGroup(groupsIn, "/otel")
	go configService.Run()

	var ocUrl, kcert, kccert, kckey string
	var skipVerify bool

	// wait for configuration
	for {
		configStringsMu.RLock()
		ocUrl = configStrings["otelCollector"]
		if configStrings["otelCACert"] != "" {
			kcert = "/extrabin/certs/" + configStrings["otelCACert"]
		}
		if configStrings["otelClientCert"] != "" {
			kccert = "/extrabin/certs/" + configStrings["otelClientCert"]
		}
		if configStrings["otelClientKey"] != "" {
			kckey = "/extrabin/certs/" + configStrings["otelClientKey"]
		}

		if configStrings["otelSkipVerify"] == "true" {
			skipVerify = true
		}

		slog.Info("Read configuration", "config", configStrings)

		configStringsMu.RUnlock()

		// minimum config available
		if ocUrl != "" {
			slog.Info("otel minimum configuration available, continuing ... \n")
			break
		}
		// wait for min configuration
		time.Sleep(time.Minute)
	}

	readOtelMeta("/extrabin/redfishToOtel.yaml")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exp, err := newHTTPExporter(ocUrl, kcert, kccert, kckey, skipVerify)
	if err != nil {
		slog.Error("error creating HTTP exporter", "error", err)
		return
	}

	slog.Info("Entering processing loop....")
	// convert DMTF metrics to OTEL format and send to OTEL Collector
	convertAndSendOtelMetrics(ctx, groupsIn, exp)
}
