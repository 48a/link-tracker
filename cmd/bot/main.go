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

	logger.Info("verifying token..")

	api, err := tgapi.NewTgApi(cfg.Token)
	if err != nil {
		logger.Error("create tg api bot", slog.String("error", err.Error()))
		os.Exit(2)
	}

	logger.Info("success!!!")

	storage, err := userstorage.NewPgStorage(cfg.toDSN(), 5)
	if err != nil {
		logger.Error("create user storage", slog.String("error", err.Error()))
		os.Exit(1)
	}

	scrapperClient := scrapper.NewClient(scrapper.Config{
		BaseURL:          "http://scrapper:8001",
		Timeout:          time.Duration(cfg.Timeout) * time.Second,
		RetryAttempts:    uint(cfg.RetryAttempts),
		RetryDelay:       time.Duration(cfg.RetryDelay) * time.Millisecond,
		CBRatioThreshold: cfg.CBRatioThreshold,
		CBMinRequests:    uint32(cfg.CBMinRequests),
		CBOpenWindow:     time.Duration(cfg.CBOpenWindow) * time.Second,
	})

	bot := bot.NewBot(logger, api, storage, scrapperClient)

	handler := botconsumer.NewHandler(bot, cfg.SchemaRegistryURL)

	var consumer botconsumer.Consumer
	var srv *botserver.Server

	switch cfg.CommunicationType {
	case "KAFKA":
		consumer, err = botconsumer.NewConsumer(
			handler,
			cfg.KafkaBroker,
			cfg.KafkaUser,
			cfg.KafkaPassword,
			cfg.KafkaConsumerGroup,
			cfg.KafkaTopic,
			logger,
		)
		if err != nil {
			logger.Error("new consumer", slog.String("error", err.Error()))
			os.Exit(1)
		}

		defer func() {
			if err := consumer.Close(); err != nil {
				logger.Error("close consumer", slog.String("error", err.Error()))
				os.Exit(1)
			}
		}()

	case "HTTP":
		handler := botserver.NewHandler(bot)
		srv = botserver.NewServer(":8002", handler)

	default:
		logger.Error("incorrect communication type", slog.String("communicationType", cfg.CommunicationType))
		os.Exit(1)
	}

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	if cfg.CommunicationType == "KAFKA" {
		logger.Info("start consumer")
		wg.Go(func() { consumer.Serve(ctx) })
	} else {
		logger.Info("start server")
		wg.Go(func() {
			if err := srv.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("server run", slog.String("error", err.Error()))
			}
		})

	}

	logger.Info("start polling")
	wg.Go(func() { bot.StartPolling(ctx) })

	<-sigterm

	logger.Info("bot gracefully shutting down")

	if srv != nil {
		srv.Stop(ctx)
	}

	cancel()

	wg.Wait()
}
