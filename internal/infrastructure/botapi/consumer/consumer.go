package consumer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/IBM/sarama"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/kafkaconfig"
)

type Consumer struct {
	cg     sarama.ConsumerGroup
	h      *Handler
	topic  string
	logger *slog.Logger
}

func NewConsumer(h *Handler, kafkaBroker []string, kafkaUser, kafkaPassword, kafkaConsumerGroup, topic string, logger *slog.Logger) (Consumer, error) {
	cfg := kafkaconfig.NewConfig(kafkaUser, kafkaPassword)

	consumerGroup, err := sarama.NewConsumerGroup(
		kafkaBroker,
		kafkaConsumerGroup,
		cfg,
	)
	if err != nil {
		return Consumer{}, fmt.Errorf("create consumer: %w", err)
	}

	return Consumer{cg: consumerGroup, h: h, topic: topic}, nil
}

func (c Consumer) Serve(ctx context.Context) {
	defer func() {
		if err := c.cg.Close(); err != nil {
			c.logger.Error("close consumer", slog.String("error", err.Error()))
		}
	}()

	for {
		if err := c.cg.Consume(ctx, []string{c.topic}, c.h); err != nil {
			c.logger.Error("consume", slog.String("error", err.Error()))
			return
		}

		if ctx.Err() != nil {
			return
		}
	}
}

func (c Consumer) Close() error {
	return c.cg.Close()
}
