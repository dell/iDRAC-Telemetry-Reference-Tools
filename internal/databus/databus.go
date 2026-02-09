// Licensed to You under the Apache License, Version 2.0.

package databus

import (
	"context"
	"log"
	"slices"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/service"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/wire"
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
	HostID    string
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
	// commands
	GET            = "get"
	SUBSCRIBE      = "subscribe"
	GETPRODUCERS   = "getproducers"
	DELETEPRODUCER = "deleteproducers"
	TERMINATE      = "terminate"

	// message types
	DATAGROUP = "datagroup"
)

const (
	CommandQueue = "/queue/databus/command"
	ReplyPrefix  = "/queue/databus/reply."
)

type DataBusService struct {
	*service.BaseService
	Recievers []string
}

type DataBusClient struct {
	*service.BaseClient
}

// NewDataBusService constructs a DataBusService backed by the supplied message bus.
// The service listens on CommandQueue and keeps track of subscriber reply queues.
func NewDataBusService(bus messagebus.Messagebus) *DataBusService {
	return &DataBusService{
		BaseService: service.NewBaseService(bus, CommandQueue),
		Recievers:   []string{},
	}
}

// NewDataBusClient constructs a DataBusClient that issues databus commands via
// BaseClient helpers and uses ReplyPrefix to generate unique reply queues.
func NewDataBusClient(bus messagebus.Messagebus, clientName string) *DataBusClient {
	return &DataBusClient{
		BaseClient: service.NewBaseClient(bus, CommandQueue, ReplyPrefix, clientName, 5*time.Second),
	}
}

// handleEnvelope routes envelopes to auxiliary hook logic; currently it ensures
// SUBSCRIBE commands update the receivers list before the envelope reaches
// downstream handlers.
func (d *DataBusService) handleEnvelope(env service.Envelope) {
	if env.Type == SUBSCRIBE {
		d.HandleSubscribeEnvelope(env)
	}
}

// SendGroup broadcasts a DataGroup to every queue recorded in the receivers list.
// Failures to deliver to individual queues are logged and do not stop the fan-out.
func (d *DataBusService) SendGroup(group DataGroup) {
	for _, queue := range d.Recievers {
		if err := d.SendGroupToQueue(group, queue); err != nil {
			log.Printf("Failed to deliver datagroup to %s: %v", queue, err)
		}
	}
}

// SendGroupToQueue sends a single DataGroup to the specified queue using the
// BaseService SendTo helper.
func (d *DataBusService) SendGroupToQueue(group DataGroup, queue string) error {
	return d.SendTo(queue, DATAGROUP, group)
}

// ReceiveEnvelopes starts listening on the databus command queue and streams
// decoded envelopes into the provided channel. SUBSCRIBE envelopes trigger the
// internal hook before being forwarded downstream.
func (d *DataBusService) ReceiveEnvelopes(envelopes chan<- service.Envelope) error {
	return d.BaseService.ListenToQueue(CommandQueue, envelopes, d.handleEnvelope)
}

// HandleSubscribeEnvelope records the ReplyTo queue of a SUBSCRIBE command so
// subsequent broadcasts can reach that client. Empty or duplicate queues are
// ignored.
func (d *DataBusService) HandleSubscribeEnvelope(env service.Envelope) {
	queue := env.ReplyTo
	if queue == "" {
		return
	}
	if slices.Contains(d.Recievers, queue) {
		return
	}
	d.Recievers = append(d.Recievers, queue)
}

// Get requests that producers send their current DataGroups, instructing the
// service to reply to the provided queue.
func (d *DataBusClient) Get(replyQueue string) error {
	return d.BaseClient.SendWithReplyTo(GET, nil, replyQueue)
}

// Subscribe registers the provided queue to receive future DataGroup broadcasts.
func (d *DataBusClient) Subscribe(replyQueue string) error {
	return d.BaseClient.SendWithReplyTo(SUBSCRIBE, nil, replyQueue)
}

// DeleteProducer asks the databus service to stop forwarding updates for the
// specified service, typically after a producer disconnects.
func (d *DataBusClient) DeleteProducer(service auth.Service) error {
	return d.BaseClient.Send(DELETEPRODUCER, service)
}

// GetProducers synchronously requests the list of active data producers.
func (d *DataBusClient) GetProducers() ([]DataProducer, error) {
	var producers []DataProducer
	if err := d.BaseClient.Call(GETPRODUCERS, nil, &producers); err != nil {
		return nil, err
	}
	return producers, nil
}

// GetGroup consumes DATAGROUP envelopes from the specified queue, decodes them,
// and forwards the resulting DataGroups into the caller-provided channel.
func (d *DataBusClient) GetGroup(groups chan<- *DataGroup, queue string) {
	ctx := context.Background()
	envelopes := make(chan service.Envelope, 10)

	go d.BaseClient.ListenToQueueFiltered(ctx, queue, DATAGROUP, envelopes)
	go func() {
		for env := range envelopes {
			var group DataGroup
			if err := wire.DecodePayload(env.Payload, &group); err != nil {
				log.Printf("Error decoding group payload: %v", err)
				continue
			}
			groups <- &group
		}
	}()
}
