// Licensed to You under the Apache License, Version 2.0.   See the LICENSE file for more details.

//nolint:revive,funlen,gofmt,stylecheck,gocognit,staticcheck
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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
)

const (
	ip = "localhost"
	// sdk behavior can be tested outside of the box with the following steps
	// change ip to idrac ip
	// update credentials from token to basic
	ERROR = 1
	WARN  = 2
	INFO  = 3
	DEBUG = 4

	successReponseCode = 200

	maxIdleConnsCount     = 10
	idleConnTimoutSeconds = 30
)

var tr = &http.Transport{
	MaxIdleConns:    maxIdleConnsCount,
	IdleConnTimeout: idleConnTimoutSeconds * time.Second,
	DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", "/run/container/http_client_socket")
	},
	// InsecureSkipVerify: true because this client is used for internal localhost calls
	// nolint:gosec
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
}
var client = &http.Client{Transport: tr}
var clientRich = &http.Client{}

var (
	jsonStart  = []byte(`{"resourceMetrics": [{ "resource": {"attributes": [{"key":"`)
	jsonKey    = []byte(`"}},{"key": "`)
	jsonInBtwn = []byte(`","value": {"stringValue":"`)
	jsonEnd    = []byte(`"}}]},`)
)

type otelMeta struct {
	name        string
	description string
	unit        string
	valueType   interface{}
	attributes  map[string]string
	Time        int64
	Value       interface{}
	MetricName  string
}
type attrMeta struct {
	Key   string `json:"key"`
	Value value  `json:"value"`
}

type value struct {
	StringValue string `json:"stringValue"`
}

type otelEventFields struct {
	Value      float64 `json:"_value"`
	MetricName string  `json:"metric_name"`
	Source     string  `json:"source"`
}

type otelEvent struct {
	Time   int64           `json:"time"`
	Event  string          `json:"event"`
	Host   string          `json:"host"`
	Fields otelEventFields `json:"fields"`
}
type SystemDetails struct {
	FQDN    string
	Name    string
	Id      string
	Version string
	SKU     string
	Model   string
}

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

func (o *otelMeta) MarshalJSON() ([]byte, error) {
	type format struct {
		Attributes    []attrMeta  `json:"attributes"`
		STimeUnixNano string      `json:"startTimeUnixNano"`
		TimeUnixNano  string      `json:"timeUnixNano"`
		AsDouble      interface{} `json:"asDouble,omitempty"`
		AsInt         interface{} `json:"asInt,omitempty"`
		// AsString      interface{} `json:"asString,omitempty"`
	}

	target := format{}
	for attr, val := range o.attributes {
		target.Attributes = append(target.Attributes, attrMeta{Key: attr, Value: value{StringValue: val}})
	}
	switch o.valueType {
	case "int", "int64":
		target.AsInt = o.Value
	case "double":
		target.AsDouble = o.Value
	case "string":
		// All the metrics are Gauge type, enum string to int
		target.AsInt = "-1"
		enumStr := strings.ToLower(o.Value.(string))
		logTrace(DEBUG, "enumStr ", enumStr, o.attributes)
		if ival, ok := o.attributes["enum."+enumStr]; ok {
			target.AsInt = ival
		}
	}

	target.TimeUnixNano = strconv.FormatInt(o.Time, 10) + "000000"
	target.STimeUnixNano = strconv.FormatInt(o.Time, 10) + "000000"
	return json.Marshal(&target)
}

type rf2Otel struct {
	ScopeAttr map[string]string
	Metric    []map[string]interface{}
}

var (
	tmpMetar = otelMeta{
		name:        "hw.temperature",
		description: "Temperature in degrees Celsius",
		unit:        "Cel",
		valueType:   "int",
		attributes:  map[string]string{"id": "", "name": "", "parent": "", "hw.type": "temperature"},
	}

	idrac2Otel = map[string]otelMeta{
		"temperature": tmpMetar,
		// others read from yaml  - readOtelMeta()
	}
)

func readOtelMeta() {
	logTrace(INFO, "readOtelMeta: using config /extrabin/redfishToOtel.yaml")
	cfg := viper.New()
	cfg.SetConfigFile("/extrabin/redfishToOtel.yaml")
	err := cfg.ReadInConfig()
	if err != nil {
		logTrace(ERROR, " error reading redfishToOtel.yaml ", err)
		os.Exit(-1)
	}

	subcfg := cfg.Sub("MetricReport")
	if subcfg == nil {
		logTrace(ERROR, " error, MetricReport not found in redfishToOtel.yaml ")
		os.Exit(-1)
	}

	for k := range subcfg.AllSettings() {
		subcfg2 := subcfg.Sub(k)
		if subcfg2 == nil {
			logTrace(WARN, " nil subcfg for ", k)
			continue
		}

		var r2o rf2Otel
		err := subcfg2.Unmarshal(&r2o)
		if err != nil {
			logTrace(WARN, " Unmarshal error ", err)
			continue
		}

		for sa, v := range r2o.ScopeAttr {
			if strings.Contains(sa, "_dot_") {
				delete(r2o.ScopeAttr, sa)
				r2o.ScopeAttr[strings.Replace(sa, "_dot_", ".", 1)] = v
			}
		}
		logTrace(DEBUG, " r2o: scopeattr ", r2o.ScopeAttr)
		logTrace(DEBUG, " r2o: Metric ", r2o.Metric)
		var om otelMeta
		for _, m := range r2o.Metric {
			onm, ok := m["otelmetricname"].(string)
			if !ok {
				logTrace(WARN, "otelMetricName not found!")
				continue
			}
			om.name = onm
			otyp, ok := m["oteltype"].(string)
			if !ok {
				logTrace(WARN, "otelType not found!")
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
				logTrace(WARN, "redfishName not found!")
				continue
			}
			om.attributes = make(map[string]string)
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
			idrac2Otel[rnm] = om
			// logTrace(DEBUG, "otelMeta ", om)
		}
	}
	// logTrace(DEBUG, "idrac2Otel ", idrac2Otel)
}

func PostOtelMetrics(url string, reader io.Reader) (io.ReadCloser, error) {
	tmp := clientRich

	req, err := http.NewRequestWithContext(context.Background(), "POST", url, reader)
	if err != nil {
		logTrace(DEBUG, "Error creating httprequest for url:", url, "error:", err)
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	resp, err := tmp.Do(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("http POST request failed for %s with Error code %d", // do not chnage the message format
			url, resp.StatusCode)
	}
	return nil, nil
}

type telemetryMetric struct {
	MetricId       string
	MetricProperty string
	MetricValue    string
	Oem            struct {
		Dell struct {
			FQDD      string
			Label     string
			Source    string
			ContextId string
		} `json:"Dell"`
	} `json:"Oem"`
	Timestamp string
}

func writeotel(otelM otelMeta, tele telemetryMetric, w io.Writer) {
	if otelM.attributes == nil {
		return
	}
	otelM.attributes["name"] = tele.Oem.Dell.FQDD
	otelM.attributes["id"] = tele.Oem.Dell.FQDD
	otelM.Value = tele.MetricValue
	thetime, _ := time.Parse(time.RFC3339, tele.Timestamp)
	otelM.Time = thetime.UnixMilli()
	err := json.NewEncoder(w).Encode(&otelM)
	if err != nil {
		if err != io.ErrClosedPipe {
			logTrace(WARN, "writeotel ", err)
			return
		}
		logTrace(DEBUG, "writeotel ", err)
	}
}

func InsertMetricReportData(group *databus.DataGroup, w io.Writer) {
	defer func() {
		if r := recover(); r != nil {
			logTrace(WARN, "error parsing the telemetry data", r)
		}
	}()
	var m telemetryMetric
	metricNameLast := ""
	btwn := "\","

	for _, value := range group.Values {
		m.MetricId = value.ID
		if strings.Contains(m.MetricId, "PowerTime") {
			continue
		}
		// map iDRAC metric to Otel meta
		otelM, ok := idrac2Otel[m.MetricId]
		if !ok || otelM.name == "" {
			logTrace(WARN, "OtelMeta not found for the redfish MetricId ", m.MetricId)
			continue
		}
		m.MetricValue = value.Value
		m.Timestamp = value.Timestamp
		m.Oem.Dell.FQDD = value.Context
		m.Oem.Dell.Label = value.Label
		// m.Oem.Dell.Source = dell["Source"].(string)   // TODO: add source
		m.Oem.Dell.ContextId = value.Context

		// metric object separator
		if metricNameLast != "" {
			_, err := w.Write([]byte(","))
			if err != nil {
				//logTrace(ERROR, "InsertMetricReportData(): ", err)
				return
			}
		}
		// start metric object
		_, err := w.Write([]byte("{\"name\":\"" + otelM.name + btwn +
			"\"description\": \"" + otelM.description + btwn +
			"\"unit\": \"" + otelM.unit + btwn +
			"\"gauge\":{\"dataPoints\": ["))
		if err != nil {
			//logTrace(ERROR, "InsertMetricReportData(): ", err)
			return
		}

		// data points
		writeotel(otelM, m, w)

		// end metric object
		_, err = w.Write([]byte("]}}"))
		if err != nil {
			//logTrace(ERROR, "InsertMetricReportData(): ", err)
			return
		}
		metricNameLast = m.MetricId
	}
}

func initOtelCollectorTransport(caCert, clientCert, clientKey string, skipVerify bool) {

	// default transport
	clientRich = &http.Client{Transport: &http.Transport{
		MaxIdleConns:    maxIdleConnsCount,
		IdleConnTimeout: idleConnTimoutSeconds * time.Second,
		// InsecureSkipVerify: true because this is updated with the user defined value
		// when SSL is enabled on the collector
		// nolint:gosec
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}

	// TLS
	if caCert != "" {
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM([]byte(caCert)); !ok {
			logTrace(WARN, "Unable to apppend cert to pool")
		}

		config := tls.Config{
			RootCAs:            pool,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: skipVerify, // nolint:gosec
		}

		// Client Authentication - optional
		if clientCert != "" && clientKey != "" {
			cert, err := tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
			if err != nil {
				logTrace(WARN, "X509KeyPair error ", err)
				return
			}
			config.Certificates = []tls.Certificate{cert}
		}
		clientRich.Transport = &http.Transport{
			MaxIdleConns:    maxIdleConnsCount,
			IdleConnTimeout: idleConnTimoutSeconds * time.Second,
			TLSClientConfig: &config,
		}
	}

}

func convertAndSendOtelMetrics(groupsChan chan *databus.DataGroup, ocUrl string) {
	logTrace(DEBUG, "convertMetricsToOtel ")
	for {
		group := <-groupsChan
		reader, writer := io.Pipe()
		go func() {
			logTrace(INFO, "parsing report ", group.ID)
			hostInfo := map[string]string{
				"host.image.id":      group.ImgID, // TODO: Id ?
				"host.image.version": group.FwVer,
				"host.id":            group.SKU,
				"host.name":          group.FQDN,
				"host.type":          group.Model,
			}
			defer writer.Close()
			fmt.Fprint(writer, string(jsonStart))
			start := true
			for key, value := range hostInfo {
				if start {
					start = false
				} else {
					fmt.Fprint(writer, string(jsonKey))
				}
				fmt.Fprint(writer, key)
				fmt.Fprint(writer, string(jsonInBtwn))
				fmt.Fprint(writer, value)
			}
			fmt.Fprint(writer, string(jsonEnd))
			_, err := writer.Write(
				[]byte("\"scopeMetrics\": [{ \"scope\":{\"name\":\"otelcol/redfishreceiver\",\"version\":\"1.0.0\"},\"metrics\":["))
			if err != nil {
				//logTrace(ERROR, "convertAndSendOtelMetrics(): ", err)
				return
			}
			InsertMetricReportData(group, writer)
			// close metric array
			_, err = writer.Write([]byte("]}]}]}"))
			if err != nil {
				//logTrace(ERROR, "convertAndSendOtelMetrics(): ", err)
			}
		}()

		// send to OTEL Collector
		_, err := PostOtelMetrics(ocUrl, reader)
		if err != nil && !strings.HasSuffix(err.Error(), ": EOF") {
			logTrace(WARN, "Unable to send metrics to Otel Collector ", err)
		}
	}

}

var setLogLevel int

func init() {
	// All logging above Error get printed.
	setLogLevel = INFO
}
func logTrace(logLevel int, v ...any) {
	if logLevel <= setLogLevel {
		fmt.Println(v...)
	}
}
func main() {
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
		logTrace(WARN, "Could not connect to message bus (%s:%d): %v, retrying... ", host, port, err)
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

		log.Println("configStrings : ", configStrings)

		configStringsMu.RUnlock()

		// minimum config available
		if ocUrl != "" {
			logTrace(INFO, "otel minimum configuration available, continuing ... \n")
			break
		}
		// wait for min configuration
		time.Sleep(time.Minute)
	}
	initOtelCollectorTransport(kcert, kccert, kckey, skipVerify)

	readOtelMeta()

	logTrace(INFO, "Entering processing loop....")
	// convert DMTF metrics to OTEL format and send to OTEL Collector
	convertAndSendOtelMetrics(groupsIn, ocUrl)
}
