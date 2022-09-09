package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/ini.v1"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/config"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"

	//"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/amqp"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
)

type SplunkEventFields struct {
	Value      float64 `json:"_value"`
	MetricName string  `json:"metric_name"`
	Source     string  `json:"source"`
}

type SplunkEvent struct {
	Time   int64             `json:"time"`
	Event  string            `json:"event"`
	Host   string            `json:"host"`
	Fields SplunkEventFields `json:"fields"`
}

//
// MEB: comment -> this appears to be racy?
//
var configStringsMu sync.RWMutex
var configStrings = map[string]string{
	"mbhost":    "activemq",
	"mbport":    "61613",
	"splunkURL": "http://splunkhost:8088",
	"splunkKey": "",
}

var configItems = map[string]*config.ConfigEntry{
	"splunkURL": {
		Set:     configSet,
		Get:     configGet,
		Default: "http://splunkhost:8088",
	},
	"splunkKey": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"splunkIndex": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
}

var client = &http.Client{}

func configSet(name string, value interface{}) error {
	configStringsMu.Lock()
	defer configStringsMu.Unlock()

	switch name {
	case "splunkURL":
		configStrings["splunkURL"] = value.(string)
	case "splunkKey":
		configStrings["splunkKey"] = value.(string)
	case "splunkIndex":
		configStrings["splunkIndex"] = value.(string)
	default:
		return fmt.Errorf("Unknown property %s", name)
	}
	return nil
}

func configGet(name string) (interface{}, error) {
	switch name {
	case "splunkURL", "splunkKey", "splunkIndex":
		configStringsMu.RLock()
		ret := configStrings[name]
		configStringsMu.RUnlock()
		return ret, nil
	default:
		return nil, fmt.Errorf("Unknown property %s", name)
	}
}

// getEnvSettings grabs environment variables used to configure splunkpump from the running environment. During normal
// operations these should be defined in a docker file and passed into the container which is running splunkpump
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
	splunkURL := os.Getenv("SPLUNK_HEC_URL")
	if len(splunkURL) > 0 {
		configStrings["splunkURL"] = splunkURL
	}
	splunkKey := os.Getenv("SPLUNK_HEC_KEY")
	if len(splunkKey) > 0 {
		configStrings["splunkKey"] = splunkKey
	}
	splunkIndex := os.Getenv("SPLUNK_HEC_INDEX")
	if len(splunkIndex) > 0 {
		configStrings["splunkIndex"] = splunkIndex
	}
}

func logToSplunk(events []*SplunkEvent) {
	var builder strings.Builder
	for _, event := range events {
		b, _ := json.Marshal(event)
		builder.Write(b)
		log.Printf("Timestamp = %d ID = %s System = %s", event.Time, event.Fields.MetricName, event.Host)
	}

	configStringsMu.RLock()
	url := configStrings["splunkURL"] + "/services/collector"
	key := configStrings["splunkKey"]
	configStringsMu.RUnlock()

	req, err := http.NewRequest("POST", url, strings.NewReader(builder.String()))
	if err != nil {
		log.Printf("Error creating request for %s: %v ", url, err)
		return
	}

	req.Header.Add("Authorization", "Splunk "+key)
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error doing http request: ", err)
		return
	}
	if resp.Body != nil {
		resp.Body.Close()
	}
	log.Printf("Sent to Splunk. Got back %d", resp.StatusCode)
}

// handleGroups brings in the events from ActiveMQ
func handleGroups(groupsChan chan *databus.DataGroup) {
	for {
		group := <-groupsChan // If you are new to GoLang see https://golangdocs.com/channels-in-golang
		events := make([]*SplunkEvent, len(group.Values))
		for index, value := range group.Values {
			timestamp, err := time.Parse(time.RFC3339, value.Timestamp)
			if err != nil {
				// For why we do this see https://datatracker.ietf.org/doc/html/rfc3339#section-4.3
				// Go does not handle time properly. See https://github.com/golang/go/issues/20555
				value.Timestamp = strings.ReplaceAll(value.Timestamp, "+0000", "Z")
				timestamp, err = time.Parse(time.RFC3339, value.Timestamp)
				if err != nil {
					log.Printf("Error parsing timestamp for point %s: (%s) %v", value.Context+"_"+value.ID, value.Timestamp, err)
					continue
				}
			}
			event := new(SplunkEvent)
			event.Time = timestamp.Unix()
			event.Event = "metric"
			event.Host = value.System
			floatVal, _ := strconv.ParseFloat(value.Value, 64)
			event.Fields.Value = floatVal
			event.Fields.MetricName = value.Context + "_" + value.ID

			configStringsMu.RLock()
			//fmt.Println("url, key, metricIndex", configStrings["splunkURL"], configStrings["splunkKey"], configStrings["splunkIndex"])
			event.Fields.Source = "http:" + configStrings["splunkIndex"]
			configStringsMu.RUnlock()
			events[index] = event
		}
		logToSplunk(events)
	}
}

func main() {
	// Desired config precedence:  CLI > ENV > configfile > defaults
	//   (but: configfile can be specified by CLI)

	configStringsMu.Lock()
	configName := flag.String("config", "config.ini", "The configuration ini file")
	mbhost := flag.String("mbhost", "", fmt.Sprintf("Message Bus hostname. Overrides default (%s). Overrides environment: MESSAGEBUS_HOST", configStrings["mbhost"]))
	mbport := flag.Int("mbport", 0, fmt.Sprintf("Message Bus port. Overrides default (%s). Overrides environment: MESSAGEBUS_PORT", configStrings["mbport"]))
	splunkurl := flag.String("splunkurl", "", "Splunk HEC URL ")
	splunkkey := flag.String("splunkkey", "", "Splunk HEC Key")
	splunkindex := flag.String("splunkindex", "", "Splunk HEC Index")

	flag.Parse()

	configIni, err := ini.Load(*configName)
	if err != nil {
		log.Printf("(continuing/non-fatal) %s: %v", *configName, err)
	}

	var port int
	if err == nil {
		// set from config, fallback to default
		// ie. CONFIG > DEFAULT
		// somewhat silly to go back and forth to strings for all the int configs, but whatevs for now.
		configStrings["mbhost"] = configIni.Section("General").Key("StompHost").MustString(configStrings["mbhost"])
		port, _ = strconv.Atoi(configStrings["mbport"])
		configStrings["mbport"] = strconv.Itoa(configIni.Section("General").Key("StompPort").MustInt(port))
		configStrings["splunkURL"] = configIni.Section("Splunk").Key("URL").MustString(configStrings["splunkURL"])
		configStrings["splunkKey"] = configIni.Section("Splunk").Key("Key").MustString(configStrings["splunkKey"])
	}

	//Gather configuration from environment variables
	// ie. ENV > CONFIG
	getEnvSettings()

	// Override everything with CLI options
	if *mbhost != "" {
		configStrings["mbhost"] = *mbhost
	}
	if *mbport != 0 {
		configStrings["mbport"] = strconv.Itoa(*mbport)
	}
	if *splunkurl != "" {
		configStrings["splunkURL"] = *splunkurl
	}
	if *splunkkey != "" {
		configStrings["splunkKey"] = *splunkkey
	}
	if *splunkindex != "" {
		configStrings["splunkIndex"] = *splunkindex
	}

	host := configStrings["mbhost"]
	port, _ = strconv.Atoi(configStrings["mbport"])
	configStringsMu.Unlock()

	var mb messagebus.Messagebus
	for {
		mb, err = stomp.NewStompMessageBus(host, port)
		if err == nil {
			defer mb.Close()
			break
		}
		log.Printf("Could not connect to message bus (%s:%d): %v ", host, port, err)
		time.Sleep(time.Minute)
	}

	dbClient := new(databus.DataBusClient)
	dbClient.Bus = mb
	configService := config.NewConfigService(mb, "/splunkpump/config", configItems)
	groupsIn := make(chan *databus.DataGroup, 10)
	dbClient.Subscribe("/spunk")
	dbClient.Get("/spunk")

	log.Printf("Entering processing loop")

	go dbClient.GetGroup(groupsIn, "/spunk")
	go configService.Run()
	handleGroups(groupsIn)
}
