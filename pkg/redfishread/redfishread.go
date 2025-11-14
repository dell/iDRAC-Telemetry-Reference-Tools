package redfishread

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/redfish"
	pdatabus "github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/databus"
)

type RedfishDevices map[string]*RedfishDevice

func NewRedfishDevices() RedfishDevices {
	return make(RedfishDevices)
}

func (r RedfishDevices) CtxCancel(k string) {
	r[k].CtxCancel()
}

func (r RedfishDevices) Delete(k string) {
	delete(r, k)
}

func (r RedfishDevices) GetCtx(k string) context.Context {
	return r[k].Ctx
}

func (r RedfishDevices) AddDevice(k string, v *RedfishDevice) {
	r[k] = v
}

var DataGroupsMap *pdatabus.DataGroups

// handleAuthServiceChannel Authenticates to the iDRAC and then launches the telemetry monitoring process via
// redfishMonitorStart
func HandleAuthServiceChannel(serviceIn chan *auth.Service, dataBusService *databus.DataBusService, devices RedfishDevices, isTelemetry, isAlerts bool, SSEFilter bool) {
	DataGroupsMap = pdatabus.NewDataGroupsMap()
	for {
		service := <-serviceIn
		if service.Ip == "" {
			log.Println("Service IP is empty")
			continue
		}
		if devices[service.Ip] != nil {
			log.Printf("Device with IP %s already exists", service.Ip)
			continue
		}

		log.Print("Got new service = ", service.Ip)
		var r redfish.RedfishClientInterface
		var err error
		//log.Println(service)
		if service.AuthType == auth.AuthTypeUsernamePassword {
			r, err = redfish.Init(service.Ip, service.Auth["username"], service.Auth["password"], service.ServiceType)
		} else if service.AuthType == auth.AuthTypeBearerToken {
			r, err = redfish.InitBearer(service.Ip, service.Auth["token"])
		}
		//log.Print(r)
		device := new(RedfishDevice)
		if err != nil {
			log.Printf("%s: Failed to instantiate redfish client %v", service.Ip, err)
			// Creating device for failed password so that it will show up on GUI
			// r = new(redfish.RedfishClient)
			// r.Hostname = service.Ip
			// r.Username = service.Auth["username"]
			// r.Password = service.Auth["password"]
			device.State = databus.CONNFAILED
		} else {
			device.State = databus.STARTING
		}
		device.Redfish = r
		device.HasChildren = service.ServiceType == auth.MSM
		ctx, cancel := context.WithCancel(context.Background())
		device.Ctx = ctx
		device.CtxCancel = cancel
		if devices == nil {
			devices = NewRedfishDevices()
		}
		devices.AddDevice(service.Ip, device)
		// Only want validated devices to be started
		if err == nil {
			go redfishMonitorStart(device, dataBusService, isTelemetry, isAlerts, SSEFilter)
		}
	}
}

// populateChildChassis If the device is a chassis, we also have to obtain IDs / info for all children in that chassis
// and pull telemetry on them. This function will expand the chassis information and obtain the necessary information.
func populateChildChassis(r *RedfishDevice, serviceRoot *redfish.RedfishPayload) {
	chassisCollection, err := serviceRoot.GetPropertyByName("Chassis")
	if err != nil {
		log.Println(err)
		return
	}
	size := chassisCollection.GetCollectionSize()
	for i := 0; i < size; i++ {
		chassis, err := chassisCollection.GetPropertyByIndex(i)
		if err != nil {
			continue
		}
		if chassis.Object["ChassisType"].(string) != "Enclosure" && chassis.Object["SKU"] != nil {
			name := chassis.Object["Name"].(string)
			if strings.HasPrefix(name, "Sled-") {
				split := strings.Split(name, "-")
				i, _ := strconv.Atoi(split[1])
				r.ChildDevices[i] = chassis.Object["SKU"].(string)
			}
		}
	}
}

// Take an instance of a Redfish device, get its system ID, get any child devices if it is a chassis, and then start
// listening for SSE events. NOTE: This expects that someone has enabled Telemetry reports and started the telemetry
// service externally.
func redfishMonitorStart(r *RedfishDevice, dataBusService *databus.DataBusService, isTelemetry, isAlerts bool, SSEFilter bool) {
	systemID, err := r.Redfish.GetSystemId()
	if err != nil || systemID == "" {
		log.Printf("%s: Failed to get system id! %v\n", r.Redfish.GetHostname(), err)
		return
	}
	hostName, sku, model, fwver, fqdn, imgid, err := r.Redfish.GetSysInfo()
	if err != nil || hostName == "" {
		log.Printf("%s: Failed to get hostName id! %v\n", r.Redfish.GetHostname(), err)
		// assume same as system id, host name cannot be empty if used as key
		hostName = systemID
	}
	log.Printf("%s: Got System ID %s, Hostname %s\n", r.Redfish.GetHostname(), systemID, hostName)
	r.SystemID = systemID
	r.HostName = hostName
	r.SKU = sku
	r.Model = model
	r.FwVer = fwver
	r.FQDN = fqdn
	r.ImgID = imgid

	r.Redfish.SetFwVer(fwver)

	serviceRoot, err := r.Redfish.GetUri("/redfish/v1")
	if err != nil {
		log.Println(err)
		return
	}
	if r.HasChildren {
		r.ChildDevices = make(map[int]string)
		populateChildChassis(r, serviceRoot)
	}
	//Does this system support Telemetry?
	telemetryService, err := serviceRoot.GetPropertyByName("TelemetryService")
	if err != nil {
		log.Println("TODO: Fake some basic telemetry...") // TODO
		r.State = databus.TELNOTFOUND
	} else {
		log.Printf("%s: Using Telemetry Service...\n", r.Redfish.GetHostname())
		//go getRedfishLce(r, telemetryService, dataBusService)
		getTelemetry(r, telemetryService, dataBusService, isTelemetry, isAlerts, SSEFilter)
	}
}

// getTelemetry Starts the service which will listen for SSE reports from the iDRAC
func getTelemetry(r *RedfishDevice, telemetryService *redfish.RedfishPayload, dataBusService *databus.DataBusService, isTelemetry, isAlerts bool, SSEFilter bool) {
	r.State = databus.RUNNING
	if isAlerts {
		go r.StartAlertListener(dataBusService, SSEFilter)
	}
	if isTelemetry {
		go r.StartMetricListener(dataBusService)
	}

}
