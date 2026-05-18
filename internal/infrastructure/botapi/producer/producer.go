package producer

import (
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/kafkaconfig"
)

type producer struct {
	prod  sarama.SyncProducer
	topic string
}

func NewProducer(brokerAddrs []string, kafkaUser, kafkaPassword, kafkaTopic string) (producer, error) {
	cfg := kafkaconfig.NewConfig(kafkaUser, kafkaPassword)

	p, err := sarama.NewSyncProducer(brokerAddrs, cfg)
	if err != nil {
		return producer{}, fmt.Errorf("new sync producer: %w", err)
	}

	return producer{prod: p, topic: kafkaTopic}, nil
}

func (p producer) SendUpdate(linkUpdate botapi.LinkUpdate) error {
	data, err := json.Marshal(linkUpdate)
	if err != nil {
		return fmt.Errorf("marshal linkUpdate: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: p.topic,
		Value: sarama.StringEncoder(data),
	}

	partition, offset, err := p.prod.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	fmt.Printf("> Message %q sent. Partition=%d. Offset=%d\n", string(data), partition, offset)
	return nil
}

func (p producer) Close() error {
	return p.prod.Close()
}
