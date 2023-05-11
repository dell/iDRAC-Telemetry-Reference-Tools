package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/config"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"

	//"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/amqp"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/kafka"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
)

type kafkaEventFields struct {
	Value      float64 `json:"_value"`
	MetricName string  `json:"metric_name"`
	Source     string  `json:"source"`
}

type kafkaEvent struct {
	Time   int64            `json:"time"`
	Event  string           `json:"event"`
	Host   string           `json:"host"`
	Fields kafkaEventFields `json:"fields"`
}

// MEB: comment -> this appears to be racy?
var configStringsMu sync.RWMutex
var configStrings = map[string]string{
	"mbhost":   "activemq",
	"mbport":   "61613",
	"kafkaURL": "",
	"kafkaKey": "",
}

var configItems = map[string]*config.ConfigEntry{
	"kafkaURL": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"kafkaKey": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"kafkaIndex": {
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
	case "kafkaURL":
		configStrings["kafkaURL"] = value.(string)
	case "kafkaKey":
		configStrings["kafkaKey"] = value.(string)
	case "kafkaIndex":
		configStrings["kafkaIndex"] = value.(string)
	default:
		return fmt.Errorf("Unknown property %s", name)
	}
	return nil
}

func configGet(name string) (interface{}, error) {
	switch name {
	case "kafkaURL", "kafkaKey", "kafkaIndex":
		configStringsMu.RLock()
		ret := configStrings[name]
		configStringsMu.RUnlock()
		return ret, nil
	default:
		return nil, fmt.Errorf("Unknown property %s", name)
	}
}

// getEnvSettings grabs environment variables used to configure kafkapump from the running environment. During normal
// operations these should be defined in a docker file and passed into the container which is running kafkapump
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
	kafkaURL := os.Getenv("KAFKA_URL")
	if len(kafkaURL) > 0 {
		configStrings["kafkaURL"] = kafkaURL
	}

}

// handleGroups brings in the events from ActiveMQ
func handleGroups(groupsChan chan *databus.DataGroup, kafkamb messagebus.Messagebus) {
	for {
		group := <-groupsChan // If you are new to GoLang see https://golangdocs.com/channels-in-golang
		events := make([]*kafkaEvent, len(group.Values))
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
			event := new(kafkaEvent)
			event.Time = timestamp.Unix()
			event.Event = "metric"
			event.Host = value.System
			floatVal, _ := strconv.ParseFloat(value.Value, 64)
			event.Fields.Value = floatVal
			event.Fields.MetricName = value.Context + "_" + value.ID

			events[index] = event
		}
		// send
		jsonStr, _ := json.Marshal(events)
		kafkamb.SendMessage(jsonStr, "kafka")
	}
}

func main() {
	getEnvSettings()
	configStringsMu.RLock()
	host := configStrings["mbhost"]
	port, _ := strconv.Atoi(configStrings["mbport"])
	configStringsMu.RUnlock()

	var mb messagebus.Messagebus
	for {
		smb, err := stomp.NewStompMessageBus(host, port)
		if err == nil {
			defer smb.Close()
			mb = smb
			break
		}
		log.Printf("Could not connect to message bus (%s:%d): %v ", host, port, err)
		time.Sleep(time.Minute)
	}

	var kafkamb messagebus.Messagebus

	for {
		configStringsMu.RLock()
		kurl := strings.Split(configStrings["kafkaURL"], ":")
		configStringsMu.RUnlock()

		if len(kurl) > 1 {
			khost := kurl[0]
			kport, _ := strconv.Atoi(kurl[1])
			log.Printf("Connecting to kafka broker (%s:%d) ", khost, kport)
			kmb, err := kafka.NewKafkaMessageBus(khost, kport)
			if err == nil {
				defer kmb.Close()
				kafkamb = kmb
				break
			}
			log.Printf("Could not connect to kafka broker (%s:%d): %v ", khost, kport, err)
		}
		time.Sleep(time.Minute)
	}

	dbClient := new(databus.DataBusClient)
	dbClient.Bus = mb
	configService := config.NewConfigService(mb, "/kafkapump/config", configItems)
	groupsIn := make(chan *databus.DataGroup, 10)
	dbClient.Subscribe("/kafka")
	dbClient.Get("/kafka")

	log.Printf("Entering processing loop")

	go dbClient.GetGroup(groupsIn, "/kafka")
	go configService.Run()
	handleGroups(groupsIn, kafkamb)
}
