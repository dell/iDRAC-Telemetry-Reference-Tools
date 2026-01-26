package wire

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestEncodeDecodeEnvelope(t *testing.T) {
	payload := map[string]string{"foo": "bar"}
	env, err := NewEnvelope("wire.test", payload)
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}

	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope() error = %v", err)
	}

	decoded, err := DecodeEnvelope(data)
	if err != nil {
		t.Fatalf("DecodeEnvelope() error = %v", err)
	}

	if decoded.Type != env.Type {
		t.Fatalf("DecodeEnvelope() type = %q, want %q", decoded.Type, env.Type)
	}

	var gotPayload map[string]string
	if err := json.Unmarshal(decoded.Payload, &gotPayload); err != nil {
		t.Fatalf("DecodeEnvelope() payload unmarshal error = %v", err)
	}
	if gotPayload["foo"] != "bar" {
		t.Fatalf("DecodeEnvelope() payload = %v, want %v", gotPayload, payload)
	}
}

func TestDecodeEnvelopeInvalidJSON(t *testing.T) {
	_, err := DecodeEnvelope([]byte("not-json"))
	if err == nil {
		t.Fatalf("DecodeEnvelope() expected error, got nil")
	}
}

func TestEncodePayload(t *testing.T) {
	t.Run("nil payload", func(t *testing.T) {
		raw, err := EncodePayload(nil)
		if err != nil {
			t.Fatalf("EncodePayload(nil) error = %v", err)
		}
		if raw != nil {
			t.Fatalf("EncodePayload(nil) = %v, want nil", raw)
		}
	})

	t.Run("unsupported payload", func(t *testing.T) {
		_, err := EncodePayload(make(chan int))
		if err == nil {
			t.Fatalf("EncodePayload(chan) expected error, got nil")
		}
	})
}

func TestNewRequest(t *testing.T) {
	payload := map[string]string{"id": "1"}
	env, err := NewRequest("wire.request", payload, "/queue/replies")
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if env.ReplyTo != "/queue/replies" {
		t.Fatalf("NewRequest() replyTo = %q, want %q", env.ReplyTo, "/queue/replies")
	}
	if env.CorrelationID == "" {
		t.Fatalf("NewRequest() correlation ID is empty")
	}
}

func TestReplyOK(t *testing.T) {
	req := Envelope{Type: "wire.request", CorrelationID: "corr-1"}
	reply, err := ReplyOK(req, map[string]string{"status": "ok"})
	if err != nil {
		t.Fatalf("ReplyOK() error = %v", err)
	}
	if reply.Status != StatusOK {
		t.Fatalf("ReplyOK() status = %q, want %q", reply.Status, StatusOK)
	}
	if reply.CorrelationID != req.CorrelationID {
		t.Fatalf("ReplyOK() correlation ID = %q, want %q", reply.CorrelationID, req.CorrelationID)
	}
}

func TestReplyErr(t *testing.T) {
	req := Envelope{Type: "wire.request", CorrelationID: "corr-2"}
	reply := ReplyErr(req, errors.New("boom"))
	if reply.Status != StatusError {
		t.Fatalf("ReplyErr() status = %q, want %q", reply.Status, StatusError)
	}
	if reply.Error != "boom" {
		t.Fatalf("ReplyErr() error = %q, want %q", reply.Error, "boom")
	}
}

func TestNewCorrelationID(t *testing.T) {
	id := NewCorrelationID("prefix")
	if !strings.HasPrefix(id, "prefix-") {
		t.Fatalf("NewCorrelationID() = %q, want prefix", id)
	}
}

func TestNewReplyQueue(t *testing.T) {
	queue := NewReplyQueue("/queue/replies.", "client")
	if !strings.HasPrefix(queue, "/queue/replies.client-") {
		t.Fatalf("NewReplyQueue() = %q, want prefix %q", queue, "/queue/replies.client-")
	}

	queue = NewReplyQueue("", "client")
	if !strings.HasPrefix(queue, DefaultReplyQueuePrefix+"client-") {
		t.Fatalf("NewReplyQueue() = %q, want prefix %q", queue, DefaultReplyQueuePrefix+"client-")
	}
}
