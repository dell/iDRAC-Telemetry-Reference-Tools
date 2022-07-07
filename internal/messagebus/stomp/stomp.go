// Licensed to You under the Apache License, Version 2.0.

package stomp

import (
	"fmt"
	"log"
	"time"

	"github.com/go-stomp/stomp"
	"github.com/go-stomp/stomp/frame"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

type StompMessagebus struct {
	conn *stomp.Conn
	subs []*stomp.Subscription
}

type StompSubscription struct {
	sub *stomp.Subscription
}

func NewStompMessageBus(host string, port int) (messagebus.Messagebus, error) {
	ret := new(StompMessagebus)

	stompAddress := fmt.Sprintf("%s:%d", host, port)

	conn, err := stomp.Dial("tcp", stompAddress, stomp.ConnOpt.HeartBeat(time.Minute, 0))
	if err != nil {
		return nil, err
	}

	ret.conn = conn

	intRet := messagebus.Messagebus(ret)
	return intRet, nil
}

func NewStompMessageBusFromConn(conn *stomp.Conn) (messagebus.Messagebus, error) {
	ret := new(StompMessagebus)
	ret.conn = conn

	intRet := messagebus.Messagebus(ret)
	return intRet, nil
}

func (m *StompMessagebus) SendMessage(message []byte, queue string) error {
	return m.conn.Send(queue, "text/plain", message)
}

func (m *StompMessagebus) SendMessageWithHeaders(message []byte, queue string, headers map[string]string) error {
	return m.conn.Send(queue, "text/plain", message, func(frame *frame.Frame) error {
		for key, value := range headers {
			frame.Header.Set(key, value)
		}
		return nil
	})
}

func (m *StompMessagebus) ReceiveMessage(message chan<- string, queue string) (messagebus.Subscription, error) {
	sub, err := m.conn.Subscribe(queue, stomp.AckClient)
	if err != nil {
		return nil, err
	}

	m.subs = append(m.subs, sub)

	go m.RecieveLoop(sub, message)
	mySub := new(StompSubscription)
	mySub.sub = sub
	return messagebus.Subscription(mySub), nil
}

func (m *StompMessagebus) RecieveLoop(sub *stomp.Subscription, message chan<- string) {
	for {
		msg := <-sub.C
		if msg == nil {
			break
		} else if msg.Err != nil {
			//This can timeout... just keep going...
			continue
		}
		message <- string(msg.Body)
		err := m.conn.Ack(msg)
		if err != nil {
			log.Printf("ACK failed! %v", err)
		}
	}
}

func (m *StompMessagebus) Close() error {
	for _, sub := range m.subs {
		err := sub.Unsubscribe()
		if err != nil {
			log.Printf("Failed to unsubscribe %v", err)
		}
	}
	return m.conn.Disconnect()
}

func (m *StompSubscription) Close() error {
	return m.sub.Unsubscribe()
}
