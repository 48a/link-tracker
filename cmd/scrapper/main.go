package main

import (
	"log/slog"
	"os"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/scrapper"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/linkstorage"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/scrapperapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	linkStorage := linkstorage.NewLinkStorage()

	svc := scrapper.NewService(logger, linkStorage)

	handler := scrapperapi.NewHandler(svc)

	srv := scrapperapi.NewServer("127.0.0.1:8001", handler)

	if err := srv.Run(); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}
