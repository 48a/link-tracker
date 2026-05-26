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

	Host     string `env:"DB_BOT_HOST"`
	Port     int    `env:"DB_BOT_PORT"`
	Username string `env:"DB_BOT_USERNAME"`
	Password string `env:"DB_BOT_PASSWORD"`
	Name     string `env:"DB_BOT_NAME"`

	Token string `env:"APP_TELEGRAM_TOKEN"`

	CommunicationType string `env:"BOT_COMMUNICATION_TYPE"`

	SkipCommands      bool   `env:"SKIP_COMMANDS"`
	SchemaRegistryURL string `env:"SCHEMA_REGISTRY_URL"`

	Timeout          int     `env:"SCRAPPER_CLIENT_TIMEOUT"`
	RetryAttempts    int     `env:"SCRAPPER_CLIENT_RETRY_ATTEMPTS"`
	RetryDelay       float64 `env:"SCRAPPER_CLIENT_RETRY_DELAY"`
	CBRatioThreshold float64 `env:"SCRAPPER_CLIENT_CB_RATIO_THRESHOLD"`
	CBMinRequests    int     `env:"SCRAPPER_CLIENT_CB_MIN_REQUESTS"`
	CBOpenWindow     int     `env:"SCRAPPER_CLIENT_CB_OPEN_WINDOW"`
}

func (c *config) toDSN() string {
	return fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=disable",
		c.Username,
		c.Password,
		c.Host,
		c.Port,
		c.Name,
	)
}

func setEnv() error {
	return errors.Join(
		os.Setenv("KAFKA_USER", getEnv("KAFKA_USER", "user1")),
		os.Setenv("KAFKA_PASSWORD", getEnv("KAFKA_PASSWORD", "29665")),
		os.Setenv("KAFKA_BROKER", getEnv("KAFKA_BROKER", "kafka:9092")),
		os.Setenv("KAFKA_CONSUMER_GROUP", getEnv("KAFKA_CONSUMER_GROUP", "updates-consumer")),
		os.Setenv("KAFKA_TOPIC", getEnv("KAFKA_TOPIC", "updates")),

		os.Setenv("DB_BOT_HOST", getEnv("DB_BOT_HOST", "postgres-1")),
		os.Setenv("DB_BOT_PORT", getEnv("DB_BOT_PORT", "5433")),
		os.Setenv("DB_BOT_USERNAME", getEnv("DB_BOT_USERNAME", "user-1")),
		os.Setenv("DB_BOT_PASSWORD", getEnv("DB_BOT_PASSWORD", "566921")),
		os.Setenv("DB_BOT_NAME", getEnv("DB_BOT_NAME", "storage")),

		os.Setenv("BOT_COMMUNICATION_TYPE", getEnv("BOT_COMMUNICATION_TYPE", "KAFKA")),

		os.Setenv("SKIP_COMMANDS", getEnv("SKIP_COMMANDS", "FALSE")),

		os.Setenv("SCHEMA_REGISTRY_URL", getEnv("SCHEMA_REGISTRY_URL", "http://schema-registry:8081")),

		os.Setenv("SCRAPPER_CLIENT_TIMEOUT", getEnv("SCRAPPER_CLIENT_TIMEOUT", "5")),
		os.Setenv("SCRAPPER_CLIENT_RETRY_ATTEMPTS", getEnv("SCRAPPER_CLIENT_RETRY_ATTEMPTS", "3")),
		os.Setenv("SCRAPPER_CLIENT_RETRY_DELAY", getEnv("SCRAPPER_CLIENT_RETRY_DELAY", "500")),
		os.Setenv("SCRAPPER_CLIENT_CB_RATIO_THRESHOLD", getEnv("SCRAPPER_CLIENT_CB_RATIO_THRESHOLD", "0.6")),
		os.Setenv("SCRAPPER_CLIENT_CB_MIN_REQUESTS", getEnv("SCRAPPER_CLIENT_CB_MIN_REQUESTS", "10")),
		os.Setenv("SCRAPPER_CLIENT_CB_OPEN_WINDOW", getEnv("SCRAPPER_CLIENT_CB_OPEN_WINDOW", "15")),
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
