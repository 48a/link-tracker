package bot

import (
	"fmt"
	"log/slog"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/tgapi"
)

type telegramAPI interface {
	GetMessagesChan() <-chan tgapi.Update
	SendMessage(int64, string) error
	SetupCommands(cmds []tgapi.Commands) (int, error)
}

type Bot struct {
	logger *slog.Logger
	tgAPI telegramAPI
}

func NewBot(logger *slog.Logger, api telegramAPI) *Bot {
	return &Bot{logger: logger, tgAPI: api}
}

func (b *Bot) handleMessage(message string) string {
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
	respCode, err := b.tgAPI.SetupCommands([]tgapi.Commands{
		{Command: "/start", Description: "start command"},
		{Command: "/help", Description: "help command"},
	})
	b.logger.Info(fmt.Sprintf("requested commands setup with err %v and code %v", err, respCode))
	updates := b.tgAPI.GetMessagesChan()
	for update := range updates {
		if !update.IsMessage {
			b.logger.Warn("received unsupported message type")
			continue
		}
		b.logger.Info(fmt.Sprintf("received text message: %s", update.Message))
		reply := b.handleMessage(update.Message)
		err := b.tgAPI.SendMessage(update.ChatID, reply)
		b.logger.Info(fmt.Sprintf("sent text message: %s", reply))
		if err != nil {
			b.logger.Error(fmt.Sprintf("can't send message: %v", err))
		}
	}
}
