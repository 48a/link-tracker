package bot

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

type mockTelegramAPI struct {
	updatesChan chan Update
	sentMessages []struct {
		chatID int64
		text string
	}
	mu sync.Mutex
}

func (m *mockTelegramAPI) GetMessagesChan() <-chan Update {
	return m.updatesChan
}

func (m *mockTelegramAPI) SendMessage(chatID int64, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sentMessages = append(m.sentMessages, struct {
		chatID int64
		text string
	}{chatID, text})
	return nil
}

func (m *mockTelegramAPI) sentMessagesCopy() []struct {
	chatID int64
	text string
} {
	m.mu.Lock()
	defer m.mu.Unlock()

	copySlice := make([]struct {
		chatID int64
		text string
	}, len(m.sentMessages))

	copy(copySlice, m.sentMessages)

	return copySlice
}

func TestStartPolling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		description string
		updates []Update
		expectedSent []struct {
			chatID int64
			text string
		}
		expectNoSend bool
	}{
		{
			description: "supported commands",
			updates: []Update{
				{IsMessage: true, Message: "/start", ChatID: 1},
				{IsMessage: true, Message: "/help", ChatID: 2},
			},
			expectedSent: []struct {
				chatID int64
				text string
			}{
				{1, "welcome"},
				{2, "only /start and /help commands supported"},
			},
		},
		{
			description: "unsupported command",
			updates: []Update{
				{IsMessage: true, Message: "random text", ChatID: 3},
			},
			expectedSent: []struct {
				chatID int64
				text string
			}{
				{3, "unsupported command"},
			},
		},
		{
			description: "non-message update (should be ignored)",
			updates: []Update{
				{IsMessage: false, Message: "not a message", ChatID: 4},
			},
			expectNoSend: true,
		},
		{
			description: "mixed updates",
			updates: []Update{
				{IsMessage: true, Message: "/start", ChatID: 1},
				{IsMessage: false, Message: "ignore", ChatID: 2},
				{IsMessage: true, Message: "/help", ChatID: 3},
				{IsMessage: true, Message: "unknown", ChatID: 4},
			},
			expectedSent: []struct {
				chatID int64
				text string
			}{
				{1, "welcome"},
				{3, "only /start and /help commands supported"},
				{4, "unsupported command"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			mockAPI := &mockTelegramAPI{
				updatesChan: make(chan Update, len(tt.updates)),
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

			bot := NewBot(logger, mockAPI)

			done := make(chan struct{})

			go func() {
				bot.StartPolling()
				close(done)
			}()

			for _, upd := range tt.updates {
				mockAPI.updatesChan <- upd
			}
			close(mockAPI.updatesChan)

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("timeout reached")
			}

			sent := mockAPI.sentMessagesCopy()

			if tt.expectNoSend {
				if len(sent) != 0 {
					t.Errorf("expected no messages sent, but got %d", len(sent))
				}
				return
			}

			if len(sent) != len(tt.expectedSent) {
				t.Errorf("expected %d sent messages, got %d", len(tt.expectedSent), len(sent))
			}

			for i, expected := range tt.expectedSent {
				if i >= len(sent) {
					t.Errorf("missing message %d: expected %+v", i, expected)
					continue
				}
				if sent[i].chatID != expected.chatID {
					t.Errorf("message %d: expected chatID %d, got %d", i, expected.chatID, sent[i].chatID)
				}
				if sent[i].text != expected.text {
					t.Errorf("message %d: expected text %q, got %q", i, expected.text, sent[i].text)
				}
			}
		})
	}
}
