package bot

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/scrapperapi"
)

const (
	Unregistered = iota
	Default
	TrackAskLink
	TrackAskTags
	UntrackAskLink
)

func (b *Bot) handleMessage(message string, chatID int64) string {
	// no value <=> unregistered
	userState, _ := b.userStorage.GetUserState(chatID)

	switch userState {
	case Unregistered:
		return b.handleUnregistered(message, chatID)
	case Default:
		return b.handleDefault(message, chatID)
	case TrackAskLink:
		return b.handleTrackAskLink(message, chatID)
	case TrackAskTags:
		return b.handleTrackAskTags(message, chatID)
	case UntrackAskLink:
		return b.handleUntrackAskLink(message, chatID)
	}
	b.logger.Error(fmt.Sprintf("unkown user state: %v", userState))
	return ""
}

func (b *Bot) handleUnregistered(message string, chatID int64) string {
	switch message {
	case "/start":
		err := b.client.RegisterChat(chatID)
		if err != nil {
			b.logger.Error(fmt.Sprintf("can't register %v: %v with wrapped: %v", chatID, err, errors.Unwrap(err)))
			return err.Error()
		}
		b.userStorage.SetUserState(chatID, Default)
		return "registered"
	case "/help":
		return "help message"
	}
	return "register first"
}

func (b *Bot) handleDefault(message string, chatID int64) string {
	// messages with no parameters
	switch message {
	case "/start":
		return "already registered"
	case "/help":
		return "help message"
	case "/track":
		b.userStorage.SetUserState(chatID, TrackAskLink)
		return "send a link to be tracked"
	case "/untrack":
		b.userStorage.SetUserState(chatID, UntrackAskLink)
		return "send a link to be untracked"
	case "/cancel":
		return "nothing to cancel"
	case "/stop":
		err := b.client.DeleteChat(chatID)
		if err != nil {
			b.logger.Error(fmt.Sprintf("can't delete %v: %v", chatID, err))
			return err.Error()
		}
		b.userStorage.SetUserState(chatID, Unregistered)
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
		links, err := b.client.GetLinks(chatID)
		if err != nil {
			return err.Error()
		}
		if links.Links == nil {
			return "nil link slice returned received"
		}

		responseMessage := []string{}
		for linkNumber := range links.Size {
			if links.Links[linkNumber].Tags == nil || len(hasTag) == 0 {
				responseMessage = append(responseMessage, links.Links[linkNumber].URL)
				continue
			}
			for _, linkTag := range links.Links[linkNumber].Tags {
				if _, ok := hasTag[linkTag]; ok {
					responseMessage = append(responseMessage, links.Links[linkNumber].URL)
				}
			}
		}
		if len(responseMessage) == 0 {
			return "no links being tracked"
		}
		return "Your links: " + strings.Join(responseMessage, "\n")
	}
	return "invalid command"
}

func (b *Bot) handleTrackAskLink(message string, chatID int64) string {
	if message == "/cancel" {
		b.userStorage.SetUserState(chatID, Default)
		return "cancelled operation"
	}

	u, err := url.Parse(message)
	if err != nil {
		b.userStorage.SetUserState(chatID, Default)
		return "couldn't parse link, cancelled operation"
	}

	if u.Scheme != "https" || (u.Host != "github.com" && u.Host != "stackoverflow.com") {
		b.userStorage.SetUserState(chatID, Default)
		return "invalid link, cancelled operation"
	}

	b.userStorage.SetRequestURL(chatID, message)
	b.userStorage.SetUserState(chatID, TrackAskTags)
	return "send link tags, separated by comma"
}

func (b *Bot) handleTrackAskTags(message string, chatID int64) string {
	if message == "/cancel" {
		b.userStorage.SetUserState(chatID, Default)
		return "cancelled operation"
	}

	tags := strings.Split(message, ",")
	for i := range tags {
		tags[i] = strings.TrimSpace(tags[i])
	}

	b.userStorage.SetRequestTags(chatID, tags)
	b.userStorage.SetUserState(chatID, Default)
	req := b.userStorage.GetRequest(chatID)

	err := b.client.AddLink(chatID, scrapperapi.AddLinkRequest{
		URL:  req.URL,
		Tags: req.Tags,
	})
	if err != nil {
		return err.Error()
	}

	return "ok, saved"
}

func (b *Bot) handleUntrackAskLink(message string, chatID int64) string {
	if message == "/cancel" {
		b.userStorage.SetUserState(chatID, Default)
		return "cancelled operation"
	}

	b.userStorage.SetRequestURL(chatID, message)
	req := b.userStorage.GetRequest(chatID)

	err := b.client.DeleteLink(chatID, scrapperapi.DeleteLinkRequest{
		URL: req.URL,
	})
	if err != nil {
		b.userStorage.SetUserState(chatID, Default)
		return err.Error()
	}

	b.userStorage.SetUserState(chatID, Default)
	return "ok"
}
