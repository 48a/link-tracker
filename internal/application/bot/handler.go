package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/scrapperapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/common/linkkind"
)

const (
	Unregistered = iota
	Default
	TrackAskLink
	TrackAskTags
	UntrackAskLink
)

func (b *Bot) handleMessage(ctx context.Context, message string, chatID int64) string {
	userState, _, err := b.userStorage.GetUserState(ctx, chatID)
	if err != nil {
		b.logger.Error("get user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
		return fmt.Sprintf("database error: %v", err)
	}

	switch userState {
	case Unregistered:
		return b.handleUnregistered(ctx, message, chatID)

	case Default:
		return b.handleDefault(ctx, message, chatID)

	case TrackAskLink:
		return b.handleTrackAskLink(ctx, message, chatID)

	case TrackAskTags:
		return b.handleTrackAskTags(ctx, message, chatID)

	case UntrackAskLink:
		return b.handleUntrackAskLink(ctx, message, chatID)
	}

	b.logger.Error("unknown user state", slog.Int("userState", userState))
	return "internal error: unkown user state"
}

func (b *Bot) handleUnregistered(ctx context.Context, message string, chatID int64) string {
	switch message {
	case "/start":
		if err := b.client.RegisterChat(ctx, chatID); err != nil {
			b.logger.Error("register chat", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
			return fmt.Sprintf("client error: %v", err)
		}

		if err := b.userStorage.SetUserState(ctx, chatID, Default); err != nil {
			b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", Default))
			return fmt.Sprintf("database error: %v", err)
		}

		return "registered"

	case "/help":
		return "help message"
	}

	return "register first"
}

func (b *Bot) handleDefault(ctx context.Context, message string, chatID int64) string {
	switch message {
	case "/start":
		return "already registered"

	case "/help":
		return "help message"

	case "/track":
		if err := b.userStorage.SetUserState(ctx, chatID, TrackAskLink); err != nil {
			b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", TrackAskLink))
			return fmt.Sprintf("database error: %v", err)
		}

		return "send a link to be tracked"

	case "/untrack":
		if err := b.userStorage.SetUserState(ctx, chatID, UntrackAskLink); err != nil {
			b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", UntrackAskLink))
			return fmt.Sprintf("database error: %v", err)
		}

		return "send a link to be untracked"

	case "/cancel":
		return "nothing to cancel"

	case "/stop":
		if err := b.client.DeleteChat(ctx, chatID); err != nil {
			b.logger.Error("delete chat", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
			return fmt.Sprintf("client error: %v", err)
		}

		if err := b.userStorage.SetUserState(ctx, chatID, Unregistered); err != nil {
			b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", Unregistered))
			return fmt.Sprintf("database error: %v", err)
		}

		return "ok"
	}

	if strings.HasPrefix(message, "/list") {
		command := strings.Split(message[5:], ",")
		hasTag := make(map[string]struct{})
		for i := range command {
			trimmed := strings.TrimSpace(command[i])
			if trimmed != "" {
				hasTag[trimmed] = struct{}{}
			}
		}

		links, err := b.client.GetLinks(ctx, chatID)
		if err != nil {
			b.logger.Error("get links", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
			return fmt.Sprintf("client error: %v", err)
		}

		if links.Links == nil {
			b.logger.Error("get links", slog.String("error", "nil link slice received"))
			return "internal error"
		}

		if len(links.Links) != links.Size {
			b.logger.Error("get links", slog.String("error", "slice sizes didn't match"))
			return "internal error"
		}

		responseMessage := []string{}
		for linkNumber := range links.Size {
			if links.Links[linkNumber].Tags == nil || len(hasTag) == 0 {
				responseMessage = append(responseMessage, links.Links[linkNumber].URL+" tags: "+strings.Join(links.Links[linkNumber].Tags, ", "))
				continue
			}

			for _, linkTag := range links.Links[linkNumber].Tags {
				if _, ok := hasTag[linkTag]; ok {
					responseMessage = append(responseMessage, links.Links[linkNumber].URL+" tags: "+strings.Join(links.Links[linkNumber].Tags, ", "))
				}
			}
		}

		if len(responseMessage) == 0 {
			return "no links being tracked"
		}

		return "your link(s): " + strings.Join(responseMessage, "\n")
	}

	return "invalid command"
}

func (b *Bot) handleTrackAskLink(ctx context.Context, message string, chatID int64) string {
	if message == "/cancel" {
		if err := b.userStorage.SetUserState(ctx, chatID, Default); err != nil {
			b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", Default))
			return fmt.Sprintf("database error: %v", err)
		}

		return "cancelled operation"
	}

	if linkkind.Kind(message) == "none" {
		if err := b.userStorage.SetUserState(ctx, chatID, Default); err != nil {
			b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", Default))
			return fmt.Sprintf("database error: %v", err)
		}

		return "invalid link, cancelled operation"
	}

	if err := b.userStorage.SetRequestURL(ctx, chatID, message); err != nil {
		b.logger.Error("set request url", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.String("url", message))
		return "can't set request url"
	}

	if err := b.userStorage.SetUserState(ctx, chatID, TrackAskTags); err != nil {
		b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", TrackAskTags))
		return fmt.Sprintf("database error: %v", err)
	}

	return "send link tags, separated by comma"
}

func (b *Bot) handleTrackAskTags(ctx context.Context, message string, chatID int64) string {
	if message == "/cancel" {
		if err := b.userStorage.SetUserState(ctx, chatID, Default); err != nil {
			b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", Default))
			return fmt.Sprintf("database error: %v", err)
		}

		return "cancelled operation"
	}

	tags := strings.Split(message, ",")
	for i := range tags {
		tags[i] = strings.TrimSpace(tags[i])
		if len(tags[i]) == 0 {
			return "empty tags not allowed"
		}
	}

	if err := b.userStorage.SetRequestTags(ctx, chatID, tags); err != nil {
		b.logger.Error("set request tags", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.String("tags", message))
		return fmt.Sprintf("database error: %v", err)
	}

	if err := b.userStorage.SetUserState(ctx, chatID, Default); err != nil {
		b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", Default))
		return fmt.Sprintf("database error: %v", err)
	}

	req, err := b.userStorage.GetRequest(ctx, chatID)
	if err != nil {
		b.logger.Error("get request", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
		return fmt.Sprintf("database error: %v", err)
	}

	addLinkRequest := scrapperapi.AddLinkRequest{
		URL:  req.URL,
		Tags: req.Tags,
	}
	if err := b.client.AddLink(ctx, chatID, addLinkRequest); err != nil {
		b.logger.Error("add link", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.String("addLinkRequest", fmt.Sprintf("%#v", addLinkRequest)))
		return fmt.Sprintf("client error: %v", err)
	}

	return "ok, saved"
}

func (b *Bot) handleUntrackAskLink(ctx context.Context, message string, chatID int64) string {
	if message == "/cancel" {
		if err := b.userStorage.SetUserState(ctx, chatID, Default); err != nil {
			b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", Default))
			return fmt.Sprintf("database error: %v", err)
		}

		return "cancelled operation"
	}

	if err := b.userStorage.SetRequestURL(ctx, chatID, message); err != nil {
		b.logger.Error("set request url", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.String("url", message))
		return "can't set request url"
	}

	req, err := b.userStorage.GetRequest(ctx, chatID)
	if err != nil {
		b.logger.Error("get request", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
		return fmt.Sprintf("database error: %v", err)
	}

	deleteLinkRequest := scrapperapi.DeleteLinkRequest{
		URL: req.URL,
	}
	if err := b.client.DeleteLink(ctx, chatID, deleteLinkRequest); err != nil {
		b.logger.Error("delete link", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.String("deleteLinkRequest", fmt.Sprintf("%#v", deleteLinkRequest)))

		if err1 := b.userStorage.SetUserState(ctx, chatID, Default); err1 != nil {
			b.logger.Error("set user state", slog.String("error", err1.Error()), slog.Int64("chatID", chatID), slog.Int("userState", Default))
			return fmt.Sprintf("client error %v + database error: %v", err, err1)
		}

		return fmt.Sprintf("can't send delete link request: %v", err)
	}

	if err := b.userStorage.SetUserState(ctx, chatID, Default); err != nil {
		b.logger.Error("set user state", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.Int("userState", Default))
		return fmt.Sprintf("database error: %v", err)
	}

	return "ok"
}
