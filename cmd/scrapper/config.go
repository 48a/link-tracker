package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
)

type config struct {
	Host          string   `env:"DB_HOST" envDefault:"postgres"`
	Port          int      `env:"DB_PORT" envDefault:"5432"`
	Username      string   `env:"DB_USERNAME" envDefault:"user"`
	Password      string   `env:"DB_PASSWORD" envDefault:"56692"`
	Name          string   `env:"DB_NAME" envDefault:"storage"`
	KafkaUser     string   `env:"KAFKA_USER"`
	KafkaPassword string   `env:"KAFKA_PASSWORD"`
	KafkaBroker   []string `env:"KAFKA_BROKER"`
	KafkaTopic    string   `env:"KAFKA_TOPIC"`
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
		os.Setenv("DB_HOST", getEnv("DB_HOST", "postgres")),
		os.Setenv("DB_PORT", getEnv("DB_PORT", "5432")),
		os.Setenv("DB_USERNAME", getEnv("DB_USERNAME", "user")),
		os.Setenv("DB_PASSWORD", getEnv("DB_PASSWORD", "56692")),
		os.Setenv("DB_NAME", getEnv("DB_NAME", "storage")),
		os.Setenv("KAFKA_USER", getEnv("KAFKA_USER", "user1")),
		os.Setenv("KAFKA_PASSWORD", getEnv("KAFKA_PASSWORD", "29665")),
		os.Setenv("KAFKA_BROKER", getEnv("KAFKA_BROKER", "kafka:9092")),
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
