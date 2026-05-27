package tgapi

import (
	"fmt"
	"net/http"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const maxLen = 4090

type TgAPI struct {
	bot *tgbotapi.BotAPI
}

type Update struct {
	IsMessage bool
	Message   string
	ChatID    int64
}

type Commands struct {
	Command     string
	Description string
}

func NewTgAPI(token string) (TgAPI, error) {
	res := TgAPI{}
	var err error

	apiEndpoint := os.Getenv("TG_API_BASE_URL")
	if apiEndpoint != "" {
		res.bot, err = tgbotapi.NewBotAPIWithClient(token, apiEndpoint+"/bot%s/%s", &http.Client{})
	} else {
		res.bot, err = tgbotapi.NewBotAPI(token)
	}

	if err != nil {
		return TgAPI{}, fmt.Errorf("new bot: %w", err)
	}
	return res, nil
}

func (t TgAPI) SetupCommands(cmds []Commands) (int, error) {
	botCmds := make([]tgbotapi.BotCommand, len(cmds))
	for i := range cmds {
		botCmds[i] = tgbotapi.BotCommand{Command: cmds[i].Command, Description: cmds[i].Description}
	}
	config := tgbotapi.NewSetMyCommands(botCmds...)
	resp, err := t.bot.Request(config)
	if err != nil {
		return resp.ErrorCode, fmt.Errorf("setup commands: %w", err)
	}
	return resp.ErrorCode, nil
}

func (t TgAPI) GetMessagesChan() <-chan Update {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := t.bot.GetUpdatesChan(updateConfig)
	out := make(chan Update)

	go func() {
		defer close(out)

		for update := range updates {
			if update.Message != nil {
				out <- Update{IsMessage: true, ChatID: update.Message.Chat.ID, Message: update.Message.Text}
			} else {
				out <- Update{IsMessage: false}
			}
		}
	}()

	return out
}

func (t TgAPI) SendMessage(chatID int64, message string) error {
	msg := tgbotapi.NewMessage(chatID, message[:min(len(message), maxLen)])
	_, err := t.bot.Send(msg)
	if err != nil {
		return fmt.Errorf("telegram send message: %w", err)
	}
	return nil
}
