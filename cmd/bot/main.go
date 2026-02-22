package main

import (
	"fmt"
	"log/slog"
	"os"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/bot"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/tgapi"
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

	bot := bot.NewBot(logger, api)

	logger.Info("start polling")

	bot.StartPolling()
}
