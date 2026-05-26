package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/scrapperapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/userstorage"
	"go.uber.org/mock/gomock"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestBot_HandleMessage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTgAPI := NewMocktelegramAPI(ctrl)
	mockStorage := NewMockstoreUserState(ctrl)
	mockClient := NewMockscrapperClient(ctrl)

	bot := NewBot(discardLogger, mockTgAPI, mockStorage, mockClient)
	ctx := context.Background()
	chatID := int64(12345)

	t.Run("track link successfully with tags", func(t *testing.T) {
		mockStorage.EXPECT().GetUserState(ctx, chatID).Return(Default, true, nil)
		mockStorage.EXPECT().SetUserState(ctx, chatID, TrackAskLink).Return(nil)

		reply := bot.handleMessage(ctx, "/track", chatID)
		expectedReply := "send a link to be tracked"
		if reply != expectedReply {
			t.Errorf("expected reply %q, got %q", expectedReply, reply)
		}

		mockStorage.EXPECT().GetUserState(ctx, chatID).Return(TrackAskLink, true, nil)
		mockStorage.EXPECT().SetRequestURL(ctx, chatID, "https://github.com/user/repo").Return(nil)
		mockStorage.EXPECT().SetUserState(ctx, chatID, TrackAskTags).Return(nil)

		reply = bot.handleMessage(ctx, "https://github.com/user/repo", chatID)
		expectedReply = "send link tags, separated by comma"
		if reply != expectedReply {
			t.Errorf("expected reply %q, got %q", expectedReply, reply)
		}

		mockStorage.EXPECT().GetUserState(ctx, chatID).Return(TrackAskTags, true, nil)
		mockStorage.EXPECT().SetRequestTags(ctx, chatID, []string{"go", "backend"}).Return(nil)
		mockStorage.EXPECT().SetUserState(ctx, chatID, Default).Return(nil)
		mockStorage.EXPECT().GetRequest(ctx, chatID).Return(userstorage.Request{
			URL:  "https://github.com/user/repo",
			Tags: []string{"go", "backend"},
		}, nil)

		mockClient.EXPECT().AddLink(ctx, chatID, scrapperapi.AddLinkRequest{
			URL:  "https://github.com/user/repo",
			Tags: []string{"go", "backend"},
		}).Return(nil)

		reply = bot.handleMessage(ctx, "go, backend", chatID)
		expectedReply = "ok, saved"
		if reply != expectedReply {
			t.Errorf("expected reply %q, got %q", expectedReply, reply)
		}
	})

	t.Run("track link failed, invalid link", func(t *testing.T) {
		mockStorage.EXPECT().GetUserState(ctx, chatID).Return(TrackAskLink, true, nil)
		mockStorage.EXPECT().SetUserState(ctx, chatID, Default).Return(nil)

		reply := bot.handleMessage(ctx, "gibberish://github.com/user/repo", chatID)
		expectedReply := "invalid link, cancelled operation"
		if reply != expectedReply {
			t.Errorf("expected reply %q, got %q", expectedReply, reply)
		}
	})

	t.Run("track link failed, already subscribed", func(t *testing.T) {
		mockStorage.EXPECT().GetUserState(ctx, chatID).Return(TrackAskTags, true, nil)
		mockStorage.EXPECT().SetRequestTags(ctx, chatID, []string{"tag"}).Return(nil)
		mockStorage.EXPECT().SetUserState(ctx, chatID, Default).Return(nil)
		mockStorage.EXPECT().GetRequest(ctx, chatID).Return(userstorage.Request{
			URL:  "https://github.com/user/repo",
			Tags: []string{"tag"},
		}, nil)

		clientErr := errors.New("link already exists")
		mockClient.EXPECT().AddLink(ctx, chatID, gomock.Any()).Return(clientErr)

		reply := bot.handleMessage(ctx, "tag", chatID)
		expectedSubstring := "client error: link already exists"
		if !strings.Contains(reply, expectedSubstring) {
			t.Errorf("expected reply to contain %q, but got %q", expectedSubstring, reply)
		}
	})

	t.Run("list active subscriptions", func(t *testing.T) {
		mockStorage.EXPECT().GetUserState(ctx, chatID).Return(Default, true, nil)

		mockClient.EXPECT().GetLinks(ctx, chatID).Return(scrapperapi.ListLinksResponse{
			Links: []scrapperapi.LinkResponse{
				{URL: "https://github.com/user/repo", Tags: []string{"go"}},
			},
			Size: 1,
		}, nil)

		reply := bot.handleMessage(ctx, "/list", chatID)
		expectedReply := "your link(s): https://github.com/user/repo tags: go"
		if reply != expectedReply {
			t.Errorf("expected reply %q, got %q", expectedReply, reply)
		}
	})

	t.Run("list subscriptions empty", func(t *testing.T) {
		mockStorage.EXPECT().GetUserState(ctx, chatID).Return(Default, true, nil)

		mockClient.EXPECT().GetLinks(ctx, chatID).Return(scrapperapi.ListLinksResponse{
			Links: []scrapperapi.LinkResponse{},
			Size:  0,
		}, nil)

		reply := bot.handleMessage(ctx, "/list", chatID)
		expectedReply := "no links being tracked"
		if reply != expectedReply {
			t.Errorf("expected reply %q, got %q", expectedReply, reply)
		}
	})

	t.Run("list subscriptions filtered by tag", func(t *testing.T) {
		mockStorage.EXPECT().GetUserState(ctx, chatID).Return(Default, true, nil)

		mockClient.EXPECT().GetLinks(ctx, chatID).Return(scrapperapi.ListLinksResponse{
			Links: []scrapperapi.LinkResponse{
				{URL: "https://github.com/user/repo1", Tags: []string{"go"}},
				{URL: "https://github.com/user/repo2", Tags: []string{"java"}},
			},
			Size: 2,
		}, nil)

		reply := bot.handleMessage(ctx, "/list go", chatID)

		if !strings.Contains(reply, "https://github.com/user/repo1 tags: go") {
			t.Errorf("expected reply to contain repo1 with go tag, got %q", reply)
		}
		if strings.Contains(reply, "repo2") {
			t.Errorf("expected reply not to contain repo2 (java tag), got %q", reply)
		}
	})
}

func TestBot_SendUpdate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTgAPI := NewMocktelegramAPI(ctrl)
	mockStorage := NewMockstoreUserState(ctrl)
	mockClient := NewMockscrapperClient(ctrl)

	bot := NewBot(discardLogger, mockTgAPI, mockStorage, mockClient)

	input := SendUpdateInput{
		ID:          1,
		URL:         "https://github.com/user/repo",
		Description: "New release v1.0.0",
		TgChatIDs:   []int64{111, 222},
	}

	mockTgAPI.EXPECT().SendMessage(int64(111), gomock.Any()).Return(nil)
	mockTgAPI.EXPECT().SendMessage(int64(222), gomock.Any()).Return(nil)

	bot.SendUpdate(input)
}
