package main

import (
	"encoding/json"
	"fmt"
	"log"
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
	// metric data
	Value      float64 `json:"_value,omitempty"`
	MetricName string  `json:"metric_name,omitempty"`
	Source     string  `json:"source,omitempty"`
	// alert data
	AlertId           string `json:"alert_id,omitempty"`
	MemberId          string `json:"memberid,omitempty"`
	Severity          string `json:"severity,omitempty"`
	MessageId         string `json:"message_id,omitempty"`
	Message           string `json:"message,omitempty"`
	OriginOfCondition string `json:"origin,omitempty"`
}

type kafkaEvent struct {
	Time   int64            `json:"time"`
	Event  string           `json:"event"`
	Host   string           `json:"host"`
	Fields kafkaEventFields `json:"fields"`
}

var configStringsMu sync.RWMutex
var configStrings = map[string]string{
	"mbhost":          "activemq",
	"mbport":          "61613",
	"kafkaBroker":     "",
	"kafkaTopic":      "",
	"kafkaPartition":  "0",
	"kafkaCACert":     "",
	"kafkaClientCert": "",
	"kafkaClientKey":  "",
	"kafkaSkipVerify": "",
}

var configItems = map[string]*config.ConfigEntry{
	"kafkaBroker": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"kafkaTopic": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"kafkaPartition": {
		Set:     configSet,
		Get:     configGet,
		Default: "0",
	},
	"kafkaCACert": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"kafkaClientCert": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"kafkaClientKey": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
	"kafkaSkipVerify": {
		Set:     configSet,
		Get:     configGet,
		Default: "",
	},
}

func configSet(name string, value interface{}) error {
	configStringsMu.Lock()
	defer configStringsMu.Unlock()

	switch name {
	case "kafkaBroker", "kafkaTopic", "kafkaPartition", "kafkaCACert", "kafkaClientCert", "kafkaClientKey", "kafkaSkipVerify":
		configStrings[name] = value.(string)
	default:
		return fmt.Errorf("unknown property %s", name)
	}
	return nil
}

func configGet(name string) (interface{}, error) {
	switch name {
	case "kafkaBroker", "kafkaTopic", "kafkaPartition", "kafkaCACert", "kafkaClientCert", "kafkaClientKey", "kafkaSkipVerify":
		configStringsMu.RLock()
		ret := configStrings[name]
		configStringsMu.RUnlock()
		return ret, nil
	default:
		return nil, fmt.Errorf("unknown property %s", name)
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
	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if len(kafkaBroker) > 0 {
		configStrings["kafkaBroker"] = kafkaBroker
	}
	kafkaTopic := os.Getenv("KAFKA_TOPIC")
	if len(kafkaTopic) > 0 {
		configStrings["kafkaTopic"] = kafkaTopic
	}
	kafkaPartition := os.Getenv("KAFKA_PARTITION")
	if len(kafkaPartition) > 0 {
		configStrings["kafkaPartition"] = kafkaPartition
	}
	kafkaCert := os.Getenv("KAFKA_CACERT")
	if len(kafkaCert) > 0 {
		configStrings["kafkaCACert"] = kafkaCert
	}
	kafkaClientCert := os.Getenv("KAFKA_CLIENT_CERT")
	if len(kafkaClientCert) > 0 {
		configStrings["kafkaClientCert"] = kafkaClientCert
	}
	kafkaClientKey := os.Getenv("KAFKA_CLIENT_KEY")
	if len(kafkaClientKey) > 0 {
		configStrings["kafkaClientKey"] = kafkaClientKey
	}
	kafkaSkipVerify := os.Getenv("KAFKA_SKIP_VERIFY")
	if len(kafkaSkipVerify) > 0 {
		configStrings["kafkaSkipVerify"] = kafkaSkipVerify
	}

}

// handleGroups brings in the events from ActiveMQ
func handleGroups(groupsChan chan *databus.DataGroup, kafkamb messagebus.Messagebus) {
	for {
		group := <-groupsChan // If you are new to GoLang see https://golangdocs.com/channels-in-golang
		// log.Println("Got a group:  size of metrics alerts ", len(group.Values), len(group.Events))
		events := make([]*kafkaEvent, len(group.Values)+len(group.Events))
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

			// --- safe numeric/boolean parsing (silent fallback) ---
			var floatVal float64
			switch strings.ToLower(value.Value) {
			case "true":
				floatVal = 1.0
			case "false":
				floatVal = 0.0
			default:
				f, err := strconv.ParseFloat(value.Value, 64)
				if err != nil {
					f = 0.0 // fallback silently, no log
				}
				floatVal = f
			}

			event.Fields.Value = floatVal
			event.Fields.MetricName = value.Context + "_" + value.ID

			events[index] = event
		}
		// alerts
		for index, evt := range group.Events {
			timestamp, err := time.Parse(time.RFC3339, evt.EventTimestamp)
			if err != nil {
				// For why we do this see https://datatracker.ietf.org/doc/html/rfc3339#section-4.3
				// Go does not handle time properly. See https://github.com/golang/go/issues/20555
				evt.EventTimestamp = strings.ReplaceAll(evt.EventTimestamp, "+0000", "Z")
				timestamp, err = time.Parse(time.RFC3339, evt.EventTimestamp)
				if err != nil {
					log.Printf("Error parsing timestamp for point %s: (%s) %v", evt.EventId+"_"+evt.MessageId, evt.EventTimestamp, err)
					continue
				}
			}
			event := new(kafkaEvent)
			event.Host = group.System
			event.Time = timestamp.Unix()
			event.Event = "alert"
			event.Fields.AlertId = evt.EventId
			event.Fields.MemberId = evt.MemberId
			event.Fields.MessageId = evt.MessageId
			event.Fields.Severity = evt.MessageSeverity
			event.Fields.Message = evt.Message
			event.Fields.OriginOfCondition = evt.OriginOfCondition

			events[index] = event
		}
		// send
		configStringsMu.RLock()
		ktopic := configStrings["kafkaTopic"]
		configStringsMu.RUnlock()

		jsonStr, _ := json.Marshal(events)
		if err := kafkamb.SendMessage(jsonStr, ktopic); err != nil {
			log.Printf("SendMessage error, terminating for restart: %v", err)
			os.Exit(1) // let K8s restart the pod
		}

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
		log.Printf("Could not connect to message bus (%s:%d): %v ", host, port, err)
		time.Sleep(time.Minute)
	}

	dbClient := new(databus.DataBusClient)
	dbClient.Bus = mb
	configService := config.NewConfigService(mb, "/kafkapump/config", configItems)

	dbClient.Subscribe("/kafka")
	dbClient.Get("/kafka")
	groupsIn := make(chan *databus.DataGroup, 10)
	go dbClient.GetGroup(groupsIn, "/kafka")
	go configService.Run()

	// external message bus - kafka
	var kafkamb messagebus.Messagebus
	var ktopic, kpart, kcert, kccert, kckey string

	var kbroker []string
	var skipVerify bool

	// wait for configuration
	for {
		configStringsMu.RLock()
		kbroker = strings.Split(configStrings["kafkaBroker"], ":")
		if configStrings["kafkaCACert"] != "" {
			kcert = "/extrabin/certs/" + configStrings["kafkaCACert"]
		}
		if configStrings["kafkaClientCert"] != "" {
			kccert = "/extrabin/certs/" + configStrings["kafkaClientCert"]
		}
		if configStrings["kafkaClientKey"] != "" {
			kckey = "/extrabin/certs/" + configStrings["kafkaClientKey"]
		}
		ktopic = configStrings["kafkaTopic"]
		kpart = configStrings["kafkaPartition"]

		if configStrings["kafkaSkipVerify"] == "true" {
			skipVerify = true
		}

		log.Println("configStrings : ", configStrings)
		configStringsMu.RUnlock()

		// minimum config available
		if len(kbroker) > 1 && kbroker[0] != "" && ktopic != "" {
			log.Printf("Kafka minimum configuration available, continuing ... \n")
			break
		}
		// wait for min configuration
		time.Sleep(time.Minute)
	}

	// connection loop
	for {
		tlsCfg := &kafka.KafkaTLSConfig{
			ServerCA:   kcert,
			ClientCert: kccert,
			ClientKey:  kckey,
			SkipVerify: skipVerify,
		}

		khost := kbroker[0]
		kport, _ := strconv.Atoi(kbroker[1])
		log.Printf("Connecting to kafka broker (%s:%d) with topic %s, partition %s\n", khost, kport, ktopic, kpart)
		p, _ := strconv.Atoi(kpart)
		kmb, err := kafka.NewKafkaMessageBus(khost, kport, ktopic, p, tlsCfg)
		if err == nil {
			defer kmb.Close()
			kafkamb = kmb
			break
		}

		log.Printf("Could not connect to kafka broker (%s:%d): %v ", khost, kport, err)
		time.Sleep(time.Minute)
	}

	log.Printf("Entering processing loop")

	handleGroups(groupsIn, kafkamb)
}
