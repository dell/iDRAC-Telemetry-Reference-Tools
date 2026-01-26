package wire

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	StatusOK    = "ok"
	StatusError = "error"
)

const DefaultReplyQueuePrefix = "/queue/reply."

type Envelope struct {
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlationId,omitempty"`
	ReplyTo       string          `json:"replyTo,omitempty"`
	Status        string          `json:"status,omitempty"`
	Error         string          `json:"error,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

func EncodeEnvelope(env Envelope) ([]byte, error) {
	return json.Marshal(env)
}

func DecodeEnvelope(data []byte) (Envelope, error) {
	var env Envelope
	err := json.Unmarshal(data, &env)
	return env, err
}

func EncodePayload(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func DecodePayload(payload json.RawMessage, v any) error {
	if v == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, v)
}

func NewEnvelope(msgType string, payload any) (Envelope, error) {
	raw, err := EncodePayload(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Type: msgType, Payload: raw}, nil
}

func NewRequest(msgType string, payload any, replyTo string) (Envelope, error) {
	env, err := NewEnvelope(msgType, payload)
	if err != nil {
		return Envelope{}, err
	}
	env.ReplyTo = replyTo
	env.CorrelationID = NewCorrelationID("req")
	return env, nil
}

func ReplyOK(req Envelope, payload any) (Envelope, error) {
	env, err := NewEnvelope(req.Type, payload)
	if err != nil {
		return Envelope{}, err
	}
	env.CorrelationID = req.CorrelationID
	env.Status = StatusOK
	return env, nil
}

func NewReply(correlationID string, msgType string, payload any) (Envelope, error) {
	payloadData, err := EncodePayload(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		Type:          msgType,
		CorrelationID: correlationID,
		Status:        StatusOK,
		Payload:       payloadData,
	}, nil
}

func NewErrorReply(correlationID string, errorMsg string) Envelope {
	return Envelope{
		Type:          "",
		CorrelationID: correlationID,
		Status:        StatusError,
		Error:         errorMsg,
	}
}

func ReplyErr(req Envelope, err error) Envelope {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return Envelope{
		Type:          req.Type,
		CorrelationID: req.CorrelationID,
		Status:        StatusError,
		Error:         msg,
	}
}

func NewCorrelationID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		if prefix == "" {
			return fmt.Sprintf("id-%d", time.Now().UnixNano())
		}
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	id := hex.EncodeToString(b)
	if prefix == "" {
		return id
	}
	return prefix + "-" + id
}

func NewReplyQueue(queuePrefix, clientPrefix string) string {
	id := NewCorrelationID("")
	if queuePrefix == "" {
		queuePrefix = DefaultReplyQueuePrefix
	}
	if clientPrefix == "" {
		return queuePrefix + id
	}
	return fmt.Sprintf("%s%s-%s", queuePrefix, clientPrefix, id)
}
