// Licensed to You under the Apache License, Version 2.0.

package kafka

import (
	"context"
	"fmt"
	"log"
	"time"

	kafka "github.com/segmentio/kafka-go"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

const KafkaMaxMessageBytes = 1024

type KafkaMessagebus struct {
	conn      *kafka.Conn
	topic     string
	partition int
	ctx       context.Context
	//subs []*kafka.Subscription
}

//type KafkaSubscription struct {
//	sub *kafka.Subscription
//}

func NewKafkaMessageBus(host string, port int, topic string, partition int) (messagebus.Messagebus, error) {
	ret := new(KafkaMessagebus)

	kafkaAddress := fmt.Sprintf("%s:%d", host, port)

	conn, err := kafka.DialLeader(context.Background(), "tcp", kafkaAddress, topic, partition)
	if err != nil {
		return nil, err
	}

	ret.conn = conn

	intRet := messagebus.Messagebus(ret)
	return intRet, nil
}

func NewKafkaMessageBusFromConn(conn *kafka.Conn) (messagebus.Messagebus, error) {
	ret := new(KafkaMessagebus)
	ret.conn = conn
	ret.ctx = context.Background()
	intRet := messagebus.Messagebus(ret)
	return intRet, nil
}

func (m *KafkaMessagebus) SendMessage(message []byte, queue string) error {
	m.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := m.conn.WriteMessages(kafka.Message{Value: message})
	if err != nil {
		log.Println("failed to write messages:", err)
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
	m.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := m.conn.WriteMessages(kafka.Message{Value: message, Headers: hdrs})
	if err != nil {
		log.Println("failed to write messages:", err)
	}
	return err
}

func (m *KafkaMessagebus) ReceiveMessage(message chan<- string, queue string) (messagebus.Subscription, error) {
	defer func() {
		_, cancel := context.WithTimeout(m.ctx, 1*time.Second)
		m.conn.Close()
		cancel()
	}()

	for {
		// Receive next message
		msg, err := m.conn.ReadMessage(KafkaMaxMessageBytes)
		if err != nil {
			return nil, err
		}

		message <- string(msg.Value)
	}
}

/*
	func (m *KafkaMessagebus) RecieveLoop(sub *kafka.Subscription, message chan<- string) {
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
*/
func (m *KafkaMessagebus) Close() error {
	//for _, sub := range m.subs {
	//	err := sub.Unsubscribe()
	//	if err != nil {
	//		log.Printf("Failed to unsubscribe %v", err)
	//	}
	//}
	return m.conn.Close()
}

//func (m *KafkaSubscription) Close() error {
//	return m.sub.Unsubscribe()
//}
