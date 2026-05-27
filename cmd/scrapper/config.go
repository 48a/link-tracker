package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
)

const (
	stopTimeout        = time.Duration(5) * time.Second
	restTimeout        = time.Duration(5) * time.Second
	linkstorageTimeout = time.Duration(5) * time.Second
)

type config struct {
	Host               string   `env:"DB_SCRAPPER_HOST" envDefault:"postgres"`
	Port               int      `env:"DB_SCRAPPER_PORT" envDefault:"5432"`
	Username           string   `env:"DB_SCRAPPER_USERNAME" envDefault:"user"`
	Password           string   `env:"DB_SCRAPPER_PASSWORD" envDefault:"56692"`
	Name               string   `env:"DB_SCRAPPER_NAME" envDefault:"storage"`
	AccessType         string   `env:"DB_SCRAPPER_ACCESS_TYPE" envDefault:"QUERY_BUILDER"`
	KafkaUser          string   `env:"KAFKA_USER"`
	KafkaPassword      string   `env:"KAFKA_PASSWORD"`
	KafkaBroker        []string `env:"KAFKA_BROKER"`
	KafkaTopic         string   `env:"KAFKA_TOPIC"`
	GithubTimeout      int      `env:"SCRAPPER_GITHUB_TIMEOUT"`
	SoTimeout          int      `env:"SCRAPPER_STACKOVERFLOW_TIMEOUT"`
	GithubToken        string   `env:"GITHUB_TOKEN" envDefault:""`
	StackoverflowToken string   `env:"STACKOVERFLOW_TOKEN" envDefault:""`
	ValkeyHost         string   `env:"VALKEY_HOST"`
	ValkeyPort         string   `env:"VALKEY_PORT"`
	ValkeyUsername     string   `env:"VALKEY_USERNAME"`
	ValkeyPassword     string   `env:"VALKEY_PASSWORD"`
	ValkeyCacheTTL     int      `env:"VALKEY_CACHE_TTL"`
	ValkeyTimeout      int      `env:"VALKEY_TIMEOUT"`
	JobDuration        int      `env:"JOB_DURATION"`
	CommunicationType  string   `env:"BOT_COMMUNICATION_TYPE"`
	Cache              bool     `env:"CACHE"`
	SchemaRegistryURL  string   `env:"SCHEMA_REGISTRY_URL"`
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
		os.Setenv("DB_SCRAPPER_HOST", getEnv("DB_SCRAPPER_HOST", "postgres")),
		os.Setenv("DB_SCRAPPER_PORT", getEnv("DB_SCRAPPER_PORT", "5432")),
		os.Setenv("DB_SCRAPPER_USERNAME", getEnv("DB_SCRAPPER_USERNAME", "user")),
		os.Setenv("DB_SCRAPPER_PASSWORD", getEnv("DB_SCRAPPER_PASSWORD", "56692")),
		os.Setenv("DB_SCRAPPER_NAME", getEnv("DB_SCRAPPER_NAME", "storage")),
		os.Setenv("DB_SCRAPPER_ACCESS_TYPE", getEnv("DB_SCRAPPER_ACCESS_TYPE", "QUERY_BUILDER")),

		os.Setenv("KAFKA_USER", getEnv("KAFKA_USER", "user1")),
		os.Setenv("KAFKA_PASSWORD", getEnv("KAFKA_PASSWORD", "29665")),
		os.Setenv("KAFKA_BROKER", getEnv("KAFKA_BROKER", "kafka:9092")),
		os.Setenv("KAFKA_TOPIC", getEnv("KAFKA_TOPIC", "updates")),

		os.Setenv("SCRAPPER_GITHUB_TIMEOUT", getEnv("SCRAPPER_GITHUB_TIMEOUT", "20")),
		os.Setenv("SCRAPPER_STACKOVERFLOW_TIMEOUT", getEnv("SCRAPPER_STACKOVERFLOW_TIMEOUT", "20")),

		os.Setenv("GITHUB_TOKEN", getEnv("GITHUB_TOKEN", "")),
		os.Setenv("STACKOVERFLOW_TOKEN", getEnv("STACKOVERFLOW_TOKEN", "")),

		os.Setenv("VALKEY_HOST", getEnv("VALKEY_HOST", "valkey")),
		os.Setenv("VALKEY_PORT", getEnv("VALKEY_PORT", "6379")),
		os.Setenv("VALKEY_USERNAME", getEnv("VALKEY_USERNAME", "default")),
		os.Setenv("VALKEY_PASSWORD", getEnv("VALKEY_PASSWORD", "566922")),

		os.Setenv("VALKEY_CACHE_TTL", getEnv("VALKEY_CACHE_TTL", "600")),
		os.Setenv("VALKEY_TIMEOUT", getEnv("VALKEY_TIMEOUT", "5")),

		os.Setenv("JOB_DURATION", getEnv("JOB_DURATION", "600")),

		os.Setenv("BOT_COMMUNICATION_TYPE", getEnv("BOT_COMMUNICATION_TYPE", "KAFKA")),

		os.Setenv("CACHE", getEnv("CACHE", "TRUE")),

		os.Setenv("SCHEMA_REGISTRY_URL", getEnv("SCHEMA_REGISTRY_URL", "http://schema-registry:8081")),
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
