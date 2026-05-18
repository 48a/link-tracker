package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/bot"
	botconsumer "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/consumer"

	// botserver "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/server"
	scrapper "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/scrapperapi/client"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/tgapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/userstorage"
)

func main() {
	if err := setEnv(); err != nil {
		fmt.Printf("setEnv: %v\n", err)
		os.Exit(1)
	}

	cfg, err := newConfigFromEnv()
	if err != nil {
		fmt.Printf("new config from env: %v\n", err)
		os.Exit(1)
	}

	if cfg.Token == "" {
		fmt.Println("telegram token not set")
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("verifying token..")

	api, err := tgapi.NewTgApi(cfg.Token)
	if err != nil {
		fmt.Printf("can't create tg api bot: %v\n", err)
		os.Exit(2)
	}

	logger.Info("success!!!")

	storage := userstorage.NewUserStorage()

	scrapperClient := scrapper.NewClient("http://scrapper:8001", 5*time.Second)

	bot := bot.NewBot(logger, api, storage, scrapperClient)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	handler := botconsumer.NewHandler(bot)

	consumer, err := botconsumer.NewConsumer(
		handler,
		cfg.KafkaBroker,
		cfg.KafkaUser,
		cfg.KafkaPassword,
		cfg.KafkaConsumerGroup,
		cfg.KafkaTopic,
	)
	if err != nil {
		fmt.Printf("new consumer: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup

	logger.Info("start consumer")
	wg.Go(func() { consumer.Serve(ctx) })
	logger.Info("start polling")
	wg.Go(bot.StartPolling)

	<-sigterm

	cancel()

	wg.Wait()

	if err := consumer.Close(); err != nil {
		fmt.Printf("close concumer: %v", err)
		os.Exit(1)
	}
}
