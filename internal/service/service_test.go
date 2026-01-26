// Licensed to You under the Apache License, Version 2.0.

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/mock"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/wire"
)

// Test data
type SimpleTestPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BaseClient Tests
func TestNewBaseClient_Simple(t *testing.T) {
	bus := mock.NewMockMessageBus()
	client := NewBaseClient(bus, "/queue/test.command", "/queue/test.reply.", "test", 30*time.Second)

	if client.Bus == nil {
		t.Error("Expected Bus to be set")
	}
	if client.CommandQueue != "/queue/test.command" {
		t.Error("Expected CommandQueue to be set correctly")
	}
	if client.ReplyPrefix != "/queue/test.reply." {
		t.Error("Expected ReplyPrefix to be set correctly")
	}
	if client.ClientName != "test" {
		t.Error("Expected ClientName to be set correctly")
	}
	if client.Timeout != 30*time.Second {
		t.Error("Expected Timeout to be set correctly")
	}
	if client.Pending == nil {
		t.Error("Expected Pending map to be initialized")
	}
}

func TestBaseClient_Send_Simple(t *testing.T) {
	bus := mock.NewMockMessageBus()
	client := NewBaseClient(bus, "/queue/test.command", "/queue/test.reply.", "test", 30*time.Second)

	payload := SimpleTestPayload{ID: "123", Name: "test"}
	err := client.Send("TEST_MESSAGE", payload)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	messages := bus.GetMessages("/queue/test.command")
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	// Verify message can be decoded
	env, err := wire.DecodeEnvelope([]byte(messages[0]))
	if err != nil {
		t.Errorf("Failed to decode envelope: %v", err)
	}
	if env.Type != "TEST_MESSAGE" {
		t.Errorf("Expected message type 'TEST_MESSAGE', got '%s'", env.Type)
	}
}

func TestBaseClient_Send_BusError(t *testing.T) {
	bus := mock.NewMockMessageBus()
	bus.SetSendError(errors.New("bus error"))
	client := NewBaseClient(bus, "/queue/test.command", "/queue/test.reply.", "test", 30*time.Second)

	payload := SimpleTestPayload{ID: "123", Name: "test"}
	err := client.Send("TEST_MESSAGE", payload)

	if err == nil {
		t.Error("Expected error due to bus failure")
	}
}

func TestBaseClient_SendEnvelope_Simple(t *testing.T) {
	bus := mock.NewMockMessageBus()
	client := NewBaseClient(bus, "/queue/test.command", "/queue/test.reply.", "test", 30*time.Second)

	payload := SimpleTestPayload{ID: "456", Name: "envelope test"}
	env, err := wire.NewEnvelope("ENVELOPE_TEST", payload)
	if err != nil {
		t.Fatalf("Failed to create envelope: %v", err)
	}

	err = client.SendEnvelope("/queue/test.target", env)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	messages := bus.GetMessages("/queue/test.target")
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
}

// BaseService Tests
func TestNewBaseService_Simple(t *testing.T) {
	bus := mock.NewMockMessageBus()
	service := NewBaseService(bus, "/queue/test.command")

	if service.Bus == nil {
		t.Error("Expected Bus to be set")
	}
	if service.CommandQueue != "/queue/test.command" {
		t.Error("Expected CommandQueue to be set correctly")
	}
}

func TestBaseService_SendEnvelope_Simple(t *testing.T) {
	bus := mock.NewMockMessageBus()
	service := NewBaseService(bus, "/queue/test.command")

	payload := SimpleTestPayload{ID: "789", Name: "service test"}
	env, err := wire.NewEnvelope("SERVICE_TEST", payload)
	if err != nil {
		t.Fatalf("Failed to create envelope: %v", err)
	}

	err = service.SendEnvelope("/queue/test.target", env)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	messages := bus.GetMessages("/queue/test.target")
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
}

func TestBaseService_SendEnvelope_BusError(t *testing.T) {
	bus := mock.NewMockMessageBus()
	bus.SetSendError(errors.New("bus error"))
	service := NewBaseService(bus, "/queue/test.command")

	payload := SimpleTestPayload{ID: "error", Name: "test"}
	env, err := wire.NewEnvelope("ERROR_TEST", payload)
	if err != nil {
		t.Fatalf("Failed to create envelope: %v", err)
	}

	err = service.SendEnvelope("/queue/test.target", env)
	if err == nil {
		t.Error("Expected error due to bus failure")
	}
}

func TestBaseService_Reply_Simple(t *testing.T) {
	bus := mock.NewMockMessageBus()
	service := NewBaseService(bus, "/queue/test.command")

	// Create a request envelope
	reqPayload := SimpleTestPayload{ID: "req123", Name: "request"}
	req, err := wire.NewRequest("TEST_REQUEST", reqPayload, "/queue/test.reply")
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Reply to the request
	replyPayload := SimpleTestPayload{ID: "reply456", Name: "response"}
	err = service.Reply(req, replyPayload)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	messages := bus.GetMessages("/queue/test.reply")
	if len(messages) != 1 {
		t.Errorf("Expected 1 reply message, got %d", len(messages))
	}

	// Verify reply content
	replyEnv, err := wire.DecodeEnvelope([]byte(messages[0]))
	if err != nil {
		t.Errorf("Failed to decode reply: %v", err)
	}
	if replyEnv.CorrelationID != req.CorrelationID {
		t.Errorf("Expected correlation ID %s, got %s", req.CorrelationID, replyEnv.CorrelationID)
	}
	if replyEnv.Status != wire.StatusOK {
		t.Errorf("Expected status OK, got %s", replyEnv.Status)
	}
}

func TestBaseService_Reply_NoReplyTo(t *testing.T) {
	bus := mock.NewMockMessageBus()
	service := NewBaseService(bus, "/queue/test.command")

	// Create request without ReplyTo
	req := Envelope{
		Type:          "TEST_REQUEST",
		CorrelationID: "test-123",
		// ReplyTo is empty
	}

	replyPayload := SimpleTestPayload{ID: "reply", Name: "response"}
	err := service.Reply(req, replyPayload)

	if err == nil {
		t.Error("Expected error for missing ReplyTo")
	}
	if err.Error() != "reply queue missing" {
		t.Errorf("Expected 'reply queue missing', got: %v", err)
	}
}

func TestBaseService_ReplyError_Simple(t *testing.T) {
	bus := mock.NewMockMessageBus()
	service := NewBaseService(bus, "/queue/test.command")

	// Create a request envelope
	reqPayload := SimpleTestPayload{ID: "req123", Name: "request"}
	req, err := wire.NewRequest("TEST_REQUEST", reqPayload, "/queue/test.reply")
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Send error reply
	testError := errors.New("test error message")
	err = service.ReplyError(req, testError)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	messages := bus.GetMessages("/queue/test.reply")
	if len(messages) != 1 {
		t.Errorf("Expected 1 error reply message, got %d", len(messages))
	}

	// Verify error reply content
	replyEnv, err := wire.DecodeEnvelope([]byte(messages[0]))
	if err != nil {
		t.Errorf("Failed to decode error reply: %v", err)
	}
	if replyEnv.CorrelationID != req.CorrelationID {
		t.Errorf("Expected correlation ID %s, got %s", req.CorrelationID, replyEnv.CorrelationID)
	}
	if replyEnv.Status != wire.StatusError {
		t.Errorf("Expected status Error, got %s", replyEnv.Status)
	}
	if replyEnv.Error != "test error message" {
		t.Errorf("Expected error 'test error message', got '%s'", replyEnv.Error)
	}
}

// Edge Cases
func TestBaseClient_InvalidPayload(t *testing.T) {
	bus := mock.NewMockMessageBus()
	client := NewBaseClient(bus, "/queue/test.command", "/queue/test.reply.", "test", 30*time.Second)

	// Try to send invalid payload (channel cannot be marshaled)
	invalidPayload := make(chan int)
	err := client.Send("TEST_MESSAGE", invalidPayload)

	if err == nil {
		t.Error("Expected error for invalid payload")
	}
}
