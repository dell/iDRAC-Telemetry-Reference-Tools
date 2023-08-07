// Licensed to You under the Apache License, Version 2.0.

package kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	kafka "github.com/segmentio/kafka-go"

	"github.com/dell/iDRAC-Telemetry-Reference-Tools/internal/messagebus"
)

const KafkaMaxMessageBytes = 16 * 1024

type KafkaMessagebus struct {
	conns       map[string]*kafka.Conn // cache of connections made at first read/write message, mapped by topic name
	addr        string
	ctx         context.Context
	dialer      *kafka.Dialer
	topicConnMu sync.RWMutex
}

type KafkaSubscription struct {
}

type KafkaTLSConfig struct {
	ServerCA   string
	ClientCert string
	ClientKey  string
	SkipVerify bool // skip hostname check
}

func NewKafkaMessageBus(host string, port int, topic string, tlsCfg *KafkaTLSConfig) (messagebus.Messagebus, error) {
	ret := new(KafkaMessagebus)
	ret.addr = fmt.Sprintf("%s:%d", host, port)
	ret.ctx = context.Background()
	ret.conns = map[string]*kafka.Conn{}
	ret.topicConnMu = sync.RWMutex{}

	dialer := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
	}

	// TLS
	if tlsCfg != nil && tlsCfg.ServerCA != "" {
		ca, err := os.ReadFile(tlsCfg.ServerCA)
		if err != nil {
			log.Println("failed to load server CA cert", err)
			return nil, err
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(ca); !ok {
			log.Println("Unable to apppend cert to pool")
		}

		config := tls.Config{
			RootCAs:            pool,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: tlsCfg.SkipVerify,
		}

		// Client Authentication - optional
		if tlsCfg.ClientCert != "" && tlsCfg.ClientKey != "" {
			ccrt, err := os.ReadFile(tlsCfg.ClientCert)
			if err != nil {
				log.Println("failed to load client cert", err)
				return nil, err
			}
			ckey, err := os.ReadFile(tlsCfg.ClientKey)
			if err != nil {
				log.Println("failed to load client key", err)
				return nil, err
			}
			cert, err := tls.X509KeyPair(ccrt, ckey)
			if err != nil {
				log.Println("X509KeyPair error ", err)
				return nil, err
			}
			config.Certificates = []tls.Certificate{cert}
		}
		dialer.TLS = &config
	}

	ret.dialer = dialer
	if topic != "" {
		conn, err := dialer.DialLeader(context.Background(), "tcp", ret.addr, topic, 0)
		if err != nil || conn == nil {
			log.Println("kafka.DialLeader: could not connect ", err)
			return nil, err
		}
		ret.topicConnMu.Lock()
		ret.conns[topic] = conn
		ret.topicConnMu.Unlock()
	}

	return messagebus.Messagebus(ret), nil
}

func NewKafkaMessageBusFromConn(conn *kafka.Conn, topic string) (messagebus.Messagebus, error) {
	ret := new(KafkaMessagebus)
	ret.conns[topic] = conn
	ret.topicConnMu = sync.RWMutex{}
	ret.ctx = context.Background()
	intRet := messagebus.Messagebus(ret)
	return intRet, nil
}

func (m *KafkaMessagebus) TopicConnect(queue string) (*kafka.Conn, error) {
	topic := strings.ReplaceAll(queue, "/", "_")
	m.topicConnMu.RLock()
	kconn, ok := m.conns[topic]
	m.topicConnMu.RUnlock()
	if !ok || kconn == nil {
		conn, err := m.dialer.DialLeader(context.Background(), "tcp", m.addr, topic, 0)
		if err != nil || conn == nil {
			log.Println("kafka.DialLeader: could not connect ", err)
			return nil, err
		}
		m.topicConnMu.Lock()
		m.conns[topic] = conn
		m.topicConnMu.Unlock()
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
	m.topicConnMu.Lock()
	defer m.topicConnMu.Unlock()
	for topic, conn := range m.conns {
		err1 := conn.Close()
		if err1 != nil {
			err = err1
		}
		delete(m.conns, topic)
	}
	return err
}

func (m *KafkaSubscription) Close() error {
	return nil
}
