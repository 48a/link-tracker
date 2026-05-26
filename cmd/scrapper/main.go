package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/scrapper"
	botclient "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/client"
	botproducer "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/producer"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/cache"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/githubfetcher"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/linkstorage"
	scrapperserver "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/scrapperapi/server"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/stackoverflowfetcher"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := setEnv(); err != nil {
		logger.Error("setEnv", slog.String("error", err.Error()))
		os.Exit(1)
	}

	cfg, err := newConfigFromEnv()
	if err != nil {
		logger.Error("newConfigFromEnv", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("started scrapper", slog.String("config", fmt.Sprintf("%#v", cfg)))

	var linkStorage scrapper.LinkStorage
	switch cfg.AccessType {
	case "QUERY_BUILDER":
		linkStorage, err = linkstorage.NewSquirrelStorage(cfg.toDSN(), 5*time.Second, "file:///app/migrations")
		logger.Info("", slog.String("dsn", cfg.toDSN()))
		if err != nil {
			logger.Error("create squirrel linkstorage", slog.String("error", err.Error()))
			os.Exit(1)
		}

	case "SQL":
		linkStorage, err = linkstorage.NewLinkStorage(cfg.toDSN(), 5*time.Second, "file:///app/migrations")
		logger.Info("", slog.String("dsn", cfg.toDSN()))
		if err != nil {
			logger.Error("create sql linkstorage", slog.String("error", err.Error()))
			os.Exit(1)
		}

	case "IN_MEMORY":
		linkStorage = linkstorage.NewLinkStorageInMemory()
		logger.Info("use in-memory link storage")

	default:
		logger.Error("incorrect access type", slog.String("accessType", cfg.AccessType))
		os.Exit(1)
	}

	var client scrapper.BotClient
	switch cfg.CommunicationType {
	case "KAFKA":
		client1, err := botproducer.NewProducer(cfg.KafkaBroker, cfg.KafkaUser, cfg.KafkaPassword, cfg.KafkaTopic, cfg.SchemaRegistryURL)
		if err != nil {
			logger.Error("new producer", slog.String("error", err.Error()))
			os.Exit(1)
		}
		defer func() {
			if err := client1.Close(); err != nil {
				logger.Error("close client", slog.String("error", err.Error()))
				os.Exit(1)
			}
		}()
		client = client1

	case "HTTP":
		client = botclient.NewRestClient("http://bot:8002", 5*time.Second)

	default:
		logger.Error("incorrect communication type", slog.String("communicationType", cfg.CommunicationType))
		os.Exit(1)
	}

	githubfetcher := githubfetcher.NewFetcher(cfg.GithubToken, cfg.GithubTimeout)
	sofetcher := stackoverflowfetcher.NewFetcher(cfg.StackoverflowToken, cfg.SoTimeout)

	var linkCache scrapper.LinksCache
	if cfg.Cache {
		linkCache, err = cache.NewCache(cfg.ValkeyHost, cfg.ValkeyPort, cfg.ValkeyUsername, cfg.ValkeyPassword, cfg.ValkeyCacheTTL, cfg.ValkeyTimeout)
		if err != nil {
			logger.Error("create link cache", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}

	svc, err := scrapper.NewService(logger, client, linkStorage, githubfetcher, sofetcher, linkCache, cfg.JobDuration)
	if err != nil {
		logger.Error("create scrapper", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if err = svc.LoadAllLinks(); err != nil {
		logger.Error("scrapper load all links", slog.String("error", err.Error()))
		os.Exit(1)
	}

	handler := scrapperserver.NewHandler(svc, logger)

	srv := scrapperserver.NewServer(":8001", handler)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("start server")
		if err := srv.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server run", slog.String("error", err.Error()))
		}
	}()

	<-sigterm

	logger.Info("scrapper gracefully shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		logger.Error("shutdown server", slog.String("error", err.Error()))
	}

	svc.Stop()
}
