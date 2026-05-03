package bot

import (
	"fmt"
	"log/slog"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/scrapperapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/tgapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/userstorage"
)

type telegramAPI interface {
	GetMessagesChan() <-chan tgapi.Update
	SendMessage(int64, string) error
	SetupCommands(cmds []tgapi.Commands) (int, error)
}

type storeUserState interface {
	GetUserState(chatID int64) (int, bool)
	SetUserState(chatID int64, newState int)
	SetRequestURL(chatID int64, link string)
	SetRequestTags(chatID int64, tags []string)
	GetRequest(chatID int64) userstorage.Request
}

type scrapperClient interface {
	RegisterChat(chatID int64) error
	DeleteChat(chatID int64) error
	GetLinks(chatID int64) (scrapperapi.ListLinksResponse, error)
	AddLink(chatID int64, link scrapperapi.AddLinkRequest) error
	DeleteLink(chatID int64, link scrapperapi.DeleteLinkRequest) error
}

type Bot struct {
	logger      *slog.Logger
	tgAPI       telegramAPI
	userStorage storeUserState
	client      scrapperClient
}

func NewBot(logger *slog.Logger, api telegramAPI, userStorage storeUserState, client scrapperClient) *Bot {
	return &Bot{logger: logger, tgAPI: api, userStorage: userStorage, client: client}
}

func (b *Bot) StartPolling() {
	respCode, err := b.tgAPI.SetupCommands([]tgapi.Commands{
		{Command: "/start", Description: "register chat"},
		{Command: "/help", Description: "help command"},
		{Command: "/track", Description: "track link"},
		{Command: "/untrack", Description: "untrack link"},
		{Command: "/list", Description: "list tracked links with optional tag"},
		{Command: "/cancel", Description: "cancel input"},
		{Command: "/stop", Description: "unregister chat"},
	})
	b.logger.Info(fmt.Sprintf("requested commands setup with err %v and code %v", err, respCode))
	updates := b.tgAPI.GetMessagesChan()
	for update := range updates {
		if !update.IsMessage {
			b.logger.Warn("received unsupported message type")
			continue
		}
		// b.logger.Info(fmt.Sprintf("received text message: %s", update.Message))
		reply := b.handleMessage(update.Message, update.ChatID)
		err := b.tgAPI.SendMessage(update.ChatID, reply)
		// b.logger.Info(fmt.Sprintf("sent text message: %s", reply))
		if err != nil {
			b.logger.Error(fmt.Sprintf("can't send message: %v", err))
		}
	}
}
