package mock

import (
	"context"

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
	Messages []string
}

func (mb *MockMessageBus) SendMessage(message []byte, queue string) error {
	mb.Messages = append(mb.Messages, string(message))
	return nil
}

func (mb *MockMessageBus) ReceiveMessage(message chan<- string, queue string) (messagebus.Subscription, error) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if len(mb.Messages) == 0 {
					cancel()
					return
				}
				message <- mb.Messages[0]
				mb.Messages = mb.Messages[1:]
			}
		}
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
