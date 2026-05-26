package scrapper

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/githubfetcher"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/stackoverflowfetcher"

	"go.uber.org/mock/gomock"
)

func TestProcess_GithubIssue(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := NewMockLinkStorage(ctrl)
	mockClient := NewMockBotClient(ctrl)
	mockGHFetcher := NewMockgithubFetcher(ctrl)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := &service{
		logger:        logger,
		storage:       mockStorage,
		client:        mockClient,
		githubFetcher: mockGHFetcher,
	}

	linkID := 1
	link := domain.Link{ID: 100, URL: "https://github.com/owner/repo"}
	lastUpdated := time.Now().Add(-1 * time.Hour)
	now := time.Now()

	mockStorage.EXPECT().GetLastUpdated(gomock.Any(), linkID).Return(lastUpdated, nil)

	mockGHFetcher.EXPECT().FetchUpdates(link.URL, lastUpdated).Return([]githubfetcher.UpdateInfo{
		{
			Type:        "Issue",
			Title:       "Test GitHub Issue Title",
			User:        "TestAuthorGH",
			Description: "Test GH Description Preview",
			UpdatedAt:   now,
			CreatedAt:   now,
		},
	}, nil)

	mockStorage.EXPECT().GetUsersWithLink(gomock.Any(), linkID).Return([]int64{100}, nil)
	mockStorage.EXPECT().SetLastUpdated(gomock.Any(), linkID, now).Return(nil)

	mockClient.EXPECT().SendUpdate(gomock.Any()).DoAndReturn(func(update botapi.LinkUpdate) error {
		if update.ID != int64(linkID) {
			t.Errorf("expected linkID %d, got %d", linkID, update.ID)
		}
		if !strings.Contains(update.Description, "Test GitHub Issue Title") {
			t.Errorf("missing title in description: %s", update.Description)
		}
		if !strings.Contains(update.Description, "TestAuthorGH") {
			t.Errorf("missing author in description: %s", update.Description)
		}
		if !strings.Contains(update.Description, "Test GH Description Preview") {
			t.Errorf("missing preview in description: %s", update.Description)
		}
		return nil
	})

	svc.process(link, linkID, svc.buildUpdateGithub)
}

func TestProcess_StackOverflowAnswer(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := NewMockLinkStorage(ctrl)
	mockClient := NewMockBotClient(ctrl)
	mockSOFetcher := NewMockstackoverflowFetcher(ctrl)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := &service{
		logger:    logger,
		storage:   mockStorage,
		client:    mockClient,
		sofetcher: mockSOFetcher,
	}

	linkID := 2
	link := domain.Link{ID: 200, URL: "https://stackoverflow.com/questions/12345"}
	lastUpdated := time.Now().Add(-1 * time.Hour)
	now := time.Now()

	mockStorage.EXPECT().GetLastUpdated(gomock.Any(), linkID).Return(lastUpdated, nil)

	mockSOFetcher.EXPECT().FetchUpdates(link.URL, lastUpdated).Return(&stackoverflowfetcher.QuestionUpdates{
		TopicTitle: "Test StackOverflow Topic",
		Updates: []stackoverflowfetcher.UpdateItem{
			{
				Type:        "Answer",
				User:        "TestAuthorSO",
				Description: "Test SO Description Preview",
				UpdatedAt:   now,
			},
		},
	}, nil)

	mockStorage.EXPECT().GetUsersWithLink(gomock.Any(), linkID).Return([]int64{200}, nil)
	mockStorage.EXPECT().SetLastUpdated(gomock.Any(), linkID, now).Return(nil)

	mockClient.EXPECT().SendUpdate(gomock.Any()).DoAndReturn(func(update botapi.LinkUpdate) error {
		if update.ID != int64(linkID) {
			t.Errorf("expected linkID %d, got %d", linkID, update.ID)
		}
		if !strings.Contains(update.Description, "Test StackOverflow Topic") {
			t.Errorf("missing topic title in description: %s", update.Description)
		}
		if !strings.Contains(update.Description, "TestAuthorSO") {
			t.Errorf("missing author in description: %s", update.Description)
		}
		if !strings.Contains(update.Description, "Test SO Description Preview") {
			t.Errorf("missing preview in description: %s", update.Description)
		}
		return nil
	})

	svc.process(link, linkID, svc.buildUpdateStackoverflow)
}

func TestProcess_APIUnavailable(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := NewMockLinkStorage(ctrl)
	mockClient := NewMockBotClient(ctrl)
	mockGHFetcher := NewMockgithubFetcher(ctrl)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := &service{
		logger:        logger,
		storage:       mockStorage,
		client:        mockClient,
		githubFetcher: mockGHFetcher,
	}

	linkID := 3
	link := domain.Link{ID: 300, URL: "https://github.com/owner/error-repo"}
	lastUpdated := time.Now().Add(-1 * time.Hour)

	mockStorage.EXPECT().GetLastUpdated(gomock.Any(), linkID).Return(lastUpdated, nil)
	mockGHFetcher.EXPECT().FetchUpdates(link.URL, lastUpdated).Return(nil, errors.New("api unavailable"))

	mockClient.EXPECT().SendUpdate(gomock.Any()).Times(0)

	svc.process(link, linkID, svc.buildUpdateGithub)
}

func TestProcess_PreviewTruncation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := NewMockLinkStorage(ctrl)
	mockClient := NewMockBotClient(ctrl)
	mockGHFetcher := NewMockgithubFetcher(ctrl)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := &service{
		logger:        logger,
		storage:       mockStorage,
		client:        mockClient,
		githubFetcher: mockGHFetcher,
	}

	linkID := 4
	link := domain.Link{ID: 400, URL: "https://github.com/owner/long-repo"}
	lastUpdated := time.Now().Add(-1 * time.Hour)
	now := time.Now()

	mockStorage.EXPECT().GetLastUpdated(gomock.Any(), linkID).Return(lastUpdated, nil)

	longText := strings.Repeat("A", 300)
	mockGHFetcher.EXPECT().FetchUpdates(link.URL, lastUpdated).Return([]githubfetcher.UpdateInfo{
		{
			Type:        "Issue",
			Title:       "Long Issue",
			User:        "Author",
			Description: longText,
			UpdatedAt:   now,
			CreatedAt:   now,
		},
	}, nil)

	mockStorage.EXPECT().GetUsersWithLink(gomock.Any(), linkID).Return([]int64{400}, nil)
	mockStorage.EXPECT().SetLastUpdated(gomock.Any(), linkID, now).Return(nil)

	mockClient.EXPECT().SendUpdate(gomock.Any()).DoAndReturn(func(update botapi.LinkUpdate) error {
		if len(update.Description) != 200 {
			t.Errorf("expected description length to be 200, got %d", len(update.Description))
		}
		return nil
	})

	svc.process(link, linkID, svc.buildUpdateGithub)
}
