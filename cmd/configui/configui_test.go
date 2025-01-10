package main

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/config"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus/stomp"
	"github.com/gin-gonic/gin"
)

// func TestMain(m *testing.M) {

// 	systemHandler := new(SystemHandler)
// 	systemHandler.AuthClient = new(auth.AuthorizationClient)
// 	systemHandler.DataBus = new(databus.DataBusClient)
// 	systemHandler.ConfigBus = new(config.ConfigClient)

// 	for {
// 		stompPort, _ := strconv.Atoi(configStrings["mbport"])
// 		mb, err := stomp.NewStompMessageBus(configStrings["mbhost"], stompPort)
// 		if err != nil {
// 			log.Printf("Could not connect to message bus: %s", err)
// 			time.Sleep(5 * time.Second)
// 		} else {
// 			systemHandler.AuthClient.Bus = mb
// 			systemHandler.DataBus.Bus = mb
// 			systemHandler.ConfigBus.Bus = mb
// 			defer mb.Close()
// 			break
// 		}
// 	}
// 	m.Run()
// }

func connectMessageBus(s *SystemHandler) {
	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus("localhost", stompPort)
		if err != nil {
			log.Printf("Could not connect to message bus: %s", err)
			time.Sleep(5 * time.Second)
		} else {
			s.AuthClient.Bus = mb
			s.DataBus.Bus = mb
			s.ConfigBus.Bus = mb
			defer mb.Close()
			break
		}
	}
}

func TestHandleCsv(t *testing.T) {

	systemHandler := new(SystemHandler)
	systemHandler.AuthClient = new(auth.AuthorizationClient)
	systemHandler.DataBus = new(databus.DataBusClient)
	systemHandler.ConfigBus = new(config.ConfigClient)

	for {
		stompPort, _ := strconv.Atoi(configStrings["mbport"])
		mb, err := stomp.NewStompMessageBus("localhost", stompPort)
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
	// connectMessageBus(systemHandler)

	// Create a new Gin engine
	router := gin.Default()
	router.POST("/api/v1/CsvUpload", func(c *gin.Context) {
		handleCsv(c, systemHandler)
	})

	// Create a new test server
	w := httptest.NewRecorder()

	csvFilePath := "test_upload.csv"
	// Create a CSV file form data object
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	file, err := os.Open(csvFilePath)

	if err != nil {
		t.Fatal("Failed creating form data payload", err)
	}
	defer file.Close()

	part1, _ := writer.CreateFormFile(
		"file",
		filepath.Base(csvFilePath),
	)
	io.Copy(part1, file)

	writer.Close()

	// Create a new request with a CSV file
	req, err := http.NewRequest("POST", "/api/v1/CsvUpload", payload)
	if err != nil {
		t.Fatal(err)
	}

	// Set the content type to csv
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Call the handler
	// router.POST("/upload", s.handleCsv)
	router.ServeHTTP(w, req)

	// Check the response code
	if w.Code != http.StatusOK {
		t.Errorf("Expected response code %d, got %d", http.StatusOK, w.Code)
	}

	// Check the response body
	if w.Body.String() != "{\"success\":\"true\"}{\"success\":\"true\"}File test_upload.csv uploaded successfully" {
		t.Errorf("Expected response body %q, got %q", "{\"success\":\"true\"}{\"success\":\"true\"}File test_upload.csv uploaded successfully", w.Body.String())
	}
}
