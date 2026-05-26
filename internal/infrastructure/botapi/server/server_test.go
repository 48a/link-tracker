package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/bot"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/botapi/server"
)

type mockBotService struct {
	lastUpdate bot.SendUpdateInput
}

func (m *mockBotService) SendUpdate(update bot.SendUpdateInput) {
	m.lastUpdate = update
}

func setupBotTestMux() (*http.ServeMux, *mockBotService) {
	svc := &mockBotService{}
	handler := server.NewHandler(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /updates", handler.SendUpdate)

	return mux, svc
}

func TestBotAPI_ValidUpdate(t *testing.T) {
	t.Parallel()

	mux, _ := setupBotTestMux()

	updateReq := bot.SendUpdateInput{
		ID:          1,
		URL:         "https://github.com/user/repo",
		Description: "New update text",
		TgChatIDs:   []int64{111, 222},
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest("POST", "/updates", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got: %d", w.Code)
	}
}

func TestBotAPI_InvalidUpdate(t *testing.T) {
	t.Parallel()

	mux, _ := setupBotTestMux()

	invalidJSON := []byte(`{
		"id": "not-an-integer",
		"url": "https://github.com/user/repo",
		"tgChatIds": "invalid-should-be-array"
	}`)

	req := httptest.NewRequest("POST", "/updates", bytes.NewBuffer(invalidJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected failure status code, got: %d", w.Code)
	}
}
