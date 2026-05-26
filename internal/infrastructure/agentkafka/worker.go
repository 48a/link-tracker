package agentkafka

import (
	"encoding/json"
	"log/slog"

	"github.com/IBM/sarama"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/agent"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type Worker struct {
	processor   *agent.Processor
	producer    sarama.SyncProducer
	outputTopic string
	logger      *slog.Logger
}

func NewWorker(processor *agent.Processor, producer sarama.SyncProducer, outputTopic string, logger *slog.Logger) *Worker {
	return &Worker{
		processor:   processor,
		producer:    producer,
		outputTopic: outputTopic,
		logger:      logger,
	}
}

func (w *Worker) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (w *Worker) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (w *Worker) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		var raw domain.RawUpdate
		if err := json.Unmarshal(message.Value, &raw); err != nil {
			w.logger.Error("unmarshal message", slog.String("error", err.Error()), slog.String("payload", string(message.Value)))
			session.MarkMessage(message, "")
			continue
		}

		processed := w.processor.Process(raw)
		if processed != nil {
			outBytes, err := json.Marshal(processed)
			if err != nil {
				w.logger.Error("marshal processed update", slog.String("error", err.Error()))
				session.MarkMessage(message, "")
				continue
			}

			outMsg := &sarama.ProducerMessage{
				Topic: w.outputTopic,
				Value: sarama.ByteEncoder(outBytes),
			}

			if _, _, err := w.producer.SendMessage(outMsg); err != nil {
				w.logger.Error("send processed message", slog.String("error", err.Error()))
				return err
			}
			w.logger.Info("processed and forwarded update", slog.Int64("id", processed.ID))
		} else {
			w.logger.Info("update filtered out", slog.Int64("id", raw.ID))
		}

		session.MarkMessage(message, "")
	}
	return nil
}
