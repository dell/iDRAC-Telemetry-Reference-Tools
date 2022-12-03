// Licensed to You under the Apache License, Version 2.0.

package kafka

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	kafka "github.com/segmentio/kafka-go"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

const KafkaMaxMessageBytes = 16 * 1024

type KafkaMessagebus struct {
	conns map[string]*kafka.Conn // cache of connections made at first read/write message, mapped by topic name
	addr  string
	ctx   context.Context
}

type KafkaSubscription struct {
}

func NewKafkaMessageBus(host string, port int) (messagebus.Messagebus, error) {
	ret := new(KafkaMessagebus)
	ret.addr = fmt.Sprintf("%s:%d", host, port)

	// make sure we can connect, real connection made at first read/write
	conn, err := kafka.Dial("tcp", ret.addr)
	if err != nil {
		return nil, err
	}
	conn.Close()

	ret.conns = map[string]*kafka.Conn{}
	intRet := messagebus.Messagebus(ret)
	return intRet, nil
}

func NewKafkaMessageBusFromConn(conn *kafka.Conn, topic string) (messagebus.Messagebus, error) {
	ret := new(KafkaMessagebus)
	ret.conns[topic] = conn
	ret.ctx = context.Background()
	intRet := messagebus.Messagebus(ret)
	return intRet, nil
}

func (m *KafkaMessagebus) TopicConnect(queue string) (*kafka.Conn, error) {
	topic := strings.ReplaceAll(queue, "/", "_")
	kconn, ok := m.conns[topic]
	if !ok || kconn == nil {
		conn, err := kafka.DialLeader(context.Background(), "tcp", m.addr, topic, 0)
		if err != nil || conn == nil {
			log.Println("kafka.DialLeader: could not connect ", err)
			return nil, err
		}
		m.conns[topic] = conn
		kconn = conn
	}
	return kconn, nil
}

func (m *KafkaMessagebus) SendMessage(message []byte, queue string) error {

	kconn, err := m.TopicConnect(queue)
	if err != nil || kconn == nil {
		return err
	}
	kconn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err = kconn.WriteMessages(kafka.Message{Value: message})
	if err != nil {
		log.Println("failed to write messages:", queue, err)
	}
	return err
}

func (m *KafkaMessagebus) SendMessageWithHeaders(message []byte, queue string, headers map[string]string) error {
	var hdr kafka.Header
	var hdrs []kafka.Header
	for key, value := range headers {
		hdr.Key = key
		hdr.Value = []byte(value)
		hdrs = append(hdrs, hdr)
	}
	kconn, err := m.TopicConnect(queue)
	if err != nil {
		return err
	}
	kconn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err = kconn.WriteMessages(kafka.Message{Value: message, Headers: hdrs})
	if err != nil {
		log.Println("failed to write messages:", queue, err)
	}

	return err
}

func (m *KafkaMessagebus) ReceiveMessage(message chan<- string, queue string) (messagebus.Subscription, error) {
	kconn, err := m.TopicConnect(queue)
	if err != nil || kconn == nil {
		return nil, err
	}
	go m.RecieveLoop(kconn, message, queue)

	mySub := new(KafkaSubscription)
	return messagebus.Subscription(mySub), nil
}

func (m *KafkaMessagebus) RecieveLoop(conn *kafka.Conn, message chan<- string, queue string) {
	defer func() {
		_, cancel := context.WithTimeout(m.ctx, 1*time.Second)
		conn.Close()
		cancel()
	}()

	for {
		// Receive next message
		msg, err := conn.ReadMessage(KafkaMaxMessageBytes)
		if err != nil {
			log.Println("failed to read message:", err)
			break
		}
		message <- string(msg.Value)
	}
}

func (m *KafkaMessagebus) Close() error {
	var err error
	for _, conn := range m.conns {
		err1 := conn.Close()
		if err1 != nil {
			err = err1
		}
	}
	return err
}

func (m *KafkaSubscription) Close() error {
	return nil
}
