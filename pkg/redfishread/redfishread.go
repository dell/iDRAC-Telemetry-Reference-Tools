package redfishread

import (
	"context"
	"fmt"
	"log"
	"os"
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

func InitNewDataGroupsMap() {
	DataGroupsMap = pdatabus.NewDataGroupsMap()
}

func ValidateAndAddDevice(service *auth.Service, devices RedfishDevices) (*RedfishDevice, error) {
	if service.Ip == "" {
		log.Println("Service IP is empty")
		return nil, fmt.Errorf("ServiceIP is empty")
	}
	log.Println("service", service)
	if devices[service.Ip] != nil {
		log.Printf("Device with IP %s already exists", service.Ip)
		return nil, fmt.Errorf("Device with IP %s already exists", service.Ip)
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
		device.State = auth.CONNFAILED
	} else {
		device.State = auth.STARTING
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
	return device, err
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
func RedfishMonitorStart(r *RedfishDevice, dataBusService *databus.DataBusService, authClient auth.AuthClientInterface) {
	systemID, err := r.Redfish.GetSystemId()
	if err != nil || systemID == "" {
		log.Printf("%s: Failed to get system id! %v\n", r.Redfish.GetHostname(), err)
		authClient.UpdateServiceState(auth.CONNFAILED, r.Redfish.GetHostname())
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
	_, err = serviceRoot.GetPropertyByName("TelemetryService")
	if err != nil {
		log.Println("TelemetryService not found, getting only Alerts - ", r.Redfish.GetHostname()) // TODO
		r.State = auth.RUNNINGWOTEL
		authClient.UpdateServiceState(auth.MONITORING, r.Redfish.GetHostname())
		getAlerts(r, dataBusService)
	} else {
		log.Printf("%s: Using Telemetry Service...\n", r.Redfish.GetHostname())
		authClient.UpdateServiceState(auth.RUNNING, r.Redfish.GetHostname())
		getTelemetry(r, dataBusService)
		getAlerts(r, dataBusService)
	}
}

// getTelemetry Starts the service which will listen for SSE reports from the RedfishDevice
func getTelemetry(r *RedfishDevice, dataBusService *databus.DataBusService) {
	r.State = auth.RUNNING
	go r.StartMetricListener(dataBusService)
}

// getAlerts Starts the service which will listen for redfis alerts from the RedfishDevice
func getAlerts(r *RedfishDevice, dataBusService *databus.DataBusService) {
	inclAlerts := os.Getenv("INCLUDE_ALERTS")
	if inclAlerts != "true" {
		return
	}
	r.State = auth.RUNNING
	go r.StartAlertListener(dataBusService)
}
