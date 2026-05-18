package main

import (
	"fmt"
	"log/slog"
	"os"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/scrapper"
	// botclient "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/client"
	botproducer "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/producer"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/linkstorage"
	scrapperserver "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/scrapperapi/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := setEnv(); err != nil {
		fmt.Printf("setEnv: %v\n", err)
		os.Exit(1)
	}

	cfg, err := newConfigFromEnv()
	if err != nil {
		fmt.Printf("newConfigFromEnv: %v\n", err)
		os.Exit(1)
	}

	linkStorage, err := linkstorage.NewLinkStorage(cfg.toDSN())
	fmt.Println("obtained DSN:", cfg.toDSN())
	if err != nil {
		fmt.Printf("create linkstorage: %v\n", err)
		os.Exit(1)
	}

	// client := botclient.NewRestClient("http://bot:8002", 5*time.Second)
	client, err := botproducer.NewProducer(cfg.KafkaBroker, cfg.KafkaUser, cfg.KafkaPassword, cfg.KafkaTopic)
	if err != nil {
		fmt.Printf("new producer: %v\n", err)
	}

	defer func() {
		err = client.Close()
		if err != nil {
			fmt.Printf("close client: %v", err)
			os.Exit(1)
		}
	}()

	svc, err := scrapper.NewService(logger, client, linkStorage)
	if err != nil {
		fmt.Printf("scrapper error: %v\n", err)
		os.Exit(1)
	}

	err = svc.LoadAllLinks()
	if err != nil {
		fmt.Printf("scrapper load all links: %v\n", err)
		os.Exit(1)
	}

	handler := scrapperserver.NewHandler(svc)

	srv := scrapperserver.NewServer(":8001", handler)

	if err := srv.Run(); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}
