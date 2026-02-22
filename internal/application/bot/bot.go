package bot

import (
	"fmt"
	"log/slog"
)

type Update struct {
	IsMessage bool
	Message string
	ChatID int64
}

type telegramAPI interface {
	GetMessagesChan() <-chan Update
	SendMessage(int64, string) error
}

type Bot struct {
	logger *slog.Logger
	tgAPI telegramAPI
}

func NewBot(logger *slog.Logger, api telegramAPI) *Bot {
	return &Bot{logger: logger, tgAPI: api}
}

func handleMessage(message string) string {
	switch message {
	case "/start":
		return "welcome"
	case "/help":
		return "only /start and /help commands supported"
	default:
		return "unsupported command"
	}
}

func (b *Bot) StartPolling() {
	updates := b.tgAPI.GetMessagesChan()
	for update := range updates {
		if !update.IsMessage {
			b.logger.Warn("received unsupported message type")
			continue
		}
		b.logger.Info(fmt.Sprintf("received text message: %s", update.Message))
		reply := handleMessage(update.Message)
		err := b.tgAPI.SendMessage(update.ChatID, reply)
		b.logger.Info(fmt.Sprintf("sent text message: %s", reply))
		if err != nil {
			b.logger.Error(fmt.Sprintf("can't send message: %v", err))
		}
	}
}
