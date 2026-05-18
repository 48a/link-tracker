package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/scrapper"
	botclient "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/client"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/linkstorage"
	scrapperserver "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/scrapperapi/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	linkStorage, err := linkstorage.NewLinkStorage(os.Getenv("DSN"))
	if err != nil {
		fmt.Printf("create linkstorage: %v\n", err)
		os.Exit(1)
	}

	client := botclient.NewClient("http://bot:8002", 5*time.Second)

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
