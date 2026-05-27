package scrapper

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

const (
	delimiterCount = 40
	maxLen         = 200
)

func (s *Service) buildUpdateGithub(url string, lastUpdated time.Time) (string, time.Time, error) {
	updates, err := s.githubFetcher.FetchUpdates(url, lastUpdated)
	if err != nil {
		s.logger.Error("fetch github update", slog.String("error", err.Error()))
		return "", time.Time{}, fmt.Errorf("fetch github update: %w", err)
	}

	lastMoment := time.Time{}

	var b strings.Builder
	for i, u := range updates {
		_, err = fmt.Fprintf(&b, "[%d] Found new %s\nTitle: %s\nUser: %s\nCreated at: %s\nUpdated at: %s\nDescription preview:\n%s\n%v\n",
			i+1,
			u.Type,
			tgbotapi.EscapeText(tgbotapi.ModeHTML, u.Title),
			u.User,
			u.CreatedAt.Local().Format("2006-01-02 15:04:05"),
			u.UpdatedAt.Local().Format("2006-01-02 15:04:05"),
			tgbotapi.EscapeText(tgbotapi.ModeHTML, u.Description),
			strings.Repeat("=", delimiterCount))
		if err != nil {
			s.logger.Error("string builder write update", slog.String("error", err.Error()))
			return "", time.Time{}, fmt.Errorf("string builder write update: %w", err)
		}

		if u.UpdatedAt.After(lastMoment) {
			lastMoment = u.UpdatedAt
		}
	}

	return b.String(), lastMoment, nil
}

func (s *Service) buildUpdateStackoverflow(url string, lastUpdated time.Time) (string, time.Time, error) {
	questionUpdates, err := s.sofetcher.FetchUpdates(url, lastUpdated)
	if err != nil {
		s.logger.Error("fetch stackoverflow update", slog.String("error", err.Error()))
		return "", time.Time{}, fmt.Errorf("fetch stackoverflow update: %w", err)
	}

	if len(questionUpdates.Updates) == 0 {
		return "", lastUpdated, nil
	}

	var b strings.Builder
	_, err = fmt.Fprintf(&b, "Updates in topic: %q\n", questionUpdates.TopicTitle)
	if err != nil {
		s.logger.Error("string builder write update", slog.String("error", err.Error()))
		return "", time.Time{}, fmt.Errorf("string builder write update: %w", err)
	}

	lastMoment := time.Time{}

	for i, u := range questionUpdates.Updates {
		_, err = fmt.Fprintf(&b, "%v\n[%d] New %s by: %s\nTime: %s\nPreview:\n%s\n",
			strings.Repeat("=", delimiterCount),
			i+1,
			u.Type,
			u.User,
			u.UpdatedAt.Local().Format("2006-01-02 15:04:05"),
			tgbotapi.EscapeText(tgbotapi.ModeHTML, u.Description))
		if err != nil {
			s.logger.Error("string builder write update", slog.String("error", err.Error()))
			return "", time.Time{}, fmt.Errorf("string builder write update: %w", err)
		}
		if u.UpdatedAt.After(lastMoment) {
			lastMoment = u.UpdatedAt
		}
	}

	return b.String(), lastMoment, nil
}

func (s *Service) process(link domain.Link, linkID int, buildUpdate func(url string, lastUpdated time.Time) (string, time.Time, error)) {
	ctx := context.Background()
	s.logger.Info("start job", slog.Int64("chatID", link.ID), slog.String("url", link.URL), slog.String("tags", strings.Join(link.Tags, ",")), slog.Int("linkID", linkID))
	lastUpdated, err := s.storage.GetLastUpdated(ctx, linkID)
	if err != nil {
		s.logger.Error("get last updated time", slog.String("error", err.Error()), slog.Int("linkID", linkID))
		return
	}

	description, lastMoment, err := buildUpdate(link.URL, lastUpdated)
	if err != nil {
		s.logger.Error("build update", slog.String("error", err.Error()))
	}

	if description != "" {
		var chatIDs []int64
		chatIDs, err = s.storage.GetUsersWithLink(ctx, linkID)
		if err != nil {
			s.logger.Error("get users with link", slog.String("error", err.Error()), slog.Int("linkID", linkID))
			return
		}

		if err = s.storage.SetLastUpdated(ctx, linkID, lastMoment); err != nil {
			s.logger.Error("set last updated time", slog.String("error", err.Error()), slog.Int("linkID", linkID), slog.Time("lastUpdate", lastMoment))
		}

		err = s.client.SendUpdate(botapi.LinkUpdate{
			ID:          int64(linkID),
			URL:         link.URL,
			Description: description[:min(len(description), maxLen)],
			TgChatIDs:   chatIDs,
		})
		if err != nil {
			s.logger.Error("send update",
				slog.String("error", err.Error()),
				slog.Int("linkID", linkID),
				slog.String("url", link.URL),
				slog.String("description", description),
				slog.String("tgChatIDs", fmt.Sprintf("%#v", chatIDs)),
			)
		}
	}
}
