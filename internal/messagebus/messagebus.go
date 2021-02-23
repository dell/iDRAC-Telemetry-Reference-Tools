// Licensed to You under the Apache License, Version 2.0.

package messagebus

type Subscription interface {
	Close() error
}

type Messagebus interface {
	SendMessage(message []byte, queue string) error
	SendMessageWithHeaders(message []byte, queue string, headers map[string]string) error
	ReceiveMessage(message chan<- string, queue string) (Subscription, error)
	Close() error
}
