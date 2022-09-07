// Licensed to You under the Apache License, Version 2.0.

package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/config"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
)

var configStrings = map[string]string{
	"mbhost":   "activemq",
	"mbport":   "61613",
	"httpport": "8082",
}

type SystemHandler struct {
	AuthClient *auth.AuthorizationClient
	DataBus    *databus.DataBusClient
	ConfigBus  *config.ConfigClient
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

type MyDelSys struct {
	Hostname []string `json:"hostname"`
}

type MyHec struct {
	Url   string `json:"url"`
	Key   string `json:"key"`
	Index string `json:"index"`
}

func configHEC(c *gin.Context, s *SystemHandler) {
	var tmp MyHec
	err := c.ShouldBind(&tmp)
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	} else {
		s.ConfigBus.CommandQueue = "/splunkpump/config"
		s.ConfigBus.ResponseQueue = "/configui"
		_, err = s.ConfigBus.Set("splunkURL", tmp.Url)
		if err != nil {
			log.Printf("Failed to send config (splunkURL) %v", err)
		}
		_, err = s.ConfigBus.Set("splunkKey", tmp.Key)
		if err != nil {
			log.Printf("Failed to send config (splunkKey) %v", err)
		}
		_, err = s.ConfigBus.Set("splunkIndex", tmp.Index)
		if err != nil {
			log.Printf("Failed to send config (splunkIndex) %v", err)
		}
	}
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

func deleteSystem(c *gin.Context, s *SystemHandler) {
	var tmp MyDelSys
	err := c.ShouldBind(&tmp)
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	} else {
		// host := []string{tmp.Hostname}
		for i := 0; i < len(tmp.Hostname); i++ {
			var service auth.Service
			service.ServiceType = auth.IDRAC
			service.Ip = tmp.Hostname[i]
			serviceerr := s.AuthClient.DeleteService(service) // Deletes from database
			if serviceerr != nil {
				log.Println("Failed to delete service parse json: ", serviceerr)
				_ = c.AbortWithError(500, err)
			}
			s.DataBus.DeleteProducer("/configui/databus_in", service)

		}
		c.JSON(200, gin.H{"success": "true"})
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

func handleCsv(c *gin.Context, s *SystemHandler) {
	// Extract the file from context
	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "get form err: %s", err.Error())
		return
	}
	log.Println("Processing uploaded file: ", filepath.Base(file.Filename))

	// Open the file for reading by our CSV reader
	csvFile, err := file.Open()
	if err != nil {
		c.String(http.StatusBadRequest, "upload file err: %s", err.Error())
		return
	}
	defer csvFile.Close()

	reader := csv.NewReader(csvFile)
	idracRecords, _ := reader.ReadAll()

	for _, line := range idracRecords {
		var service auth.Service
		service.ServiceType = auth.IDRAC
		service.Ip = line[0]
		service.AuthType = auth.AuthTypeUsernamePassword
		service.Auth = make(map[string]string)
		service.Auth["username"] = line[1]
		service.Auth["password"] = line[2]
		serviceerr := s.AuthClient.AddService(service)
		if serviceerr != nil {
			log.Println("Failed to add service parse json: ", serviceerr)
			_ = c.AbortWithError(500, err)
		} else {
			c.JSON(200, gin.H{"success": "true"})
		}
	}

	log.Println("Successfully processed: ", filepath.Base(file.Filename))
	c.String(http.StatusOK, "File %s uploaded successfully", file.Filename)
}

func main() {

	//Gather configuration from environment variables
	getEnvSettings()

	systemHandler := new(SystemHandler)
	systemHandler.AuthClient = new(auth.AuthorizationClient)
	systemHandler.DataBus = new(databus.DataBusClient)
	systemHandler.ConfigBus = new(config.ConfigClient)

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
			systemHandler.ConfigBus.Bus = mb
			defer mb.Close()
			break
		}
	}

	// DEBUGGING
	// Uncomment this when you would like to debug configui in a standalone debugger. The issue is that the working
	// directory when in a container will have all the appropriate files. However, when running this in a debugger
	// you have to change the working directory to the appropriate directory for everything to run correctly.
	/*
		os.Chdir("cmd/configui")
		newDir, direrr := os.Getwd()
		if direrr != nil {
		}
		fmt.Printf("Current Working Directory: %s\n", newDir)
	*/

	//Setup http handlers and start webservice
	router := gin.Default()
	router.StaticFile("/", "index.html")
	router.StaticFile("/index.html", "index.html")
	router.GET("/api/v1/Systems", func(c *gin.Context) {
		getSystemList(c, systemHandler)
	})
	router.POST("/api/v1/Systems", func(c *gin.Context) {
		addSystem(c, systemHandler)
	})
	router.POST("/api/v1/CsvUpload", func(c *gin.Context) {
		handleCsv(c, systemHandler)
	})
	router.POST("/api/v1/Delete", func(c *gin.Context) {
		deleteSystem(c, systemHandler)
	})
	router.POST("/api/v1/HecConfig", func(c *gin.Context) {
		configHEC(c, systemHandler)
	})
	err := router.Run(fmt.Sprintf(":%s", configStrings["httpport"]))
	if err != nil {
		log.Printf("Failed to run webserver %v", err)
	}
}
