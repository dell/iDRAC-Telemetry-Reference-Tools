// Licensed to You under the Apache License, Version 2.0.

package databus

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/mitchellh/mapstructure"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

type DataValue struct {
	ID        string
	Context   string
	Label     string
	Value     string
	System    string
	HostName  string
	Timestamp string
	// MessageId                 string
	// EventType                 string
	// MaxBandwidthPercent       float64
	// MinBandwidthPercent       float64
	// DiscardedPkts             float64
	// RxBroadcast               float64
	// RxBytes                   float64
	// RxErrorPktAlignmentErrors float64
	// RxMulticastPackets        float64
	// RxUnicastPackets          float64
	// TxBroadcast               float64
	// TxBytes                   float64
	// TxMutlicastPackets        float64
	// TxUnicastPackets          float64
}

type EventValue struct {
	EventType         string
	EventId           string
	EventTimestamp    string
	MemberId          string
	MessageSeverity   string
	Message           string
	MessageId         string
	MessageArgs       []string
	OriginOfCondition string
}

type DataGroup struct {
	ID        string
	Label     string
	Sequence  string
	System    string
	HostName  string
	Model     string
	SKU       string
	FQDN      string
	FwVer     string
	ImgID     string
	Timestamp string
	Values    []DataValue
	Events    []EventValue
}

type DataProducer struct {
	Hostname  string
	Username  string
	State     string
	LastEvent time.Time
}

const (
	STARTING    = "Starting"
	RUNNING     = "Running"
	TELNOTFOUND = "Telemetry Service Not Found"
	CONNFAILED  = "Connection Failed"
)

const (
	GET            = "get"
	SUBSCRIBE      = "subscribe"
	GETPRODUCERS   = "getproducers"
	DELETEPRODUCER = "deleteproducers"
	TERMINATE      = "terminate"
)

type Command struct {
	Command      string `json:"command"`
	ReceiveQueue string `json:"ReceiveQueue"`
	ReportData   string `json:"reportdata,omitempty"`
	ServiceIP    string `json:"serviceIP,omitempty"`
}

type Response struct {
	Command  string      `json:"command"`
	DataType string      `json:"dataType"`
	Data     interface{} `json:"data"`
}

const CommandQueue = "/databus"

type DataBusService struct {
	Recievers []string
	Bus       messagebus.Messagebus
}

type DataBusClient struct {
	Bus messagebus.Messagebus
}

func (d *DataBusService) SendResponse(queue string, command string, dataType string, data interface{}) error {
	res := new(Response)
	res.Command = command
	res.DataType = dataType
	res.Data = data
	jsonStr, _ := json.Marshal(res)
	err := d.Bus.SendMessage(jsonStr, queue)
	if err != nil {
		log.Printf("Failed to send response %v", err)
	}
	return err
}

func (d *DataBusService) SendMultipleResponses(command string, dataType string, data interface{}) {
	res := new(Response)
	res.Command = command
	res.DataType = dataType
	res.Data = data
	jsonStr, _ := json.Marshal(res)
	for _, queue := range d.Recievers {
		err := d.Bus.SendMessage(jsonStr, queue)
		if err != nil {
			log.Printf("Failed to send response %v", err)
		}
	}
}

func (d *DataBusService) SendGroup(group DataGroup) {
	d.SendMultipleResponses(SUBSCRIBE, "DataGroup", group)
}

func (d *DataBusService) SendGroupToQueue(group DataGroup, queue string) {
	d.SendResponse(queue, GET, "DataGroup", group)
}

func (d *DataBusService) SendProducersToQueue(producer []*DataProducer, queue string) error {
	err := d.SendResponse(queue, GETPRODUCERS, "DataProducer", producer)
	return err
}

func (d *DataBusService) ReceiveCommand(commands chan<- *Command) error {
	messages := make(chan string, 10)

	go func() {
		_, err := d.Bus.ReceiveMessage(messages, CommandQueue)
		if err != nil {
			log.Printf("Error recieving messages %v", err)
		}
	}()
	for {
		message := <-messages
		command := new(Command)
		err := json.Unmarshal([]byte(message), command)
		if err != nil {
			log.Printf("Error reading command queue: ", err)
			//return err
		}
		if command.Command == SUBSCRIBE {
			found := false
			for _, rec := range d.Recievers {
				if rec == command.ReceiveQueue {
					found = true
				}
			}
			if !found {
				d.Recievers = append(d.Recievers, command.ReceiveQueue)
			}
		} else {
			commands <- command
		}
	}
	return nil
}

func (d *DataBusClient) SendCommand(command Command) {
	jsonStr, _ := json.Marshal(command)
	err := d.Bus.SendMessage(jsonStr, CommandQueue)
	if err != nil {
		log.Printf("Failed to send command %v", err)
	}
}

func (d *DataBusClient) Get(queue string) {
	var command Command
	command.Command = GET
	command.ReceiveQueue = queue
	d.SendCommand(command)
}

func (d *DataBusClient) Subscribe(queue string) {
	var command Command
	command.Command = SUBSCRIBE
	command.ReceiveQueue = queue
	d.SendCommand(command)
}

func (d *DataBusClient) ReadOneMessage(queue string) string {
	messages := make(chan string)
	sub, err := d.Bus.ReceiveMessage(messages, queue)
	if err != nil {
		log.Println("Error receiving message: ", err)
		return ""
	}
	message := <-messages
	//log.Println("Got message: ", message)
	sub.Close()
	return message
}

func (d *DataBusClient) GetResponse(queue string) *Response {
	message := d.ReadOneMessage(queue)
	resp := new(Response)
	err := json.Unmarshal([]byte(message), resp)
	if err != nil {
		log.Print("Error reading queue: ", err)
	}

	return resp
}

func (d *DataBusClient) DeleteProducer(queue string, service auth.Service) {
	fmt.Println("Entered Delete Producer")
	var command Command
	command.Command = DELETEPRODUCER
	command.ReceiveQueue = queue
	command.ServiceIP = service.Ip
	d.SendCommand(command)
}

func (d *DataBusClient) GetProducers(queue string) []DataProducer {
	var command Command
	command.Command = GETPRODUCERS
	command.ReceiveQueue = queue
	d.SendCommand(command)

	resp := d.GetResponse(queue)
	fmt.Printf("%+v", resp)

	producers := []DataProducer{}
	mapstructure.Decode(resp.Data, &producers)
	//	return resp.Data.([]DataProducer)
	return producers
}

func (d *DataBusClient) GetGroup(groups chan<- *DataGroup, queue string) {
	messages := make(chan string, 10)

	go func() {
		_, err := d.Bus.ReceiveMessage(messages, queue)
		if err != nil {
			log.Printf("Error recieving messages %v", err)
		}
	}()
	for {
		message := <-messages
		resp := new(Response)
		err := json.Unmarshal([]byte(message), resp)
		if err != nil {
			log.Print("Error reading queue: ", err)
		}

		group := DataGroup{}
		mapstructure.Decode(resp.Data, &group)
		//		group := resp.Data.(DataGroup)
		groups <- &group
	}
}
