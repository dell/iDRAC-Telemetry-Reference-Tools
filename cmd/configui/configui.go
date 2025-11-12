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

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/config"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
	"github.com/gin-gonic/gin"
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

type KfkConfig struct {
	Broker          string `json:"kafkaBroker"`
	Topic           string `json:"kafkaTopic"`
	KafkaCACert     string `json:"kafkaCACert"`
	KafkaClientCert string `json:"kafkaClientCert"`
	KafkaClientKey  string `json:"kafkaClientKey"`
	KafkaSkipVerify string `json:"kafkaSkipVerify"`
	TLS             string `json:"tls"`
	ClientAuth      string `json:"clientAuth"`
}

type OtelConfig struct {
	OtelCollector  string `json:"otelCollector"`
	OtelCACert     string `json:"otelCACert"`
	OtelClientCert string `json:"otelClientCert"`
	OtelClientKey  string `json:"otelClientKey"`
	OtelSkipVerify string `json:"otelSkipVerify"`
	TLS            string `json:"tls"`
	ClientAuth     string `json:"clientAuth"`
}

func getKafkaBrokerConfig(c *gin.Context, s *SystemHandler) {
	var KafkaConfig KfkConfig
	s.ConfigBus.CommandQueue = "/kafkapump/config"
	s.ConfigBus.ResponseQueue = "/kconfigui"
	configValues, err := s.ConfigBus.Get("kafkaBroker")
	if err != nil {
		log.Printf("Failed to get kafkaBroker values %v", err)
	} else {
		KafkaConfig.Broker = configValues.Value.(string)
	}

	configValues, err = s.ConfigBus.Get("kafkaTopic")
	if err != nil {
		log.Printf("Failed to get kafkaTopic values %v", err)
	} else {
		KafkaConfig.Topic = configValues.Value.(string)
	}

	configValues, err = s.ConfigBus.Get("kafkaCACert")
	if err != nil {
		log.Printf("Failed to get kafkaTopic values %v", err)
	} else {
		val := configValues.Value.(string)
		if val != "" {
			KafkaConfig.TLS = "true"
		}
	}

	configValues, err = s.ConfigBus.Get("kafkaSkipVerify")
	if err != nil {
		log.Printf("Failed to get kafkaClientCert values %v", err)
	} else {
		KafkaConfig.KafkaSkipVerify = configValues.Value.(string)
	}

	configValues, err = s.ConfigBus.Get("kafkaClientCert")
	if err != nil {
		log.Printf("Failed to get kafkaClientCert values %v", err)
	} else {
		val := configValues.Value.(string)
		if val != "" {
			KafkaConfig.ClientAuth = "true"
		}
	}

	c.JSON(200, KafkaConfig)
}

func getSplunkHttpConfig(c *gin.Context, s *SystemHandler) {
	var SplunkConfig MyHec
	s.ConfigBus.CommandQueue = "/splunkpump/config"
	s.ConfigBus.ResponseQueue = "/configui"
	configValues, err := s.ConfigBus.Get("splunkURL")
	if err != nil {
		log.Printf("Failed to get any config url values %v", err)
	}
	SplunkConfig.Url = configValues.Value.(string)
	configValues, err = s.ConfigBus.Get("splunkKey")
	if err != nil {
		log.Printf("Failed to get any config key values %v", err)
	}
	SplunkConfig.Key = configValues.Value.(string)
	configValues, err = s.ConfigBus.Get("splunkIndex")

	if err != nil {
		log.Printf("Failed to get any Index values %v", err)
	}
	SplunkConfig.Index = configValues.Value.(string)
	c.JSON(200, SplunkConfig)
}

func kafkaConfig(c *gin.Context, s *SystemHandler) {
	var tmp KfkConfig
	err := c.ShouldBind(&tmp)
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	}
	s.ConfigBus.CommandQueue = "/kafkapump/config"
	s.ConfigBus.ResponseQueue = "/kconfigui"

	if tmp.Broker != "" {
		_, err = s.ConfigBus.Set("kafkaBroker", tmp.Broker)
		if err != nil {
			log.Println("Failed to update kafkaBroker config: ", err)
		}
	}

	if tmp.Topic != "" {
		_, err = s.ConfigBus.Set("kafkaTopic", tmp.Topic)
		if err != nil {
			log.Println("Failed to update kafkaTopic config: ", err)
		}
	}

	if tmp.KafkaSkipVerify != "" {
		_, err = s.ConfigBus.Set("kafkaSkipVerify", tmp.KafkaSkipVerify)
		if err != nil {
			log.Println("Failed to update kafkaTopic config: ", err)
		}
	}

	if tmp.KafkaCACert != "" {
		err = SaveUploadedFile(tmp.KafkaCACert, "/extrabin/certs/kafkaCACert")
		if err != nil {
			log.Println("Failed to save CA cert: ", err)
		}

		//log.Println(tmp.KafkaCACert.Filename)
		_, err = s.ConfigBus.Set("kafkaCACert", "kafkaCACert")
		if err != nil {
			log.Println("Failed to update kafkaCACert config: ", err)
		}
	}

	if tmp.KafkaClientCert != "" {
		err = SaveUploadedFile(tmp.KafkaClientCert, "/extrabin/certs/kafkaClientCert")
		if err != nil {
			log.Println("Failed to save client cert: ", err)
		}

		_, err = s.ConfigBus.Set("kafkaClientCert", "kafkaClientCert")
		if err != nil {
			log.Println("Failed to update kafkaClientCert config: ", err)
		}
	}

	if tmp.KafkaClientKey != "" {
		err = SaveUploadedFile(tmp.KafkaClientKey, "/extrabin/certs/kafkaClientKey")
		if err != nil {
			log.Println("Failed to save client key: ", err)
		}
		_, err = s.ConfigBus.Set("kafkaClientKey", "kafkaClientKey")
		if err != nil {
			log.Println("Failed to update kafkaClientCert config: ", err)
		}
	}

}

func otelConfig(c *gin.Context, s *SystemHandler) {
	var tmp OtelConfig
	err := c.ShouldBind(&tmp)
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	}
	s.ConfigBus.CommandQueue = "/otelpump/config"
	s.ConfigBus.ResponseQueue = "/oconfigui"

	if tmp.OtelCollector != "" {
		_, err = s.ConfigBus.Set("otelCollector", tmp.OtelCollector)
		if err != nil {
			log.Println("Failed to update otelCollector config: ", err)
		}
	}

	if tmp.OtelSkipVerify != "" {
		_, err = s.ConfigBus.Set("otelSkipVerify", tmp.OtelSkipVerify)
		if err != nil {
			log.Println("Failed to update otelTopic config: ", err)
		}
	}

	if tmp.OtelCACert != "" {
		err = SaveUploadedFile(tmp.OtelCACert, "/extrabin/certs/otelCACert")
		if err != nil {
			log.Println("Failed to save CA cert: ", err)
		}

		//log.Println(tmp.OtelCACert.Filename)
		_, err = s.ConfigBus.Set("otelCACert", "otelCACert")
		if err != nil {
			log.Println("Failed to update otelCACert config: ", err)
		}
	}

	if tmp.OtelClientCert != "" {
		err = SaveUploadedFile(tmp.OtelClientCert, "/extrabin/certs/otelClientCert")
		if err != nil {
			log.Println("Failed to save client cert: ", err)
		}

		_, err = s.ConfigBus.Set("otelClientCert", "otelClientCert")
		if err != nil {
			log.Println("Failed to update otelClientCert config: ", err)
		}
	}

	if tmp.OtelClientKey != "" {
		err = SaveUploadedFile(tmp.OtelClientKey, "/extrabin/certs/otelClientKey")
		if err != nil {
			log.Println("Failed to save client key: ", err)
		}
		_, err = s.ConfigBus.Set("otelClientKey", "otelClientKey")
		if err != nil {
			log.Println("Failed to update otelClientCert config: ", err)
		}
	}

}

func SaveUploadedFile(cert string, destFile string) error {
	log.Println("Saving cert to ", destFile)
	err := os.WriteFile(destFile, []byte(cert), 0644)
	if err != nil {
		return err
	}
	return nil
}

func configHEC(c *gin.Context, s *SystemHandler) {
	var tmp MyHec
	err := c.ShouldBind(&tmp)
	if err != nil {
		log.Println("Failed to parse json: ", err)
		_ = c.AbortWithError(500, err)
	} else {
		var hecconfig auth.SplunkConfig
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
		Addhec := s.AuthClient.SplunkAddHEC(hecconfig)
		if Addhec != nil {
			log.Println("Failed to add HEC config parse json: ", Addhec)
			_ = c.AbortWithError(500, err)
		} else {
			c.JSON(200, gin.H{"success": "true"})
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
	router.StaticFile("/index.bundle.js", "index.bundle.js")
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
	router.POST("/api/v1/KafkaConfig", func(c *gin.Context) {
		kafkaConfig(c, systemHandler)
	})
	router.POST("/api/v1/OtelConfig", func(c *gin.Context) {
		otelConfig(c, systemHandler)
	})
	router.GET("/api/v1/HttpEventCollector", func(c *gin.Context) {
		getSplunkHttpConfig(c, systemHandler)
	})
	router.GET("api/v1/KafkaBrokerConnection", func(c *gin.Context) {
		getKafkaBrokerConfig(c, systemHandler)
	})
	err := router.Run(fmt.Sprintf(":%s", configStrings["httpport"]))
	if err != nil {
		log.Printf("Failed to run webserver %v", err)
	}
}
