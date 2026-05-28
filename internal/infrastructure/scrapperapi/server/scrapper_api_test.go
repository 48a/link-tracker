package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/scrapperapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application/scrapper"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	scrapperserver "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/scrapperapi/server"
)

type mockScrapperService struct {
	chats map[int64]bool
	links map[int64][]domain.Link
}

func (m *mockScrapperService) RegisterChat(chatID int64) error {
	if m.chats[chatID] {
		return domain.ChatAlreadyExistError{}
	}
	m.chats[chatID] = true
	return nil
}

func (m *mockScrapperService) DeleteChat(chatID int64) error {
	if !m.chats[chatID] {
		return domain.ChatNotExistError{}
	}
	delete(m.chats, chatID)
	delete(m.links, chatID)
	return nil
}

func (m *mockScrapperService) GetLinks(chatID int64) ([]domain.Link, error) {
	if !m.chats[chatID] {
		return nil, domain.ChatNotExistError{}
	}
	return m.links[chatID], nil
}

func (m *mockScrapperService) AddLink(chatID int64, addLinkRequest scrapper.AddLinkInput) (domain.Link, error) {
	if !m.chats[chatID] {
		return domain.Link{}, domain.ChatNotExistError{}
	}
	for _, l := range m.links[chatID] {
		if l.URL == addLinkRequest.URL {
			return l, domain.AlreadyTrackingError{}
		}
	}
	link := domain.Link{ID: chatID, URL: addLinkRequest.URL, Tags: addLinkRequest.Tags}
	m.links[chatID] = append(m.links[chatID], link)
	return link, nil
}

func (m *mockScrapperService) DeleteLink(chatID int64, deleteLinkRequest scrapper.DeleteLinkInput) (domain.Link, error) {
	if !m.chats[chatID] {
		return domain.Link{}, domain.ChatOrLinkNotFoundError{}
	}
	links := m.links[chatID]
	for i, l := range links {
		if l.URL == deleteLinkRequest.URL {
			m.links[chatID] = append(links[:i], links[i+1:]...)
			return l, nil
		}
	}
	return domain.Link{}, domain.ChatOrLinkNotFoundError{}
}

func setupTestMux() *http.ServeMux {
	svc := &mockScrapperService{
		chats: make(map[int64]bool),
		links: make(map[int64][]domain.Link),
	}
	handler := scrapperserver.NewHandler(svc, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /tg-chat/{id}", handler.RegisterChat)
	mux.HandleFunc("DELETE /tg-chat/{id}", handler.DeleteChat)
	mux.HandleFunc("GET /links", handler.GetLinks)
	mux.HandleFunc("POST /links", handler.AddLink)
	mux.HandleFunc("DELETE /links", handler.DeleteLink)

	return mux
}

func TestScrapperAPI_AddAndGetLink(t *testing.T) {
	t.Parallel()

	mux := setupTestMux()

	req := httptest.NewRequest("POST", "/tg-chat/1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got: %d", w.Code)
	}

	addReq := scrapperapi.AddLinkRequest{
		URL:  "https://github.com/user/repo",
		Tags: []string{"golang"},
	}
	body, _ := json.Marshal(addReq)
	req = httptest.NewRequest("POST", "/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "1")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got: %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/links", nil)
	req.Header.Set("Tg-Chat-Id", "1")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got: %d", w.Code)
	}

	var listResp scrapperapi.ListLinksResponse
	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if listResp.Size != 1 {
		t.Errorf("expected list size 1, got: %d", listResp.Size)
	}
	if len(listResp.Links) == 0 || listResp.Links[0].URL != addReq.URL {
		t.Errorf("added link mismatch in final response")
	}
}

func TestScrapperAPI_AddAndDeleteLink(t *testing.T) {
	t.Parallel()

	mux := setupTestMux()

	req := httptest.NewRequest("POST", "/tg-chat/1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	addReq := scrapperapi.AddLinkRequest{URL: "https://github.com/user/repo"}
	body, _ := json.Marshal(addReq)
	req = httptest.NewRequest("POST", "/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "1")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	remReq := scrapperapi.DeleteLinkRequest{URL: "https://github.com/user/repo"}
	body, _ = json.Marshal(remReq)
	req = httptest.NewRequest("DELETE", "/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "1")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got: %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/links", nil)
	req.Header.Set("Tg-Chat-Id", "1")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var listResp scrapperapi.ListLinksResponse
	json.NewDecoder(w.Body).Decode(&listResp)

	if listResp.Size != 0 {
		t.Errorf("expected empty link list, got size: %d", listResp.Size)
	}
}

func TestScrapperAPI_DeleteLinkFromNonExistentChat(t *testing.T) {
	t.Parallel()

	mux := setupTestMux()

	req := httptest.NewRequest("POST", "/tg-chat/1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	addReq := scrapperapi.AddLinkRequest{URL: "https://github.com/user/repo"}
	body, _ := json.Marshal(addReq)
	req = httptest.NewRequest("POST", "/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "1")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	remReq := scrapperapi.DeleteLinkRequest{URL: "https://github.com/user/repo"}
	body, _ = json.Marshal(remReq)
	req = httptest.NewRequest("DELETE", "/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "999")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected failure status code, got: %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/links", nil)
	req.Header.Set("Tg-Chat-Id", "1")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var listResp scrapperapi.ListLinksResponse
	json.NewDecoder(w.Body).Decode(&listResp)

	if listResp.Size != 1 {
		t.Errorf("expected link to still be present in chat 1, got size: %d", listResp.Size)
	}
}

func TestScrapperAPI_AddLinkToNonExistentChat(t *testing.T) {
	t.Parallel()

	mux := setupTestMux()

	addReq := scrapperapi.AddLinkRequest{URL: "https://github.com/user/repo"}
	body, _ := json.Marshal(addReq)
	req := httptest.NewRequest("POST", "/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "2")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected failure status code when adding link to missing chat, got: %d", w.Code)
	}
}

func TestScrapperAPI_InteractWithDeletedChat(t *testing.T) {
	t.Parallel()

	mux := setupTestMux()

	req := httptest.NewRequest("POST", "/tg-chat/1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	req = httptest.NewRequest("DELETE", "/tg-chat/1", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 OK on chat deletion, got: %d", w.Code)
	}

	addReq := scrapperapi.AddLinkRequest{URL: "https://github.com/user/repo"}
	body, _ := json.Marshal(addReq)
	req = httptest.NewRequest("POST", "/links", bytes.NewBuffer(body))
	req.Header.Set("Tg-Chat-Id", "1")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("expected failure status code when adding link to deleted chat, got: %d", w.Code)
	}
}

func TestScrapperAPI_DeleteNonExistentChat(t *testing.T) {
	t.Parallel()

	mux := setupTestMux()

	req := httptest.NewRequest("DELETE", "/tg-chat/1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 Not Found, got: %d", w.Code)
	}
}
