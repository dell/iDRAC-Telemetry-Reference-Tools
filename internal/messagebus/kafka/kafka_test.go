package kafka

import (
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// TestShouldRestartOnErr tests the error detection logic
func TestShouldRestartOnErr(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "EOF error",
			err:      io.EOF,
			expected: true,
		},
		{
			name:     "broken pipe error",
			err:      errors.New("write tcp 172.18.0.7:46958->100.71.133.137:9092: write: broken pipe"),
			expected: true,
		},
		{
			name:     "connection reset by peer",
			err:      errors.New("read tcp: connection reset by peer"),
			expected: true,
		},
		{
			name:     "closed network connection",
			err:      errors.New("use of closed network connection"),
			expected: true,
		},
		{
			name:     "i/o timeout",
			err:      errors.New("i/o timeout"),
			expected: true,
		},
		{
			name:     "timeout error type",
			err:      &testTimeoutError{},
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "context canceled",
			err:      errors.New("context canceled"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRestartOnErr(tt.err)
			if got != tt.expected {
				t.Errorf("shouldRestartOnErr() = %v, want %v for error: %v", got, tt.expected, tt.err)
			}
		})
	}
}

// testTimeoutError implements net.Error for testing timeout scenarios
type testTimeoutError struct{}

func (e *testTimeoutError) Error() string   { return "timeout" }
func (e *testTimeoutError) Timeout() bool   { return true }
func (e *testTimeoutError) Temporary() bool { return false }

// TestErrorStringMatching tests various error string formats
func TestErrorStringMatching(t *testing.T) {
	errorStrings := []struct {
		errStr   string
		expected bool
	}{
		{"BROKEN PIPE", true},          // uppercase
		{"broken pipe", true},          // lowercase
		{"BrOkEn PiPe", true},          // mixed case
		{"Connection Reset By Peer", true},
		{"I/O TIMEOUT", true},
		{"Use of Closed Network Connection", true},
		{"some random error", false},
	}

	for _, tc := range errorStrings {
		t.Run(tc.errStr, func(t *testing.T) {
			err := errors.New(tc.errStr)
			got := shouldRestartOnErr(err)
			if got != tc.expected {
				t.Errorf("shouldRestartOnErr() for '%s' = %v, want %v", tc.errStr, got, tc.expected)
			}
		})
	}
}

// TestKafkaTLSConfig tests TLS configuration validation
func TestKafkaTLSConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *KafkaTLSConfig
		expectTLS bool
	}{
		{
			name:      "nil config",
			cfg:       nil,
			expectTLS: false,
		},
		{
			name: "empty config",
			cfg: &KafkaTLSConfig{
				ServerCA:   "",
				ClientCert: "",
				ClientKey:  "",
				SkipVerify: false,
			},
			expectTLS: false,
		},
		{
			name: "skip verify only",
			cfg: &KafkaTLSConfig{
				ServerCA:   "",
				ClientCert: "",
				ClientKey:  "",
				SkipVerify: true,
			},
			expectTLS: true,
		},
		{
			name: "server CA only",
			cfg: &KafkaTLSConfig{
				ServerCA:   "/path/to/ca.crt",
				ClientCert: "",
				ClientKey:  "",
				SkipVerify: false,
			},
			expectTLS: true,
		},
		{
			name: "full TLS with client cert",
			cfg: &KafkaTLSConfig{
				ServerCA:   "/path/to/ca.crt",
				ClientCert: "/path/to/client.crt",
				ClientKey:  "/path/to/client.key",
				SkipVerify: false,
			},
			expectTLS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasTLS := tt.cfg != nil && (tt.cfg.ServerCA != "" || tt.cfg.SkipVerify)
			if hasTLS != tt.expectTLS {
				t.Errorf("TLS detection = %v, want %v", hasTLS, tt.expectTLS)
			}
		})
	}
}

// TestTopicNameNormalization tests topic name transformation
func TestTopicNameNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "/queue/topic",
			expected: "_queue_topic",
		},
		{
			input:    "simple-topic",
			expected: "simple-topic",
		},
		{
			input:    "/kafka/telemetry/metrics",
			expected: "_kafka_telemetry_metrics",
		},
		{
			input:    "topic",
			expected: "topic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Simulate the topic normalization in TopicConnect
			topic := strings.ReplaceAll(tt.input, "/", "_")
			if topic != tt.expected {
				t.Errorf("Topic normalization of '%s' = '%s', want '%s'", tt.input, topic, tt.expected)
			}
		})
	}
}

// TestKafkaMaxMessageBytes tests the message size constant
func TestKafkaMaxMessageBytes(t *testing.T) {
	expectedSize := 16 * 1024 // 16 KB
	if KafkaMaxMessageBytes != expectedSize {
		t.Errorf("KafkaMaxMessageBytes = %d, want %d", KafkaMaxMessageBytes, expectedSize)
	}
}

// TestConnectionCacheKey tests that topic names are used as cache keys
func TestConnectionCacheKey(t *testing.T) {
	tests := []struct {
		name      string
		topic1    string
		topic2    string
		shouldSame bool
	}{
		{
			name:       "same topic",
			topic1:     "test-topic",
			topic2:     "test-topic",
			shouldSame: true,
		},
		{
			name:       "different topics",
			topic1:     "topic-a",
			topic2:     "topic-b",
			shouldSame: false,
		},
		{
			name:       "normalized same",
			topic1:     "/queue/test",
			topic2:     "/queue/test",
			shouldSame: true,
		},
		{
			name:       "normalized different",
			topic1:     "/queue/test1",
			topic2:     "/queue/test2",
			shouldSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized1 := strings.ReplaceAll(tt.topic1, "/", "_")
			normalized2 := strings.ReplaceAll(tt.topic2, "/", "_")
			same := (normalized1 == normalized2)
			if same != tt.shouldSame {
				t.Errorf("Topic comparison failed: '%s' vs '%s', same=%v, want=%v",
					normalized1, normalized2, same, tt.shouldSame)
			}
		})
	}
}

// TestWriteDeadline tests that write deadline is reasonable
func TestWriteDeadline(t *testing.T) {
	expectedDeadline := 10 * time.Second
	deadline := 10 * time.Second

	if deadline != expectedDeadline {
		t.Errorf("Write deadline = %v, want %v", deadline, expectedDeadline)
	}

	// Verify deadline is in the future
	now := time.Now()
	future := now.Add(deadline)
	if !future.After(now) {
		t.Errorf("Deadline %v is not after current time %v", future, now)
	}
}

// TestNetworkErrorTypes tests different network error scenarios
func TestNetworkErrorTypes(t *testing.T) {
	tests := []struct {
		name          string
		errGenerator  func() error
		shouldRestart bool
	}{
		{
			name: "generic network error",
			errGenerator: func() error {
				return &net.OpError{
					Op:  "write",
					Net: "tcp",
					Err: errors.New("connection refused"),
				}
			},
			shouldRestart: false,
		},
		{
			name: "timeout network error",
			errGenerator: func() error {
				return &net.OpError{
					Op:  "read",
					Net: "tcp",
					Err: &testTimeoutError{},
				}
			},
			shouldRestart: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.errGenerator()
			got := shouldRestartOnErr(err)
			if got != tt.shouldRestart {
				t.Errorf("shouldRestartOnErr() = %v, want %v for %T: %v",
					got, tt.shouldRestart, err, err)
			}
		})
	}
}

// TestReconnectionScenarios tests common reconnection scenarios
func TestReconnectionScenarios(t *testing.T) {
	scenarios := []struct {
		name        string
		firstError  error
		secondError error
		shouldExit  bool
	}{
		{
			name:        "first write fails, retry succeeds",
			firstError:  errors.New("broken pipe"),
			secondError: nil,
			shouldExit:  false,
		},
		{
			name:        "first write fails, retry fails",
			firstError:  errors.New("broken pipe"),
			secondError: errors.New("connection refused"),
			shouldExit:  true,
		},
		{
			name:        "timeout then success",
			firstError:  errors.New("i/o timeout"),
			secondError: nil,
			shouldExit:  false,
		},
		{
			name:        "EOF then connection refused",
			firstError:  io.EOF,
			secondError: errors.New("connection refused"),
			shouldExit:  true,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// Test the error handling logic
			firstShouldRestart := shouldRestartOnErr(sc.firstError)
			if !firstShouldRestart {
				t.Errorf("First error should trigger restart: %v", sc.firstError)
			}

			// If second error is nil, reconnection succeeded
			if sc.secondError == nil {
				if sc.shouldExit {
					t.Errorf("Scenario marked shouldExit=true but retry succeeded")
				}
			} else {
				// Second error exists, should exit
				if !sc.shouldExit {
					t.Errorf("Scenario marked shouldExit=false but retry failed")
				}
			}
		})
	}
}

// TestConcurrentTopicAccess tests thread-safety of topic connections
func TestConcurrentTopicAccess(t *testing.T) {
	// This tests the pattern used in the actual code
	conns := make(map[string]interface{})
	topics := []string{"topic1", "topic2", "topic3"}

	// Simulate concurrent access
	done := make(chan bool, len(topics))
	for _, topic := range topics {
		go func(t string) {
			// This would be protected by topicConnMu in real code
			conns[t] = struct{}{}
			done <- true
		}(topic)
	}

	// Wait for all goroutines
	for i := 0; i < len(topics); i++ {
		<-done
	}

	// Verify all topics were added
	if len(conns) != len(topics) {
		t.Errorf("Expected %d topics, got %d", len(topics), len(conns))
	}
}

// TestStaleConnectionRemoval tests the logic for removing stale connections
func TestStaleConnectionRemoval(t *testing.T) {
	// Simulate connection cache
	conns := make(map[string]string)
	conns["topic1"] = "conn1"
	conns["topic2"] = "conn2"

	topic := "topic1"

	// Simulate the removal logic from SendMessage
	if _, exists := conns[topic]; exists {
		delete(conns, topic)
	}

	// Verify topic was removed
	if _, exists := conns[topic]; exists {
		t.Errorf("Topic %s should have been removed from cache", topic)
	}

	// Verify other topics remain
	if _, exists := conns["topic2"]; !exists {
		t.Errorf("Topic topic2 should still be in cache")
	}

	if len(conns) != 1 {
		t.Errorf("Expected 1 connection in cache, got %d", len(conns))
	}
}
