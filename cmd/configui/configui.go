// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/auth"
	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/databus"

	"gitlab.pgre.dell.com/enterprise/telemetryservice/internal/messagebus/stomp"
)

var configStrings = map[string]string{
	"mbhost":   "activemq",
	"mbport":   "61613",
	"httpport": "8082",
}

type SystemHandler struct {
	AuthClient *auth.AuthorizationClient
	DataBus    *databus.DataBusClient
}

func getSystemList(c *gin.Context, s *SystemHandler) {
	producers := s.DataBus.GetProducers("/configui/databus_in")
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
		serviceerr := s.AuthClient.AddService(service)
		if serviceerr != nil {
			log.Println("Failed to add service parse json: ", serviceerr)
			_ = c.AbortWithError(500, err)
		} else {
			c.JSON(200, gin.H{"success": "true"})
		}
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
	httpPort := os.Getenv("CONFIGUI_HTTP_PORT")
	if len(httpPort) > 0 {
		configStrings["httpport"] = httpPort
	}
}

func main() {

	//Gather configuration from environment variables
	getEnvSettings()

	systemHandler := new(SystemHandler)
	systemHandler.AuthClient = new(auth.AuthorizationClient)
	systemHandler.DataBus = new(databus.DataBusClient)

	//Initialize messagebus
	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			systemHandler.AuthClient.Bus = mb
			systemHandler.DataBus.Bus = mb
			defer mb.Close()
			break
		}
	}

	//Setup http handlers and start webservice
	r := gin.Default()
	r.StaticFile("/", "index.html")
	r.StaticFile("/index.html", "index.html")
	r.GET("/api/v1/Systems", func(c *gin.Context) {
		getSystemList(c, systemHandler)
	})
	r.POST("/api/v1/Systems", func(c *gin.Context) {
		addSystem(c, systemHandler)
	})

	err := r.Run(fmt.Sprintf(":%s", configStrings["httpport"]))
	if err != nil {
		log.Printf("Failed to run webserver %v", err)
	}
}
