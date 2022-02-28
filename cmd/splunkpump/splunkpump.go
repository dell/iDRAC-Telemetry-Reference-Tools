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

	bolt "go.etcd.io/bbolt"
	"gopkg.in/ini.v1"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/config"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/databus"

	//"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/amqp"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

type SplunkEventFields struct {
	Value      float64 `json:"_value"`
	MetricName string  `json:"metric_name"`
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
	"splunkKey": "87b52214-1950-4b22-8fd7-f57543431b81",
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
		Default: "87b52214-1950-4b22-8fd7-f57543431b81",
	},
}

var client = &http.Client{}
var bdb *bolt.DB

func configSet(name string, value interface{}) error {
	err := bdb.Update(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket([]byte("Config"))
		if b == nil {
			b, err = tx.CreateBucket([]byte("Config"))
			if err != nil {
				return fmt.Errorf("create bucket: %s", err)
			}
		}
		err = b.Put([]byte(name), []byte(value.(string)))
		return err
	})
	if err != nil {
		log.Printf("Failed to update config db %v", err)
	}
	switch name {
	case "splunkURL":
		configStringsMu.Lock()
		configStrings["splunkURL"] = value.(string)
		configStringsMu.Unlock()
	case "splunkKey":
		configStringsMu.Lock()
		configStrings["splunkKey"] = value.(string)
		configStringsMu.Unlock()
	default:
		return fmt.Errorf("Unknown property %s", name)
	}
	return nil
}

func configGet(name string) (interface{}, error) {
	switch name {
	case "splunkURL":
		fallthrough
	case "splunkKey":
		configStringsMu.RLock()
		ret := configStrings[name]
		configStringsMu.RUnlock()
		return ret, nil
	default:
		return nil, fmt.Errorf("Unknown property %s", name)
	}
}

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
	splunkURL := os.Getenv("SPLUNK_URL")
	if len(mbPort) > 0 {
		configStrings["splunkURL"] = splunkURL
	}
	splunkKey := os.Getenv("SPLUNK_KEY")
	if len(mbPort) > 0 {
		configStrings["splunkKey"] = splunkKey
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
		log.Print("Error creating request: ", err)
	}

	req.Header.Add("Authorization", "Splunk "+key)
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error doing http request: ", err)
	}
	if resp.Body != nil {
		resp.Body.Close()
	}
	log.Printf("Sent to Splunk. Got back %d", resp.StatusCode)
}

func handleGroups(groupsChan chan *databus.DataGroup) {
	for {
		group := <-groupsChan
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
	splunkurl := flag.String("splunkurl", "", "URL of the splunk host")
	splunkkey := flag.String("splunkkey", "", "Splunk key")

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

	host := configStrings["mbhost"]
	port, _ = strconv.Atoi(configStrings["mbport"])
	configStringsMu.Unlock()

	mb, err := stomp.NewStompMessageBus(host, port)
	if err != nil {
		log.Fatal("Could not connect to message bus: ", err)
	}
	defer mb.Close()

	bdb, err = bolt.Open("splunkpump.db", 0666, nil)
	if err != nil {
		log.Fatalf("Fail to open config db: %v", err)
	}

	_ = bdb.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Config"))
		if b == nil {
			return nil
		}
		configStringsMu.Lock()
		defer configStringsMu.Unlock()
		for key := range configStrings {
			v := b.Get([]byte(key))
			configStrings[key] = string(v)
		}
		return nil
	})

	dbClient := new(databus.DataBusClient)
	dbClient.Bus = mb
	configService := config.NewConfigService(mb, "/splunkpump/config", configItems)
	groupsIn := make(chan *databus.DataGroup, 10)
	dbClient.Subscribe("/spunk")
	dbClient.Get("/spunk")
	go dbClient.GetGroup(groupsIn, "/spunk")
	go configService.Run()
	handleGroups(groupsIn)
}
