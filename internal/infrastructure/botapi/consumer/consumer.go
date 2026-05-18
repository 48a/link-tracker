package consumer

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/kafkaconfig"
)

type consumer struct {
	cg    sarama.ConsumerGroup
	h     *handler
	topic string
}

func NewConsumer(h *handler, kafkaBroker []string, kafkaUser, kafkaPassword, kafkaConsumerGroup, topic string) (consumer, error) {
	cfg := kafkaconfig.NewConfig(kafkaUser, kafkaPassword)

	consumerGroup, err := sarama.NewConsumerGroup(
		kafkaBroker,
		kafkaConsumerGroup,
		cfg,
	)
	if err != nil {
		return consumer{}, fmt.Errorf("create consumer: %w", err)
	}

	return consumer{cg: consumerGroup, h: h, topic: topic}, nil
}

func (c consumer) Serve(ctx context.Context) {
	for {
		if err := c.cg.Consume(ctx, []string{c.topic}, c.h); err != nil {
			fmt.Println("Consume error", err)
			return
		}

		if ctx.Err() != nil {
			return
		}
	}
}

func (c consumer) Close() error {
	return c.cg.Close()
}
