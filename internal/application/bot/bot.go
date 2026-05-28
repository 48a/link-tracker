package bot

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/scrapperapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/tgapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/userstorage"
)

const (
	startCommand  = "/start"
	helpCommand   = "/help"
	cancelCommand = "/cancel"
)

type telegramAPI interface {
	GetMessagesChan() <-chan tgapi.Update
	SendMessage(int64, string) error
	SetupCommands(cmds []tgapi.Commands) (int, error)
}

type storeUserState interface {
	GetUserState(ctx context.Context, chatID int64) (int, bool, error)
	SetUserState(ctx context.Context, chatID int64, newState int) error
	SetRequestURL(ctx context.Context, chatID int64, link string) error
	SetRequestTags(ctx context.Context, chatID int64, tags []string) error
	GetRequest(ctx context.Context, chatID int64) (userstorage.Request, error)
	Close()
}

type scrapperClient interface {
	RegisterChat(ctx context.Context, chatID int64) error
	DeleteChat(ctx context.Context, chatID int64) error
	GetLinks(ctx context.Context, chatID int64) (scrapperapi.ListLinksResponse, error)
	AddLink(ctx context.Context, chatID int64, link scrapperapi.AddLinkRequest) error
	DeleteLink(ctx context.Context, chatID int64, link scrapperapi.DeleteLinkRequest) error
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

func (b *Bot) StartPolling(ctx context.Context) {
	if skipCommands := os.Getenv("SKIP_COMMANDS"); skipCommands != "TRUE" {
		respCode, err := b.tgAPI.SetupCommands([]tgapi.Commands{
			{Command: startCommand, Description: "register chat"},
			{Command: helpCommand, Description: "help command"},
			{Command: "/track", Description: "track link"},
			{Command: "/untrack", Description: "untrack link"},
			{Command: "/list", Description: "list tracked links with optional tag"},
			{Command: cancelCommand, Description: "cancel input"},
			{Command: "/stop", Description: "unregister chat"},
		})
		if err != nil {
			b.logger.Error("requested commands setup", slog.String("error", err.Error()), slog.Int("responseCode", respCode))
		}
	}

	updates := b.tgAPI.GetMessagesChan()

	defer b.stop()

	for {
		select {
		case <-ctx.Done():
			b.logger.Info("ctx done", slog.String("err", ctx.Err().Error()))
			return

		case update, ok := <-updates:
			if !ok {
				b.logger.Info("telegram message channel closed")
				return
			}

			if !update.IsMessage {
				b.logger.Warn("received unsupported message type")
				continue
			}

			reply := b.handleMessage(ctx, update.Message, update.ChatID)

			if err := b.tgAPI.SendMessage(update.ChatID, reply); err != nil {
				b.logger.Error("send message", slog.String("error", err.Error()))
			}
		}
	}
}

func (b *Bot) SendUpdate(updateInput SendUpdateInput) {
	for _, chatID := range updateInput.TgChatIDs {
		err := b.tgAPI.SendMessage(chatID, fmt.Sprintf("update to link %q:\n%v", updateInput.URL, updateInput.Description))
		if err != nil {
			b.logger.Error("send update", slog.Int64("chatID", chatID), slog.String("error", err.Error()))
		}
	}
}

func (b *Bot) stop() {
	b.userStorage.Close()
}
