package databus

import (
	"sync"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

const (
	GET            = databus.GET
	SUBSCRIBE      = databus.SUBSCRIBE
	GETPRODUCERS   = databus.GETPRODUCERS
	DELETEPRODUCER = databus.DELETEPRODUCER
	TERMINATE      = databus.TERMINATE
)

func NewCommandChan() chan *databus.Command {
	return make(chan *databus.Command)
}

type DataGroups struct {
	Mu     sync.RWMutex
	Groups map[string]map[string]*databus.DataGroup
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
	d.Mu.RLock()
	defer d.Mu.RUnlock()
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
	return &databus.DataBusService{
		Bus: mb,
	}
}

func NewDataBusClientWithBus(mb messagebus.Messagebus) *databus.DataBusClient {
	return &databus.DataBusClient{
		Bus: mb,
	}
}

func NewDataProducers(n int) []*databus.DataProducer {
	return make([]*databus.DataProducer, n)
}

func NewDataProducer() *databus.DataProducer {
	return &databus.DataProducer{}
}
