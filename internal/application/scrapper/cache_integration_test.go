package scrapper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/valkey-io/valkey-go"
	"go.uber.org/mock/gomock"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/cache"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/linkstorage"
)

func TestValkeyCacheIntegration(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "valkey/valkey:latest",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}

	valkeyContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start valkey container: %v", err)
	}
	defer func() {
		if err := valkeyContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %v", err)
		}
	}()

	host, err := valkeyContainer.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := valkeyContainer.MappedPort(ctx, "6379")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	ttlSeconds := 2
	timeout := 5

	realCache, err := cache.NewCache(host, port.Port(), "default", "", ttlSeconds, timeout)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	storageMock := NewMockLinkStorage(ctrl)
	botMock := NewMockBotClient(ctrl)
	ghMock := NewMockgithubFetcher(ctrl)
	soMock := NewMockstackoverflowFetcher(ctrl)

	ghMock.EXPECT().FetchUpdates(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	soMock.EXPECT().FetchUpdates(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	botMock.EXPECT().SendUpdate(gomock.Any()).Return(nil).AnyTimes()
	storageMock.EXPECT().GetLastUpdated(gomock.Any(), gomock.Any()).Return(time.Now(), nil).AnyTimes()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := NewService(logger, botMock, storageMock, ghMock, soMock, realCache, 600)
	if err != nil {
		t.Fatalf("failed to create scrapper service: %v", err)
	}

	var chatID int64 = 1

	storageMock.EXPECT().
		GetLinks(gomock.Any(), chatID).
		Return([]linkstorage.Link{
			{ChatID: chatID, URL: "https://github.com/test/repo", Tags: []string{"test"}, LinkID: 1},
		}, nil).
		Times(1)

	_, err = svc.GetLinks(chatID)
	if err != nil {
		t.Fatalf("first GetLinks call failed: %v", err)
	}

	_, err = svc.GetLinks(chatID)
	if err != nil {
		t.Fatalf("second GetLinks call failed: %v", err)
	}

	rawClient, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{fmt.Sprintf("%s:%s", host, port.Port())},
	})
	if err != nil {
		t.Fatalf("failed to create raw valkey client: %v", err)
	}

	cacheKey := fmt.Sprintf("chat:%d:links", chatID)
	val, err := rawClient.Do(ctx, rawClient.B().Get().Key(cacheKey).Build()).ToString()
	if err != nil {
		t.Fatalf("failed to get raw cache key: %v", err)
	}

	var parsedLinks []domain.Link
	if err := json.Unmarshal([]byte(val), &parsedLinks); err != nil {
		t.Fatalf("cached data is not in valid JSON format: %v", err)
	}

	if len(parsedLinks) != 1 || parsedLinks[0].URL != "https://github.com/test/repo" {
		t.Fatalf("unexpected cached json data: %s", val)
	}

	storageMock.EXPECT().
		AddLink(gomock.Any(), chatID, gomock.Any()).
		Return(linkstorage.Link{ChatID: chatID, URL: "https://github.com/test/repo2", Tags: []string{"test2"}, LinkID: 2}, nil).
		Times(1)

	_, err = svc.AddLink(chatID, AddLinkInput{
		URL:  "https://github.com/test/repo2",
		Tags: []string{"test2"},
	})
	if err != nil {
		t.Fatalf("AddLink call failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	storageMock.EXPECT().
		GetLinks(gomock.Any(), chatID).
		Return([]linkstorage.Link{
			{ChatID: chatID, URL: "https://github.com/test/repo", Tags: []string{"test"}, LinkID: 1},
			{ChatID: chatID, URL: "https://github.com/test/repo2", Tags: []string{"test2"}, LinkID: 2},
		}, nil).
		Times(1)

	_, err = svc.GetLinks(chatID)
	if err != nil {
		t.Fatalf("GetLinks after invalidation failed: %v", err)
	}

	time.Sleep(3 * time.Second)

	storageMock.EXPECT().
		GetLinks(gomock.Any(), chatID).
		Return([]linkstorage.Link{
			{ChatID: chatID, URL: "https://github.com/test/repo", Tags: []string{"test"}, LinkID: 1},
			{ChatID: chatID, URL: "https://github.com/test/repo2", Tags: []string{"test2"}, LinkID: 2},
		}, nil).
		Times(1)

	_, err = svc.GetLinks(chatID)
	if err != nil {
		t.Fatalf("GetLinks after TTL failed: %v", err)
	}
}
