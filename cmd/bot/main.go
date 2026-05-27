package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/golang-migrate/migrate/v4/source/file"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/bot"
	botconsumer "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/consumer"
	botserver "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/server"
	scrapper "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/scrapperapi/client"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/tgapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/userstorage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := setEnv(); err != nil {
		logger.Error("setEnv", slog.String("error", err.Error()))
		os.Exit(1)
	}

	cfg, err := newConfigFromEnv()
	if err != nil {
		logger.Error("new config from env:", slog.String("err", err.Error()))
		os.Exit(1)
	}

	logger.Info("starting bot", slog.String("config", fmt.Sprintf("%#v", cfg)))

	if cfg.Token == "" {
		logger.Error("telegram token not set")
		os.Exit(1)
	}

	api, err := tgapi.NewTgAPI(cfg.Token)
	if err != nil {
		logger.Error("create tg api bot", slog.String("error", err.Error()))
		return
	}

	storage, err := userstorage.NewPgStorage(cfg.toDSN(), storageTimeout)
	if err != nil {
		logger.Error("create user storage", slog.String("error", err.Error()))
		os.Exit(1)
	}

	b := bot.NewBot(logger, api, storage, buildScrapperClient(cfg))

	consumer, srv, cleanup, err := setupCommunication(cfg, b, logger)
	if err != nil {
		logger.Error("setup communication", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer cleanup()

	runWaitgroups(cfg, b, consumer, srv, logger)
}

func buildScrapperClient(cfg *config) scrapper.Client {
	return scrapper.NewClient(scrapper.Config{
		BaseURL:          "http://scrapper:8001",
		Timeout:          time.Duration(cfg.Timeout) * time.Second,
		RetryAttempts:    uint(cfg.RetryAttempts),
		RetryDelay:       time.Duration(cfg.RetryDelay) * time.Millisecond,
		CBRatioThreshold: cfg.CBRatioThreshold,
		CBMinRequests:    uint32(cfg.CBMinRequests),
		CBOpenWindow:     time.Duration(cfg.CBOpenWindow) * time.Second,
	})
}

func setupCommunication(cfg *config, b *bot.Bot, logger *slog.Logger) (botconsumer.Consumer, *botserver.Server, func(), error) {
	var consumer botconsumer.Consumer
	var srv *botserver.Server
	cleanup := func() {}

	switch cfg.CommunicationType {
	case "KAFKA":
		handler := botconsumer.NewHandler(b, cfg.SchemaRegistryURL)
		c, err := botconsumer.NewConsumer(
			handler, cfg.KafkaBroker, cfg.KafkaUser, cfg.KafkaPassword,
			cfg.KafkaConsumerGroup, cfg.KafkaTopic, logger,
		)
		if err != nil {
			return consumer, srv, cleanup, fmt.Errorf("new consumer: %w", err)
		}
		consumer = c
		cleanup = func() {
			if err = consumer.Close(); err != nil {
				logger.Error("close consumer", slog.String("error", err.Error()))
			}
		}
	case "HTTP":
		srv = botserver.NewServer(":8002", botserver.NewHandler(b))
	default:
		return consumer, srv, cleanup, fmt.Errorf("incorrect communication type: %s", cfg.CommunicationType)
	}

	return consumer, srv, cleanup, nil
}

func runWaitgroups(cfg *config, b *bot.Bot, consumer botconsumer.Consumer, srv *botserver.Server, logger *slog.Logger) {
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	if cfg.CommunicationType == "KAFKA" {
		logger.Info("start consumer")
		wg.Go(func() { consumer.Serve(ctx) })
	} else if srv != nil {
		logger.Info("start server")
		wg.Go(func() {
			if err := srv.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("server run", slog.String("error", err.Error()))
			}
		})
	}

	logger.Info("start polling")
	wg.Go(func() { b.StartPolling(ctx) })

	<-sigterm
	logger.Info("bot gracefully shutting down")

	if srv != nil {
		if err := srv.Stop(ctx); err != nil {
			logger.Error("stop server", slog.String("error", err.Error()))
		}
	}

	cancel()
	wg.Wait()
}
