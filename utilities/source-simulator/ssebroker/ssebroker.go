// Licensed to You under the Apache License, Version 2.0.
// Adapted from github.com/gin-gonic/examples

package ssebroker

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

type (
	SSEBroker struct {

		// Events are pushed to this channel by the main 
		// events-gathering routine
		Message chan string

		// New client connections
		NewClients chan chan string

		// Closed client connections
		ClosedClients chan chan string

		// Total client connections
		TotalClients map[chan string]bool
	}

	// New event messages are broadcast to all registered client connection channels
	ClientChan chan string
)

func NewSSEBroker() (ssebroker *SSEBroker) {
	// Instantiate a broker
	ssebroker = &SSEBroker{
		Message: 	   make(chan string),
		NewClients:    make(chan chan string),
		ClosedClients: make(chan chan string),
		TotalClients:  make(map[chan string]bool),
	}

	go ssebroker.Listen()

	return ssebroker
}

func (broker *SSEBroker) ServeHTTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Initialize client channel
		clientChan := make(ClientChan)

		// Send new connection to event server
		broker.NewClients <- clientChan

		defer func() {
			// Send closed connection to event server
			broker.ClosedClients <- clientChan
		}()

		go func() {
			// Send connection that is closed by client to event server
			<-c.Done()
			broker.ClosedClients <- clientChan
		}()

		c.Next()
	}
}

// to accomodate slow clients
const slowClientTimeout time.Duration = time.Second * 5

// Listen for new sse event requests
// Process events clients
func (broker *SSEBroker) Listen() {
	for {
		select {
		case s := <-broker.NewClients:

			// A new client has connected.
			// Register their message channel
			broker.TotalClients[s] = true
			log.Printf("Client added. %d registered clients", len(broker.TotalClients))
		case s := <-broker.ClosedClients:

			// Remove closed client
			delete(broker.TotalClients, s)
			log.Printf("Removed client. %d registered clients", len(broker.TotalClients))
		case event := <-broker.Message:

			// Broadcast message to clients
			for clientChan := range broker.TotalClients {
				select {
				case clientChan <- event:
				case <-time.After(slowClientTimeout):
					log.Print("Skipping a slow or inactive client...")
				}
			}
		}
	}
}

func HeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")
		c.Next()
	}
}
