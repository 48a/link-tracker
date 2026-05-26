package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/caarlos0/env/v11"
)

type config struct {
	KafkaBroker        []string `env:"KAFKA_BROKER"`
	KafkaUser          string   `env:"KAFKA_USER"`
	KafkaPassword      string   `env:"KAFKA_PASSWORD"`
	KafkaConsumerGroup string   `env:"KAFKA_CONSUMER_GROUP"`
	InputTopic         string   `env:"KAFKA_INPUT_TOPIC"`
	OutputTopic        string   `env:"KAFKA_OUTPUT_TOPIC"`

	StopWords       string `env:"AGENT_STOP_WORDS"`
	ExcludedAuthors string `env:"AGENT_EXCLUDED_AUTHORS"`
	MinLength       int    `env:"AGENT_MIN_LENGTH"`
	SumThreshold    int    `env:"AGENT_SUM_THRESHOLD"`
}

func setEnv() error {
	return errors.Join(
		os.Setenv("KAFKA_BROKER", getEnv("KAFKA_BROKER", "kafka:9092")),
		os.Setenv("KAFKA_USER", getEnv("KAFKA_USER", "user1")),
		os.Setenv("KAFKA_PASSWORD", getEnv("KAFKA_PASSWORD", "pass123")),
		os.Setenv("KAFKA_CONSUMER_GROUP", getEnv("KAFKA_CONSUMER_GROUP", "ai-agent-group")),
		os.Setenv("KAFKA_INPUT_TOPIC", getEnv("KAFKA_INPUT_TOPIC", "link.raw-updates")),
		os.Setenv("KAFKA_OUTPUT_TOPIC", getEnv("KAFKA_OUTPUT_TOPIC", "link.processed-updates")),

		os.Setenv("AGENT_STOP_WORDS", getEnv("AGENT_STOP_WORDS", "spam,ads,promo")),
		os.Setenv("AGENT_EXCLUDED_AUTHORS", getEnv("AGENT_EXCLUDED_AUTHORS", "bot-user")),
		os.Setenv("AGENT_MIN_LENGTH", getEnv("AGENT_MIN_LENGTH", "20")),
		os.Setenv("AGENT_SUM_THRESHOLD", getEnv("AGENT_SUM_THRESHOLD", "500")),
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

func (c *config) getStopWords() []string {
	if c.StopWords == "" {
		return []string{}
	}
	return strings.Split(c.StopWords, ",")
}

func (c *config) getExcludedAuthors() []string {
	if c.ExcludedAuthors == "" {
		return []string{}
	}
	return strings.Split(c.ExcludedAuthors, ",")
}
