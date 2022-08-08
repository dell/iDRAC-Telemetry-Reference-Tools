// Licensed to You under the Apache License, Version 2.0.

package amqp

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"pack.ag/amqp"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

type AmqpMessagebus struct {
	client  *amqp.Client
	ctx     context.Context
	session *amqp.Session
}

func NewAmqpMessageBus(host string, port int) (messagebus.Messagebus, error) {
	ret := new(AmqpMessagebus)

	amqpAddress := fmt.Sprintf("amqp://%s:%d", host, port)

	client, err := amqp.Dial(amqpAddress, amqp.ConnSASLAnonymous())
	if err != nil {
		return nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	ret.client = client
	ret.session = session
	ret.ctx = context.Background()

	intRet := messagebus.Messagebus(ret)
	return intRet, nil
}

func (m *AmqpMessagebus) SendMessage(message []byte, queue string) error {
	sender, err := m.session.NewSender(
		amqp.LinkTargetAddress(queue),
	)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	err = sender.Send(ctx, amqp.NewMessage(message))
	if err != nil {
		cancel()
		return err
	}

	sender.Close(ctx)
	cancel()
	return nil
}

func (m *AmqpMessagebus) SendMessageWithHeaders(message []byte, queue string, headers map[string]string) error {
	sender, err := m.session.NewSender(
		amqp.LinkTargetAddress(queue),
	)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	msg := amqp.NewMessage(message)
	ttl, ok := headers["expires"]
	if ok {
		ittl, _ := strconv.Atoi(ttl)
		msg.Header.TTL = time.Duration(ittl) * time.Millisecond
	}
	err = sender.Send(ctx, msg)
	if err != nil {
		cancel()
		return err
	}

	sender.Close(ctx)
	cancel()
	return nil
}

func (m *AmqpMessagebus) ReceiveMessage(message chan<- string, queue string) (messagebus.Subscription, error) {
	receiver, err := m.session.NewReceiver(
		amqp.LinkSourceAddress(queue),
		amqp.LinkCredit(10),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(m.ctx, 1*time.Second)
		receiver.Close(ctx)
		cancel()
	}()

	for {
		// Receive next message
		msg, err := receiver.Receive(m.ctx)
		if err != nil {
			return nil, err
		}

		// Accept message
		err = msg.Accept()
		if err != nil {
			return nil, err
		}
		message <- string(msg.GetData())
	}
}

func (m *AmqpMessagebus) Close() error {
	return m.client.Close()
}
