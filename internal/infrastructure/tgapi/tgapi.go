package tgapi

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/bot"
)

type TgAPI struct {
	bot *tgbotapi.BotAPI
}

func NewTgApi(token string) (TgAPI, error) {
	res := TgAPI{}
	var err error
	res.bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		return TgAPI{}, err
	}
	return res, nil
}

func (t TgAPI) GetMessagesChan() <-chan bot.Update {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := t.bot.GetUpdatesChan(updateConfig)
	out := make(chan bot.Update)

	go func() {
		defer close(out)

		for update := range updates {
			if update.Message != nil {
				out <- bot.Update{IsMessage: true, ChatID: update.Message.Chat.ID, Message: update.Message.Text}
			} else {
				out <- bot.Update{IsMessage: false}
			}
		}
	}()

	return out
}

func (t TgAPI) SendMessage(chatID int64, message string) error {
	msg := tgbotapi.NewMessage(chatID, message)
	_, err := t.bot.Send(msg)
	return err
}
