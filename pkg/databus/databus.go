package databus

import (
	"sync"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/service"
)

const (
	GET            = databus.GET
	SUBSCRIBE      = databus.SUBSCRIBE
	GETPRODUCERS   = databus.GETPRODUCERS
	DELETEPRODUCER = databus.DELETEPRODUCER
	TERMINATE      = databus.TERMINATE
)

type DataGroup = databus.DataGroup
type EventValue = databus.EventValue
type DataBusService = databus.DataBusService
type Envelope = service.Envelope

func NewCommandChan() chan Envelope {
	return make(chan Envelope)
}

type DataGroups struct {
	Mu     sync.RWMutex
	Groups map[string]map[string]*databus.DataGroup // map from sysid to groupID to groups
}

func NewDataGroupChan(n int) chan *databus.DataGroup {
	return make(chan *databus.DataGroup, n)
}

func (d *DataGroups) GetDataGroups() map[string]map[string]*databus.DataGroup {
	d.Mu.RLock()
	defer d.Mu.RUnlock()
	return d.Groups
}

func (d *DataGroups) AddDataGroup(sysId string, group *databus.DataGroup) {
	d.Mu.Lock()
	defer d.Mu.Unlock()
	if d.Groups[sysId] == nil {
		d.Groups[sysId] = make(map[string]*databus.DataGroup)
	}
	d.Groups[sysId][group.ID] = group
}

func NewDataGroupsMap() *DataGroups {
	return &DataGroups{
		Groups: make(map[string]map[string]*databus.DataGroup),
	}
}

func NewDataBusServiceWithBus(mb messagebus.Messagebus) *databus.DataBusService {
	return databus.NewDataBusService(mb)
}

func NewDataBusClientWithBus(mb messagebus.Messagebus, clientName string) *databus.DataBusClient {
	return databus.NewDataBusClient(mb, clientName)
}

func NewDataProducers(n int) []*databus.DataProducer {
	return make([]*databus.DataProducer, n)
}

func NewDataProducer() *databus.DataProducer {
	return &databus.DataProducer{}
}
