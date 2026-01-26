package wire

import (
	"encoding/json"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/wire"
)

// Envelope is a standard message envelope for request/reply and broadcast messaging
type Envelope = wire.Envelope

// NewEnvelope creates a new envelope with the given type and payload
func NewEnvelope(msgType string, payload any) (Envelope, error) {
	return wire.NewEnvelope(msgType, payload)
}

// DecodePayload decodes the envelope payload into the target type
func DecodePayload(payload json.RawMessage, target any) error {
	return wire.DecodePayload(payload, target)
}
