// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"

	"github.com/gin-gonic/gin"
	"gopkg.in/ini.v1"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/auth"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/config"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/databus"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/ps"

	//"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/amqp"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

type Process struct {
	Required    bool
	Enabled     bool
	Pid         int
	Running     bool
	Description string
	Uri         string
}

var ProcessList = map[string]*Process{
	"dbdiscauth":     {true, false, -1, false, "Discovery and Authentication from DB", ""},
	"redfishread":    {true, false, -1, false, "Redfish Data Reader/Event Consumer", ""},
	"prometheuspump": {false, false, -1, false, "Prometheus Data Endpoint", ":2112/metrics"},
	"sailfishpump":   {false, false, -1, false, "Redfish Data Creator", ""},
	"sailfish":       {false, false, -1, false, "Redfish Data Endpoint", ""},
	"influxpump":     {false, false, -1, false, "InfluxDB Data Producer", ""},
	"splunkpump":     {false, false, -1, false, "Splunk Data Producer", ""},
}

func getProcessList(c *gin.Context) {
	keys := make([]string, len(ProcessList))
	i := 0
	for k := range ProcessList {
		keys[i] = k
		i++
	}
	procs, err := ps.Processes(keys)
	if err != nil {
		log.Println("Failed to get process list: ", err)
		_ = c.AbortWithError(500, err)
	} else {
		for procName, proc := range procs {
			myProc := ProcessList[procName]
			myProc.Running = proc.Running
			myProc.Pid = proc.Pid
			myProc.Enabled = proc.Enabled
		}
		c.JSON(200, ProcessList)
	}
}

func isValidProcess(processName string) bool {
	for key := range ProcessList {
		if processName == key {
			return true
		}
	}
	return false
}

func getLogs(c *gin.Context) {
	processName := c.Param("processName")
	if !isValidProcess(processName) {
		log.Println("Called with bad process name: ", processName)
		c.AbortWithStatus(404)
		return
	}
	cmd := exec.Command("journalctl", "-u", processName, "--no-pager")
	out, _ := cmd.CombinedOutput()
	c.String(200, "%s", out)
}

func startProcess(c *gin.Context) {
	processName := c.Param("processName")
	if !isValidProcess(processName) {
		log.Println("Called with bad process name: ", processName)
		c.AbortWithStatus(404)
		return
	}
	cmd := exec.Command("systemctl", "start", processName)
	err := cmd.Run()
	if err != nil {
		_ = c.AbortWithError(500, err)
	} else {
		c.JSON(200, gin.H{
			"success": "true",
		})
	}
}

func stopProcess(c *gin.Context) {
	processName := c.Param("processName")
	if !isValidProcess(processName) {
		log.Println("Called with bad process name: ", processName)
		c.AbortWithStatus(404)
		return
	}
	cmd := exec.Command("systemctl", "stop", processName)
	err := cmd.Run()
	if err != nil {
		_ = c.AbortWithError(500, err)
	} else {
		c.JSON(200, gin.H{
			"success": "true",
		})
	}
}

func enableProcess(c *gin.Context) {
	processName := c.Param("processName")
	if !isValidProcess(processName) {
		log.Println("Called with bad process name: ", processName)
		c.AbortWithStatus(404)
		return
	}
	cmd := exec.Command("systemctl", "enable", processName)
	err := cmd.Run()
	if err != nil {
		_ = c.AbortWithError(500, err)
	} else {
		c.JSON(200, gin.H{
			"success": "true",
		})
	}
}

func disableProcess(c *gin.Context) {
	processName := c.Param("processName")
	if !isValidProcess(processName) {
		log.Println("Called with bad process name: ", processName)
		c.AbortWithStatus(404)
		return
	}
	cmd := exec.Command("systemctl", "disable", processName)
	err := cmd.Run()
	if err != nil {
		_ = c.AbortWithError(500, err)
	} else {
		c.JSON(200, gin.H{
			"success": "true",
		})
	}
}

type SystemHandler struct {
	AuthClient *auth.AuthorizationClient
	DataBus    *databus.DataBusClient
}

func getSystemList(c *gin.Context, s *SystemHandler) {
	producers := s.DataBus.GetProducers("/redconfigui/databus_in")
	c.JSON(200, producers)
}

type MySys struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func addSystem(c *gin.Context, s *SystemHandler) {
	var tmp MySys
	err := c.ShouldBind(&tmp)
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	} else {
		var service auth.Service
		service.ServiceType = auth.IDRAC
		service.Ip = tmp.Hostname
		service.AuthType = auth.AuthTypeUsernamePassword
		service.Auth = make(map[string]string)
		service.Auth["username"] = tmp.Username
		service.Auth["password"] = tmp.Password
		s.AuthClient.AddService(service)
		c.JSON(200, gin.H{
			"success": "true",
		})
	}
}

type ConfigHandler struct {
	Bus           messagebus.Messagebus
	ConfigClients map[string]*config.ConfigClient
}

func (h *ConfigHandler) getConfig(c *gin.Context) {
	log.Println("Entered!")
	processName := c.Param("processName")
	if !isValidProcess(processName) {
		log.Println("Called with bad process name: ", processName)
		c.AbortWithStatus(404)
		return
	}
	if h.ConfigClients == nil {
		h.ConfigClients = make(map[string]*config.ConfigClient)
	}
	client, ok := h.ConfigClients[processName]
	if !ok {
		client = config.NewConfigClient(h.Bus, "/"+processName+"/config", "/redconfigui/config/"+processName)
		h.ConfigClients[processName] = client
	}

	props, err := client.GetProperties()
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	} else {
		c.JSON(200, props)
	}
}

func (h *ConfigHandler) getConfigAttribute(c *gin.Context) {
	processName := c.Param("processName")
	if !isValidProcess(processName) {
		log.Println("Called with bad process name: ", processName)
		c.AbortWithStatus(404)
		return
	}
	if h.ConfigClients == nil {
		h.ConfigClients = make(map[string]*config.ConfigClient)
	}
	client, ok := h.ConfigClients[processName]
	if !ok {
		client = config.NewConfigClient(h.Bus, "/"+processName+"/config", "/redconfigui/config/"+processName)
		h.ConfigClients[processName] = client
	}

	props, err := client.Get(c.Param("attrName"))
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	} else {
		c.JSON(200, props)
	}
}

type MyValue struct {
	Value interface{} `json:"value"`
}

func (h *ConfigHandler) setConfigAttribute(c *gin.Context) {
	var tmp MyValue
	err := c.Bind(&tmp)
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	}
	processName := c.Param("processName")
	if !isValidProcess(processName) {
		log.Println("Called with bad process name: ", processName)
		c.AbortWithStatus(404)
		return
	}
	if h.ConfigClients == nil {
		h.ConfigClients = make(map[string]*config.ConfigClient)
	}
	client, ok := h.ConfigClients[processName]
	if !ok {
		client = config.NewConfigClient(h.Bus, "/"+processName+"/config", "/redconfigui/config/"+processName)
		h.ConfigClients[processName] = client
	}

	props, err := client.Set(c.Param("attrName"), tmp.Value)
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	} else {
		c.JSON(200, props)
	}
}

func main() {
	configName := flag.String("config", "config.ini", "The configuration ini file")

	flag.Parse()

	configIni, err := ini.Load(*configName)
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}

	//amqpHost := configIni.Section("General").Key("AmpqHost").MustString("0.0.0.0")
	//amqpPort := configIni.Section("General").Key("AmqpPort").MustInt(5672)
	stompHost := configIni.Section("General").Key("StompHost").MustString("0.0.0.0")
	stompPort := configIni.Section("General").Key("StompPort").MustInt(61613)
	httpPort := configIni.Section("General").Key("AdminPort").MustInt(8082)

	//mb, err := amqp.NewAmqpMessageBus(amqpHost, amqpPort)
	mb, err := stomp.NewStompMessageBus(stompHost, stompPort)
	if err != nil {
		log.Fatal("Could not connect to message bus: ", err)
	}
	defer mb.Close()

	systemHandler := new(SystemHandler)
	systemHandler.AuthClient = new(auth.AuthorizationClient)
	systemHandler.AuthClient.Bus = mb
	systemHandler.DataBus = new(databus.DataBusClient)
	systemHandler.DataBus.Bus = mb

	configHandler := new(ConfigHandler)
	configHandler.Bus = mb

	r := gin.Default()
	r.StaticFile("/", "index.html")
	r.StaticFile("/index.html", "index.html")
	r.GET("/api/v1/Processes", getProcessList)
	r.GET("/api/v1/Systems", func(c *gin.Context) {
		getSystemList(c, systemHandler)
	})
	r.POST("/api/v1/Systems", func(c *gin.Context) {
		addSystem(c, systemHandler)
	})
	r.GET("/api/v1/Logs/:processName", getLogs)
	r.POST("/api/v1/Processes/:processName/Actions/Start", startProcess)
	r.POST("/api/v1/Processes/:processName/Actions/Stop", stopProcess)
	r.POST("/api/v1/Processes/:processName/Actions/Enable", enableProcess)
	r.POST("/api/v1/Processes/:processName/Actions/Disable", disableProcess)
	r.GET("/api/v1/Processes/:processName/config", configHandler.getConfig)
	r.GET("/api/v1/Processes/:processName/config/:attrName", configHandler.getConfigAttribute)
	r.PATCH("/api/v1/Processes/:processName/config/:attrName", configHandler.setConfigAttribute)

	err = r.Run(fmt.Sprintf(":%d", httpPort))
	if err != nil {
		log.Printf("Failed to run webserver %v", err)
	}
}
