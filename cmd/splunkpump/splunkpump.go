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

var configStrings = map[string]string{
	"splunkHost": "http://splunkhost:8088",
	"splunkKey":  "87b52214-1950-4b22-8fd7-f57543431b81",
}

var configItems = map[string]*config.ConfigEntry{
	"splunkHost": {
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
	case "splunkHost":
		configStrings["splunkHost"] = value.(string)
	case "splunkKey":
		configStrings["splunkKey"] = value.(string)
	default:
		return fmt.Errorf("Unknown property %s", name)
	}
	return nil
}

func configGet(name string) (interface{}, error) {
	switch name {
	case "splunkHost":
		fallthrough
	case "splunkKey":
		return configStrings[name], nil
	default:
		return nil, fmt.Errorf("Unknown property %s", name)
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
	splunkHost := os.Getenv("SPLUNK_URL")
	if len(mbPort) > 0 {
		configStrings["splunkHost"] = splunkHost
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

	req, err := http.NewRequest("POST", configStrings["splunkHost"]+"/services/collector", strings.NewReader(builder.String()))
	if err != nil {
		log.Print("Error creating request: ", err)
	}

	req.Header.Add("Authorization", "Splunk "+configStrings["splunkKey"])
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error doing http request: ", err)
	}
	if resp.Body != nil {
		resp.Body.Close()
	}
	log.Printf("%s: Sent to Splunk. Got back %d", configStrings["splunkKey"], resp.StatusCode)
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

	configName := flag.String("config", "config.ini", "The configuration ini file")

	//Gather configuration from environment variables
	getEnvSettings()

	flag.Parse()

	configIni, err := ini.Load(*configName)
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}

	bdb, err = bolt.Open("splunkpump.db", 0666, nil)
	if err != nil {
		log.Fatalf("Fail to open config db: %v", err)
	}

	_ = bdb.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Config"))
		if b == nil {
			return nil
		}
		for key := range configStrings {
			v := b.Get([]byte(key))
			configStrings[key] = string(v)
		}
		return nil
	})

	stompHost := configIni.Section("General").Key("StompHost").MustString("0.0.0.0")
	stompPort := configIni.Section("General").Key("StompPort").MustInt(61613)

	mb, err := stomp.NewStompMessageBus(stompHost, stompPort)
	if err != nil {
		log.Fatal("Could not connect to message bus: ", err)
	}
	defer mb.Close()

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
