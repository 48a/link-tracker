package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/bot"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/scrapperapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/tgapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/userstorage"
)

func main() {
	token := os.Getenv("APP_TELEGRAM_TOKEN")

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("verifying token..")

	api, err := tgapi.NewTgApi(token)
	if err != nil {
		fmt.Printf("can't create tg api bot: %v\n", err)
		os.Exit(2)
	}

	logger.Info("success!!!")

	storage := userstorage.NewUserStorage()

	scrapperClient := scrapperapi.NewClient("http://127.0.0.1:8001", 5*time.Second)

	bot := bot.NewBot(logger, api, storage, scrapperClient)

	logger.Info("start polling")

	bot.StartPolling()
}
