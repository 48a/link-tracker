package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/IBM/sarama"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/agent"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/agentkafka"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/kafkaconfig"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := setEnv(); err != nil {
		logger.Error("set env", slog.String("error", err.Error()))
		os.Exit(1)
	}

	cfg, err := newConfigFromEnv()
	if err != nil {
		logger.Error("new config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	processorCfg := agent.Config{
		StopWords:       cfg.getStopWords(),
		ExcludedAuthors: cfg.getExcludedAuthors(),
		MinLength:       cfg.MinLength,
		SumThreshold:    cfg.SumThreshold,
	}
	processor := agent.NewProcessor(processorCfg)

	saramaCfg := kafkaconfig.NewConfig(cfg.KafkaUser, cfg.KafkaPassword)

	producer, err := sarama.NewSyncProducer(cfg.KafkaBroker, saramaCfg)
	if err != nil {
		logger.Error("create producer", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = producer.Close() }()

	consumerGroup, err := sarama.NewConsumerGroup(cfg.KafkaBroker, cfg.KafkaConsumerGroup, saramaCfg)
	if err != nil {
		logger.Error("create consumer group", slog.String("error", err.Error()))
		return
	}
	defer func() { _ = consumerGroup.Close() }()

	worker := agentkafka.NewWorker(processor, producer, cfg.OutputTopic, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			if err = consumerGroup.Consume(ctx, []string{cfg.InputTopic}, worker); err != nil {
				logger.Error("consume", slog.String("error", err.Error()))
				return
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()

	logger.Info("AI Agent Service started")

	<-sigterm
	logger.Info("shutting down AI Agent Service")
	cancel()
}
