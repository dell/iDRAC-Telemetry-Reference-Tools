// Licensed to You under the Apache License, Version 2.0.

package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/wire"
)

type Envelope = wire.Envelope

// BaseClient provides common client functionality for all services
type BaseClient struct {
	Bus          messagebus.Messagebus
	CommandQueue string
	ReplyPrefix  string
	ClientName   string
	Timeout      time.Duration
	ReplyQueue   string
	Pending      map[string]chan Envelope
	ReplyMu      sync.RWMutex
	ReplyOnce    sync.Once
	ReplyErr     error
}

// BaseService provides common service functionality for all services
type BaseService struct {
	Bus          messagebus.Messagebus
	CommandQueue string
}

// NewBaseClient creates a new base client with common functionality
func NewBaseClient(bus messagebus.Messagebus, commandQueue, replyPrefix, clientName string, timeout time.Duration) *BaseClient {
	return &BaseClient{
		Bus:          bus,
		CommandQueue: commandQueue,
		ReplyPrefix:  replyPrefix,
		ClientName:   clientName,
		Timeout:      timeout,
		Pending:      make(map[string]chan Envelope),
	}
}

// NewBaseService creates a new base service with common functionality
func NewBaseService(bus messagebus.Messagebus, commandQueue string) *BaseService {
	return &BaseService{
		Bus:          bus,
		CommandQueue: commandQueue,
	}
}

// SendEnvelope sends an envelope to the specified queue
func (bc *BaseClient) SendEnvelope(queue string, env Envelope) error {
	jsonStr, err := wire.EncodeEnvelope(env)
	if err != nil {
		return err
	}
	err = bc.Bus.SendMessage(jsonStr, queue)
	if err != nil {
		log.Printf("Failed to send message to queue %s: %v", queue, err)
	}
	return err
}

// Send sends a message without expecting a reply
func (bc *BaseClient) Send(msgType string, payload any) error {
	return bc.SendTo(bc.CommandQueue, msgType, payload)
}

// SendTo sends a message to a specific queue without expecting a reply
func (bc *BaseClient) SendTo(queue, msgType string, payload any) error {
	env, err := wire.NewEnvelope(msgType, payload)
	if err != nil {
		return err
	}
	return bc.SendEnvelope(queue, env)
}

// SendWithReplyTo sends a message to the default command queue with a ReplyTo override
func (bc *BaseClient) SendWithReplyTo(msgType string, payload any, replyTo string) error {
	return bc.SendToWithReplyTo(bc.CommandQueue, msgType, payload, replyTo)
}

// SendToWithReplyTo sends a message to a specific queue with a ReplyTo override
func (bc *BaseClient) SendToWithReplyTo(queue, msgType string, payload any, replyTo string) error {
	env, err := wire.NewEnvelope(msgType, payload)
	if err != nil {
		return err
	}
	env.ReplyTo = replyTo
	return bc.SendEnvelope(queue, env)
}

// initReplyListener initializes the reply listener for request/reply patterns
func (bc *BaseClient) initReplyListener() error {
	bc.ReplyOnce.Do(func() {
		bc.ReplyQueue = bc.ReplyPrefix + bc.ClientName
		bc.ReplyErr = bc.listenForReplies(bc.ReplyQueue)
	})
	return bc.ReplyErr
}

// listenForReplies listens for reply messages on the reply queue
func (bc *BaseClient) listenForReplies(queue string) error {
	messages := make(chan string, 10)
	// Subscribe synchronously so the channel is registered before we return
	_, err := bc.Bus.ReceiveMessage(messages, queue)
	if err != nil {
		log.Printf("Error receiving reply messages: %v", err)
		return err
	}
	go func() {
		for message := range messages {
			env, err := wire.DecodeEnvelope([]byte(message))
			if err != nil {
				log.Printf("Error decoding reply envelope: %v", err)
				continue
			}
			bc.ReplyMu.Lock()
			if ch, exists := bc.Pending[env.CorrelationID]; exists {
				select {
				case ch <- env:
				default:
					log.Printf("Reply channel full for correlation ID %s", env.CorrelationID)
				}
			}
			bc.ReplyMu.Unlock()
		}
	}()
	return nil
}

// Call sends a message and waits for a reply
func (bc *BaseClient) Call(msgType string, payload any, resp any) error {
	if err := bc.initReplyListener(); err != nil {
		return err
	}
	req, err := wire.NewRequest(msgType, payload, bc.ReplyQueue)
	if err != nil {
		return err
	}
	replyCh := make(chan Envelope, 1)
	bc.ReplyMu.Lock()
	bc.Pending[req.CorrelationID] = replyCh
	bc.ReplyMu.Unlock()
	defer func() {
		bc.ReplyMu.Lock()
		delete(bc.Pending, req.CorrelationID)
		bc.ReplyMu.Unlock()
	}()

	if err := bc.SendEnvelope(bc.CommandQueue, req); err != nil {
		return err
	}

	select {
	case env := <-replyCh:
		if env.Status == wire.StatusError {
			if env.Error != "" {
				return fmt.Errorf("%s", env.Error)
			}
			return fmt.Errorf("request failed")
		}
		return wire.DecodePayload(env.Payload, resp)
	case <-time.After(bc.Timeout):
		return fmt.Errorf("timeout waiting for message from queue %s", bc.ReplyQueue)
	}
}

// ListenToQueueFiltered provides a method to listen to any queue and filter envelopes by type
// Uses context for standard Go cancellation pattern
func (bc *BaseClient) ListenToQueueFiltered(ctx context.Context, queueName string, messageType string, envelopes chan<- Envelope) {
	messages := make(chan string, 10)

	sub, err := bc.Bus.ReceiveMessage(messages, queueName)
	if err != nil {
		log.Printf("Error receiving messages from queue %s: %v", queueName, err)
		close(envelopes)
		return
	}

	go func() {
		defer sub.Close()
		defer close(envelopes)

		for {
			select {
			case message := <-messages:
				env, err := wire.DecodeEnvelope([]byte(message))
				if err != nil {
					log.Printf("Error decoding envelope from queue %s: %v", queueName, err)
					continue
				}

				// Filter by message type
				if env.Type == messageType {
					select {
					case envelopes <- env:
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// SendEnvelope sends an envelope to the specified queue
func (bs *BaseService) SendEnvelope(queue string, env Envelope) error {
	jsonStr, err := wire.EncodeEnvelope(env)
	if err != nil {
		return err
	}
	err = bs.Bus.SendMessage(jsonStr, queue)
	if err != nil {
		log.Printf("Failed to send message to queue %s: %v", queue, err)
	}
	return err
}

// Send sends a message on the service command queue
func (bs *BaseService) Send(msgType string, payload any) error {
	return bs.SendTo(bs.CommandQueue, msgType, payload)
}

// SendTo sends to an arbitrary queue without expecting a reply
func (bs *BaseService) SendTo(queue string, msgType string, payload any) error {
	env, err := wire.NewEnvelope(msgType, payload)
	if err != nil {
		return err
	}
	return bs.SendEnvelope(queue, env)
}

// Reply sends a successful reply to a request
func (bs *BaseService) Reply(req Envelope, payload any) error {
	if req.ReplyTo == "" {
		return fmt.Errorf("reply queue missing")
	}
	env, err := wire.ReplyOK(req, payload)
	if err != nil {
		return err
	}
	return bs.SendEnvelope(req.ReplyTo, env)
}

// ReplyError sends an error reply to a request
func (bs *BaseService) ReplyError(req Envelope, replyErr error) error {
	if req.ReplyTo == "" {
		return fmt.Errorf("reply queue missing")
	}
	env := wire.ReplyErr(req, replyErr)
	return bs.SendEnvelope(req.ReplyTo, env)
}

// ReceiveEnvelopes receives and decodes commands from the command queue
func (bs *BaseService) ReceiveEnvelopes(envelopes chan<- Envelope) error {
	return bs.ListenToQueue(bs.CommandQueue, envelopes, nil)
}

// ListenToQueue provides a generic method to continuously listen to any queue and decode envelopes
func (bs *BaseService) ListenToQueue(queueName string, envelopes chan<- Envelope, hook func(Envelope)) error {
	messages := make(chan string, 10)

	go func() {
		_, err := bs.Bus.ReceiveMessage(messages, queueName)
		if err != nil {
			log.Printf("Error receiving messages from queue %s: %v", queueName, err)
		}
	}()
	for {
		message := <-messages
		env, err := wire.DecodeEnvelope([]byte(message))
		if err != nil {
			log.Printf("Error decoding envelope from queue %s: %v", queueName, err)
			log.Printf("Message %#v\n", message)
			continue
		}
		if hook != nil {
			hook(env)
		}
		envelopes <- env
	}
}
