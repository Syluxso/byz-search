package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type KafkaPublisher struct {
	client  *kgo.Client
	topic   string
	enabled bool
}

func NewKafkaPublisher(brokers []string, enabled bool, topic string) (*KafkaPublisher, error) {
	if !enabled {
		return &KafkaPublisher{enabled: false}, nil
	}
	if topic == "" {
		topic = "byz.search.query"
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ClientID("byz-search"),
		kgo.ProducerBatchMaxBytes(1_000_000),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProduceRequestTimeout(2*time.Second),
		kgo.RecordRetries(3),
	)
	if err != nil {
		return nil, err
	}
	return &KafkaPublisher{client: client, topic: topic, enabled: true}, nil
}

func (p *KafkaPublisher) Close() {
	if p != nil && p.client != nil {
		p.client.Close()
	}
}

func (p *KafkaPublisher) PublishQuery(ev SearchQueryEvent) {
	if p == nil || !p.enabled || p.client == nil {
		return
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	key := ev.OrganizationID
	if key == "" {
		key = ev.EventID
	}
	rec := &kgo.Record{Topic: p.topic, Key: []byte(key), Value: b}
	p.client.Produce(context.Background(), rec, func(_ *kgo.Record, err error) {
		if err != nil {
			log.Printf("kafka produce %s: %v", p.topic, err)
		}
	})
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
