package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
)

type config struct {
	KafkaUser          string   `env:"KAFKA_USER"`
	KafkaPassword      string   `env:"KAFKA_PASSWORD"`
	KafkaBroker        []string `env:"KAFKA_BROKER"`
	KafkaConsumerGroup string   `env:"KAFKA_CONSUMER_GROUP"`
	KafkaTopic         string   `env:"KAFKA_TOPIC"`
	Token              string   `env:"APP_TELEGRAM_TOKEN"`
}

func setEnv() error {
	return errors.Join(
		os.Setenv("KAFKA_USER", getEnv("KAFKA_USER", "user1")),
		os.Setenv("KAFKA_PASSWORD", getEnv("KAFKA_PASSWORD", "29665")),
		os.Setenv("KAFKA_BROKER", getEnv("KAFKA_BROKER", "kafka:9092")),
		os.Setenv("KAFKA_CONSUMER_GROUP", getEnv("KAFKA_CONSUMER_GROUP", "updates-consumer")),
		os.Setenv("KAFKA_TOPIC", getEnv("KAFKA_TOPIC", "updates")),
	)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func newConfigFromEnv() (*config, error) {
	cfg := &config{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
