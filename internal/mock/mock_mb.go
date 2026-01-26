package mock

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

type MockSubscription struct {
	ctxCancel context.CancelFunc
}

func (ms MockSubscription) Close() error {
	// No-op for mock
	ms.ctxCancel()
	return nil
}

// MockMessageBus is a mock that implements the Messagebus interface.
//
// For now, it doesn't do anything with the queue name. All messages sent are appended to
// the Messages slice.
type MockMessageBus struct {
	Messages      []string
	queueMsgs     map[string][]string
	replyPayload  any
	sendError     error
	replyChannels map[string]chan<- string
	mu            sync.Mutex
}

// NewMockMessageBus creates a new MockMessageBus instance
func NewMockMessageBus() *MockMessageBus {
	return &MockMessageBus{
		Messages:      []string{},
		queueMsgs:     make(map[string][]string),
		replyChannels: make(map[string]chan<- string),
	}
}

// SetReplyPayload sets the payload that will be used to reply to requests
func (mb *MockMessageBus) SetReplyPayload(payload any) {
	mb.replyPayload = payload
}

// SetReplyMessage is deprecated - use SetReplyPayload instead
func (mb *MockMessageBus) SetReplyMessage(msg string) {
	// For backward compatibility, try to extract payload from JSON
	mb.replyPayload = msg
}

// SetSendError sets an error to be returned by SendMessage
func (mb *MockMessageBus) SetSendError(err error) {
	mb.sendError = err
}

// GetMessages returns messages sent to a specific queue
func (mb *MockMessageBus) GetMessages(queue string) []string {
	return mb.queueMsgs[queue]
}

// AddQueueMessage adds a message to a specific queue (for test setup)
func (mb *MockMessageBus) AddQueueMessage(queue string, msg string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.queueMsgs == nil {
		mb.queueMsgs = make(map[string][]string)
	}
	mb.queueMsgs[queue] = append(mb.queueMsgs[queue], msg)
}

// envelope is a minimal struct to extract correlation ID and reply queue
type envelope struct {
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlationId,omitempty"`
	ReplyTo       string          `json:"replyTo,omitempty"`
	Status        string          `json:"status,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

func (mb *MockMessageBus) SendMessage(message []byte, queue string) error {
	if mb.sendError != nil {
		return mb.sendError
	}
	mb.Messages = append(mb.Messages, string(message))
	if mb.queueMsgs == nil {
		mb.queueMsgs = make(map[string][]string)
	}
	mb.queueMsgs[queue] = append(mb.queueMsgs[queue], string(message))

	// Check if this is a request that needs a reply
	var env envelope
	if err := json.Unmarshal(message, &env); err == nil && env.ReplyTo != "" && env.CorrelationID != "" {
		// This is a request - send a reply if we have a payload configured
		if mb.replyPayload != nil {
			// Build reply envelope with matching correlation ID
			payloadBytes, _ := json.Marshal(mb.replyPayload)
			reply := envelope{
				Type:          env.Type,
				CorrelationID: env.CorrelationID,
				Status:        "ok",
				Payload:       payloadBytes,
			}
			replyJSON, _ := json.Marshal(reply)

			// Send reply directly to the registered channel
			mb.mu.Lock()
			replyCh, exists := mb.replyChannels[env.ReplyTo]
			mb.mu.Unlock()
			if exists {
				// Send in goroutine to avoid blocking
				go func(ch chan<- string, msg string) {
					ch <- msg
				}(replyCh, string(replyJSON))
			}
		}
	}

	return nil
}

func (mb *MockMessageBus) ReceiveMessage(message chan<- string, queue string) (messagebus.Subscription, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Register this channel for potential replies BEFORE returning
	// This ensures the channel is available when SendMessage is called
	mb.mu.Lock()
	if mb.replyChannels == nil {
		mb.replyChannels = make(map[string]chan<- string)
	}
	mb.replyChannels[queue] = message
	mb.mu.Unlock()

	// For legacy tests that pre-load Messages, consume them from the queue-specific slice
	// Reply messages are sent directly via SendMessage, not through this goroutine
	go func() {
		// Check for pre-loaded messages in the queue-specific slice
		mb.mu.Lock()
		queueMessages := mb.queueMsgs[queue]
		mb.mu.Unlock()

		for _, msg := range queueMessages {
			select {
			case message <- msg:
			case <-ctx.Done():
				return
			}
		}
		// Then just wait for context cancellation
		<-ctx.Done()
	}()
	msg := &MockSubscription{
		ctxCancel: cancel,
	}
	return msg, nil
}

func (mb *MockMessageBus) Close() error {
	// No-op for mock
	return nil
}

func (mb *MockMessageBus) SendMessageWithHeaders(message []byte, queue string, headers map[string]string) error {
	// No-op for mock
	return nil
}
