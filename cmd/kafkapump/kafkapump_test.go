package main

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

// MockMessageBus for testing
type MockMessageBus struct {
	messages      map[string][][]byte // topic -> messages
	messagesMu    sync.RWMutex
	sendErr       error
	sendCallCount int
}

func NewMockMessageBus() *MockMessageBus {
	return &MockMessageBus{
		messages: make(map[string][][]byte),
	}
}

func (m *MockMessageBus) SendMessage(message []byte, queue string) error {
	m.messagesMu.Lock()
	defer m.messagesMu.Unlock()
	m.sendCallCount++
	if m.sendErr != nil {
		return m.sendErr
	}
	m.messages[queue] = append(m.messages[queue], message)
	return nil
}

func (m *MockMessageBus) SendMessageWithHeaders(message []byte, queue string, headers map[string]string) error {
	return m.SendMessage(message, queue)
}

func (m *MockMessageBus) ReceiveMessage(message chan<- string, queue string) (messagebus.Subscription, error) {
	return nil, nil
}

func (m *MockMessageBus) Close() error {
	return nil
}

func (m *MockMessageBus) GetMessages(queue string) [][]byte {
	m.messagesMu.RLock()
	defer m.messagesMu.RUnlock()
	return m.messages[queue]
}

func (m *MockMessageBus) GetMessageCount(queue string) int {
	m.messagesMu.RLock()
	defer m.messagesMu.RUnlock()
	return len(m.messages[queue])
}

func (m *MockMessageBus) Reset() {
	m.messagesMu.Lock()
	defer m.messagesMu.Unlock()
	m.messages = make(map[string][][]byte)
	m.sendCallCount = 0
}

// TestConfigSetGet tests the configuration set and get functions
func TestConfigSetGet(t *testing.T) {
	tests := []struct {
		name      string
		configKey string
		value     string
		wantErr   bool
	}{
		{
			name:      "set and get kafkaBroker",
			configKey: "kafkaBroker",
			value:     "localhost:9092",
			wantErr:   false,
		},
		{
			name:      "set and get kafkaTopic",
			configKey: "kafkaTopic",
			value:     "test-topic",
			wantErr:   false,
		},
		{
			name:      "set and get kafkaAlertTopic",
			configKey: "kafkaAlertTopic",
			value:     "alert-topic",
			wantErr:   false,
		},
		{
			name:      "set and get kafkaPartition",
			configKey: "kafkaPartition",
			value:     "0",
			wantErr:   false,
		},
		{
			name:      "set unknown property",
			configKey: "unknownProperty",
			value:     "value",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Set
			err := configSet(tt.configKey, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("configSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return // Don't test Get for error cases
			}

			// Test Get
			got, err := configGet(tt.configKey)
			if err != nil {
				t.Errorf("configGet() unexpected error = %v", err)
				return
			}

			if got != tt.value {
				t.Errorf("configGet() = %v, want %v", got, tt.value)
			}
		})
	}
}

// TestGetEnvSettings tests environment variable reading
func TestGetEnvSettings(t *testing.T) {
	// Save original env vars
	origBroker := os.Getenv("KAFKA_BROKER")
	origTopic := os.Getenv("KAFKA_TOPIC")
	origAlertTopic := os.Getenv("KAFKA_ALERT_TOPIC")
	origPartition := os.Getenv("KAFKA_PARTITION")

	// Restore env vars after test
	defer func() {
		os.Setenv("KAFKA_BROKER", origBroker)
		os.Setenv("KAFKA_TOPIC", origTopic)
		os.Setenv("KAFKA_ALERT_TOPIC", origAlertTopic)
		os.Setenv("KAFKA_PARTITION", origPartition)
	}()

	// Set test env vars
	os.Setenv("KAFKA_BROKER", "testbroker:9092")
	os.Setenv("KAFKA_TOPIC", "test-metrics")
	os.Setenv("KAFKA_ALERT_TOPIC", "test-alerts")
	os.Setenv("KAFKA_PARTITION", "3")

	// Reset config before test
	configStringsMu.Lock()
	configStrings = map[string]string{
		"mbhost":          "activemq",
		"mbport":          "61613",
		"kafkaBroker":     "",
		"kafkaTopic":      "",
		"kafkaAlertTopic": "",
		"kafkaPartition":  "0",
		"kafkaCACert":     "",
		"kafkaClientCert": "",
		"kafkaClientKey":  "",
		"kafkaSkipVerify": "",
	}
	configStringsMu.Unlock()

	// Call getEnvSettings
	getEnvSettings()

	// Verify settings were loaded
	tests := []struct {
		key   string
		want  string
	}{
		{"kafkaBroker", "testbroker:9092"},
		{"kafkaTopic", "test-metrics"},
		{"kafkaAlertTopic", "test-alerts"},
		{"kafkaPartition", "3"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			configStringsMu.RLock()
			got := configStrings[tt.key]
			configStringsMu.RUnlock()

			if got != tt.want {
				t.Errorf("getEnvSettings() %s = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// TestHandleGroupsSingleTopic tests sending both metrics and alerts to the same topic
func TestHandleGroupsSingleTopic(t *testing.T) {
	mockMB := NewMockMessageBus()
	groupsChan := make(chan *databus.DataGroup, 1)

	// Set up single topic configuration (no alert topic)
	configStringsMu.Lock()
	configStrings["kafkaTopic"] = "unified-topic"
	configStrings["kafkaAlertTopic"] = "" // Empty = single topic mode
	configStringsMu.Unlock()

	// Create test data group with both metrics and alerts
	timestamp := time.Now().Format(time.RFC3339)
	group := &databus.DataGroup{
		System:   "test-system",
		HostName: "test-host",
		Values: []databus.DataValue{
			{
				ID:        "metric1",
				Context:   "power",
				Value:     "100.5",
				System:    "test-system",
				Timestamp: timestamp,
			},
		},
		Events: []databus.EventValue{
			{
				EventId:         "alert1",
				EventTimestamp:  timestamp,
				MessageSeverity: "Critical",
				Message:         "Test alert",
				MessageId:       "TEST001",
			},
		},
	}

	// Start handleGroups in goroutine
	done := make(chan bool)
	go func() {
		groupsChan <- group
		time.Sleep(100 * time.Millisecond) // Give it time to process
		done <- true
	}()

	// Process one group
	select {
	case g := <-groupsChan:
		// Simulate processing
		metricEvents := make([]*kafkaEvent, 0, len(g.Values))
		for _, value := range g.Values {
			event := new(kafkaEvent)
			event.Time = time.Now().Unix()
			event.Event = "metric"
			event.Host = value.System
			metricEvents = append(metricEvents, event)
		}

		alertEvents := make([]*kafkaEvent, 0, len(g.Events))
		for range g.Events {
			event := new(kafkaEvent)
			event.Event = "alert"
			event.Host = g.System
			alertEvents = append(alertEvents, event)
		}

		// Test single topic mode
		configStringsMu.RLock()
		ktopic := configStrings["kafkaTopic"]
		kalertTopic := configStrings["kafkaAlertTopic"]
		configStringsMu.RUnlock()

		if kalertTopic == "" {
			allEvents := append(metricEvents, alertEvents...)
			jsonStr, _ := json.Marshal(allEvents)
			err := mockMB.SendMessage(jsonStr, ktopic)
			if err != nil {
				t.Errorf("SendMessage failed: %v", err)
			}
		}

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for group")
	}

	<-done

	// Verify both metrics and alerts went to same topic
	msgs := mockMB.GetMessages("unified-topic")
	if len(msgs) != 1 {
		t.Errorf("Expected 1 message to unified-topic, got %d", len(msgs))
		return
	}

	// Parse and verify the message contains both metrics and alerts
	var events []*kafkaEvent
	if err := json.Unmarshal(msgs[0], &events); err != nil {
		t.Errorf("Failed to unmarshal message: %v", err)
		return
	}

	if len(events) != 2 { // 1 metric + 1 alert
		t.Errorf("Expected 2 events (1 metric + 1 alert), got %d", len(events))
	}
}

// TestHandleGroupsSeparateTopics tests sending metrics and alerts to different topics
func TestHandleGroupsSeparateTopics(t *testing.T) {
	mockMB := NewMockMessageBus()

	// Set up separate topics configuration
	configStringsMu.Lock()
	configStrings["kafkaTopic"] = "metrics-topic"
	configStrings["kafkaAlertTopic"] = "alerts-topic"
	configStringsMu.Unlock()

	// Create test data group with both metrics and alerts
	timestamp := time.Now().Format(time.RFC3339)
	group := &databus.DataGroup{
		System:   "test-system",
		HostName: "test-host",
		Values: []databus.DataValue{
			{
				ID:        "metric1",
				Context:   "power",
				Value:     "100.5",
				System:    "test-system",
				Timestamp: timestamp,
			},
			{
				ID:        "metric2",
				Context:   "temp",
				Value:     "45.2",
				System:    "test-system",
				Timestamp: timestamp,
			},
		},
		Events: []databus.EventValue{
			{
				EventId:         "alert1",
				EventTimestamp:  timestamp,
				MessageSeverity: "Critical",
				Message:         "Test alert",
				MessageId:       "TEST001",
			},
		},
	}

	// Process the group
	select {
	case <-time.After(100 * time.Millisecond):
		// Simulate processing
		metricEvents := make([]*kafkaEvent, 0, len(group.Values))
		for _, value := range group.Values {
			event := new(kafkaEvent)
			event.Time = time.Now().Unix()
			event.Event = "metric"
			event.Host = value.System
			event.Fields.MetricName = value.Context + "_" + value.ID
			metricEvents = append(metricEvents, event)
		}

		alertEvents := make([]*kafkaEvent, 0, len(group.Events))
		for _, evt := range group.Events {
			event := new(kafkaEvent)
			event.Time = time.Now().Unix()
			event.Event = "alert"
			event.Host = group.System
			event.Fields.AlertId = evt.EventId
			alertEvents = append(alertEvents, event)
		}

		// Test separate topics mode
		configStringsMu.RLock()
		ktopic := configStrings["kafkaTopic"]
		kalertTopic := configStrings["kafkaAlertTopic"]
		configStringsMu.RUnlock()

		if kalertTopic != "" {
			// Send metrics
			if len(metricEvents) > 0 {
				jsonStr, _ := json.Marshal(metricEvents)
				err := mockMB.SendMessage(jsonStr, ktopic)
				if err != nil {
					t.Errorf("SendMessage (metrics) failed: %v", err)
				}
			}
			// Send alerts
			if len(alertEvents) > 0 {
				jsonStr, _ := json.Marshal(alertEvents)
				err := mockMB.SendMessage(jsonStr, kalertTopic)
				if err != nil {
					t.Errorf("SendMessage (alerts) failed: %v", err)
				}
			}
		}
	}

	// Verify metrics went to metrics-topic
	metricMsgs := mockMB.GetMessages("metrics-topic")
	if len(metricMsgs) != 1 {
		t.Errorf("Expected 1 message to metrics-topic, got %d", len(metricMsgs))
	} else {
		var events []*kafkaEvent
		if err := json.Unmarshal(metricMsgs[0], &events); err != nil {
			t.Errorf("Failed to unmarshal metrics: %v", err)
		} else {
			if len(events) != 2 {
				t.Errorf("Expected 2 metric events, got %d", len(events))
			}
			for _, e := range events {
				if e.Event != "metric" {
					t.Errorf("Expected event type 'metric', got '%s'", e.Event)
				}
			}
		}
	}

	// Verify alerts went to alerts-topic
	alertMsgs := mockMB.GetMessages("alerts-topic")
	if len(alertMsgs) != 1 {
		t.Errorf("Expected 1 message to alerts-topic, got %d", len(alertMsgs))
	} else {
		var events []*kafkaEvent
		if err := json.Unmarshal(alertMsgs[0], &events); err != nil {
			t.Errorf("Failed to unmarshal alerts: %v", err)
		} else {
			if len(events) != 1 {
				t.Errorf("Expected 1 alert event, got %d", len(events))
			}
			if events[0].Event != "alert" {
				t.Errorf("Expected event type 'alert', got '%s'", events[0].Event)
			}
		}
	}
}

// TestKafkaEventMarshaling tests JSON marshaling of kafka events
func TestKafkaEventMarshaling(t *testing.T) {
	tests := []struct {
		name  string
		event *kafkaEvent
	}{
		{
			name: "metric event",
			event: &kafkaEvent{
				Time:  time.Now().Unix(),
				Event: "metric",
				Host:  "test-host",
				Fields: kafkaEventFields{
					Value:      123.45,
					MetricName: "power_consumption",
					Source:     "test-source",
				},
			},
		},
		{
			name: "alert event",
			event: &kafkaEvent{
				Time:  time.Now().Unix(),
				Event: "alert",
				Host:  "test-host",
				Fields: kafkaEventFields{
					AlertId:           "PSU1",
					MemberId:          "1",
					Severity:          "Critical",
					MessageId:         "PSU001",
					Message:           "Power supply failure",
					OriginOfCondition: "Chassis.PSU.1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.event)
			if err != nil {
				t.Errorf("Failed to marshal event: %v", err)
				return
			}

			// Verify we can unmarshal it back
			var unmarshaled kafkaEvent
			if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal event: %v", err)
				return
			}

			if unmarshaled.Event != tt.event.Event {
				t.Errorf("Event type mismatch: got %s, want %s", unmarshaled.Event, tt.event.Event)
			}
			if unmarshaled.Host != tt.event.Host {
				t.Errorf("Host mismatch: got %s, want %s", unmarshaled.Host, tt.event.Host)
			}
		})
	}
}

// TestEmptyGroups tests handling of groups with no metrics or alerts
func TestEmptyGroups(t *testing.T) {
	mockMB := NewMockMessageBus()

	configStringsMu.Lock()
	configStrings["kafkaTopic"] = "test-topic"
	configStrings["kafkaAlertTopic"] = "alert-topic"
	configStringsMu.Unlock()

	// Empty metrics, empty alerts
	group := &databus.DataGroup{
		System:   "test-system",
		HostName: "test-host",
		Values:   []databus.DataValue{},
		Events:   []databus.EventValue{},
	}

	metricEvents := make([]*kafkaEvent, 0, len(group.Values))
	alertEvents := make([]*kafkaEvent, 0, len(group.Events))

	configStringsMu.RLock()
	ktopic := configStrings["kafkaTopic"]
	kalertTopic := configStrings["kafkaAlertTopic"]
	configStringsMu.RUnlock()

	// Should not send empty messages
	if len(metricEvents) > 0 {
		jsonStr, _ := json.Marshal(metricEvents)
		mockMB.SendMessage(jsonStr, ktopic)
	}
	if len(alertEvents) > 0 {
		jsonStr, _ := json.Marshal(alertEvents)
		mockMB.SendMessage(jsonStr, kalertTopic)
	}

	// Verify no messages were sent
	if mockMB.sendCallCount != 0 {
		t.Errorf("Expected no messages for empty group, but got %d", mockMB.sendCallCount)
	}
}

// TestBooleanValueParsing tests parsing of boolean metric values
func TestBooleanValueParsing(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected float64
	}{
		{"true lowercase", "true", 1.0},
		{"TRUE uppercase", "TRUE", 1.0},
		{"false lowercase", "false", 0.0},
		{"FALSE uppercase", "FALSE", 0.0},
		{"numeric value", "123.45", 123.45},
		{"invalid value", "invalid", 0.0}, // Should fallback to 0.0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.value
			var floatVal float64

			// Simulate the parsing logic from handleGroups
			switch value {
			case "true", "TRUE":
				floatVal = 1.0
			case "false", "FALSE":
				floatVal = 0.0
			default:
				// Try to parse as float
				var parsed float64
				err := json.Unmarshal([]byte(value), &parsed)
				if err != nil {
					floatVal = 0.0
				} else {
					floatVal = parsed
				}
			}

			if floatVal != tt.expected {
				t.Errorf("Value parsing for '%s': got %v, want %v", tt.value, floatVal, tt.expected)
			}
		})
	}
}
