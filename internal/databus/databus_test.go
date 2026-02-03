package databus

import (
	"encoding/json"
	"testing"

	mockbus "github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/mock"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/wire"
)

func decodeEnvelope(t *testing.T, msg string) wire.Envelope {
	t.Helper()
	var env wire.Envelope
	if err := json.Unmarshal([]byte(msg), &env); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}
	return env
}

func TestHandleSubscribeEnvelopeAddsQueue(t *testing.T) {
	bus := mockbus.NewMockMessageBus()
	svc := NewDataBusService(bus)

	env := wire.Envelope{Type: SUBSCRIBE, ReplyTo: "/queue/foo"}
	svc.HandleSubscribeEnvelope(env)

	if got := len(svc.Recievers); got != 1 {
		t.Fatalf("expected 1 receiver, got %d", got)
	}
	if svc.Recievers[0] != "/queue/foo" {
		t.Fatalf("unexpected receiver stored: %s", svc.Recievers[0])
	}
}

func TestHandleSubscribeEnvelopeSkipsDuplicates(t *testing.T) {
	bus := mockbus.NewMockMessageBus()
	svc := NewDataBusService(bus)
	svc.Recievers = []string{"/queue/foo"}

	env := wire.Envelope{Type: SUBSCRIBE, ReplyTo: "/queue/foo"}
	svc.HandleSubscribeEnvelope(env)

	if got := len(svc.Recievers); got != 1 {
		t.Fatalf("expected receivers length to remain 1, got %d", got)
	}
}

func TestSendGroupToQueueSendsDataGroupEnvelope(t *testing.T) {
	bus := mockbus.NewMockMessageBus()
	svc := NewDataBusService(bus)

	group := DataGroup{ID: "group-1"}
	if err := svc.SendGroupToQueue(group, "/queue/a"); err != nil {
		t.Fatalf("SendGroupToQueue returned error: %v", err)
	}

	msgs := bus.GetMessages("/queue/a")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	env := decodeEnvelope(t, msgs[0])
	if env.Type != DATAGROUP {
		t.Fatalf("expected envelope type %s, got %s", DATAGROUP, env.Type)
	}
}

func TestSendGroupFansOutToReceivers(t *testing.T) {
	bus := mockbus.NewMockMessageBus()
	svc := NewDataBusService(bus)
	svc.Recievers = []string{"/queue/a", "/queue/b"}

	svc.SendGroup(DataGroup{ID: "group-1"})

	if len(bus.GetMessages("/queue/a")) != 1 || len(bus.GetMessages("/queue/b")) != 1 {
		t.Fatalf("expected each receiver queue to get a message")
	}
}

func TestDataBusClientGetSetsReplyQueue(t *testing.T) {
	bus := mockbus.NewMockMessageBus()
	client := NewDataBusClient(bus, "tester")

	replyQueue := "/queue/custom-reply"
	if err := client.Get(replyQueue); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	msgs := bus.GetMessages(CommandQueue)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	env := decodeEnvelope(t, msgs[0])
	if env.Type != GET {
		t.Fatalf("expected envelope type %s, got %s", GET, env.Type)
	}
	if env.ReplyTo != replyQueue {
		t.Fatalf("expected ReplyTo %s, got %s", replyQueue, env.ReplyTo)
	}
}

func TestDataBusClientSubscribeSetsReplyQueue(t *testing.T) {
	bus := mockbus.NewMockMessageBus()
	client := NewDataBusClient(bus, "tester")

	replyQueue := "/queue/custom-reply"
	if err := client.Subscribe(replyQueue); err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	msgs := bus.GetMessages(CommandQueue)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	env := decodeEnvelope(t, msgs[0])
	if env.Type != SUBSCRIBE {
		t.Fatalf("expected envelope type %s, got %s", SUBSCRIBE, env.Type)
	}
	if env.ReplyTo != replyQueue {
		t.Fatalf("expected ReplyTo %s, got %s", replyQueue, env.ReplyTo)
	}
}
