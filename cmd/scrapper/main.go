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

	_ "github.com/golang-migrate/migrate/v4/source/file"
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

	linkStorage, err := setupStorage(cfg, logger)
	if err != nil {
		logger.Error("setup storage", slog.String("error", err.Error()))
		os.Exit(1)
	}

	client, cleanupClient, err := setupClient(cfg, logger)
	if err != nil {
		logger.Error("setup client", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer cleanupClient()

	var linkCache scrapper.LinksCache
	if cfg.Cache {
		var c *cache.Cache
		c, err = cache.NewCache(cfg.ValkeyHost, cfg.ValkeyPort, cfg.ValkeyUsername, cfg.ValkeyPassword, cfg.ValkeyCacheTTL, cfg.ValkeyTimeout)
		if err != nil {
			logger.Error("create link cache", slog.String("error", err.Error()))
			return
		}
		linkCache = c
	}

	ghFetcher := githubfetcher.NewFetcher(cfg.GithubToken, cfg.GithubTimeout)
	soFetcher := stackoverflowfetcher.NewFetcher(cfg.StackoverflowToken, cfg.SoTimeout)

	svc, err := scrapper.NewService(logger, client, linkStorage, ghFetcher, soFetcher, linkCache, cfg.JobDuration)
	if err != nil {
		logger.Error("create scrapper", slog.String("error", err.Error()))
		return
	}

	if err = svc.LoadAllLinks(); err != nil {
		logger.Error("scrapper load all links", slog.String("error", err.Error()))
		return
	}

	handler := scrapperserver.NewHandler(svc, logger)
	srv := scrapperserver.NewServer(":8001", handler)

	startServer(srv, svc, logger)

}

func setupStorage(cfg *config, logger *slog.Logger) (scrapper.LinkStorage, error) {
	switch cfg.AccessType {
	case "QUERY_BUILDER":
		logger.Info("", slog.String("dsn", cfg.toDSN()))
		store, err := linkstorage.NewSquirrelStorage(cfg.toDSN(), linkstorageTimeout, "file:///app/migrations")
		if err != nil {
			return nil, fmt.Errorf("create squirrel storage: %w", err)
		}
		return store, nil
	case "SQL":
		logger.Info("", slog.String("dsn", cfg.toDSN()))
		store, err := linkstorage.NewLinkStorage(cfg.toDSN(), linkstorageTimeout, "file:///app/migrations")
		if err != nil {
			return nil, fmt.Errorf("create sql storage: %w", err)
		}
		return store, nil
	case "IN_MEMORY":
		logger.Info("use in-memory link storage")
		return linkstorage.NewInMemory(), nil
	default:
		return nil, fmt.Errorf("incorrect access type: %s", cfg.AccessType)
	}
}

func setupClient(cfg *config, logger *slog.Logger) (scrapper.BotClient, func(), error) {
	cleanup := func() {}

	switch cfg.CommunicationType {
	case "KAFKA":
		prod, err := botproducer.NewProducer(cfg.KafkaBroker, cfg.KafkaUser, cfg.KafkaPassword, cfg.KafkaTopic, cfg.SchemaRegistryURL)
		if err != nil {
			return nil, cleanup, fmt.Errorf("new producer: %w", err)
		}
		cleanup = func() {
			if err = prod.Close(); err != nil {
				logger.Error("close client", slog.String("error", err.Error()))
			}
		}
		return prod, cleanup, nil
	case "HTTP":
		return botclient.NewRestClient("http://bot:8002", restTimeout), cleanup, nil
	default:
		return nil, cleanup, fmt.Errorf("incorrect communication type: %s", cfg.CommunicationType)
	}
}

func startServer(srv scrapperserver.Server, svc *scrapper.Service, logger *slog.Logger) {
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

	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		logger.Error("shutdown server", slog.String("error", err.Error()))
	}

	svc.Stop()
}
