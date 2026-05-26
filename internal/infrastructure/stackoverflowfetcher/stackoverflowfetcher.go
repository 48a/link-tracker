package stackoverflowfetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type QuestionUpdates struct {
	TopicTitle string
	Updates    []UpdateItem
}

type UpdateItem struct {
	Type        string
	User        string
	UpdatedAt   time.Time
	Description string
}

type seResponse struct {
	Items        []seItem `json:"items"`
	ErrorMessage string   `json:"error_message"`
}

type seItem struct {
	Owner struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	CreationDate int64  `json:"creation_date"`
	LastEditDate int64  `json:"last_edit_date"`
	Body         string `json:"body"`
	Title        string `json:"title"`
}

type Fetcher struct {
	httpClient *http.Client
	apiKey     string
	timeout    time.Duration
	baseURL    string
}

func NewFetcher(apiKey string, timeout int) *Fetcher {
	baseURL := os.Getenv("STACKEXCHANGE_API_URL")
	if baseURL == "" {
		baseURL = "https://api.stackexchange.com/2.3"
	}

	return &Fetcher{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		apiKey:     apiKey,
		timeout:    time.Duration(timeout) * time.Second,
		baseURL:    baseURL,
	}
}

func (f *Fetcher) FetchUpdates(questionURL string, since time.Time) (*QuestionUpdates, error) {
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	qID, err := extractQuestionID(questionURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	title, err := f.getQuestionTitle(ctx, qID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch question title: %w", err)
	}

	result := &QuestionUpdates{
		TopicTitle: title,
		Updates:    make([]UpdateItem, 0),
	}

	sinceUnix := since.Unix()
	baseParams := fmt.Sprintf("site=stackoverflow&fromdate=%d&filter=withbody", sinceUnix)

	answersURL := fmt.Sprintf("%s/questions/%s/answers?%s", f.baseURL, qID, baseParams)
	answers, err := f.fetchItems(ctx, answersURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch answers: %w", err)
	}
	result.Updates = append(result.Updates, processItems(answers, "Answer", since)...)

	commentsURL := fmt.Sprintf("%s/questions/%s/comments?%s", f.baseURL, qID, baseParams)
	comments, err := f.fetchItems(ctx, commentsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}
	result.Updates = append(result.Updates, processItems(comments, "Comment", since)...)

	return result, nil
}

func (f *Fetcher) fetchItems(ctx context.Context, reqURL string) ([]seItem, error) {
	if f.apiKey != "" {
		reqURL += "&key=" + f.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "My-Stack-Fetcher-App")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp seResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	if apiResp.ErrorMessage != "" {
		return nil, fmt.Errorf("API error: %s", apiResp.ErrorMessage)
	}

	return apiResp.Items, nil
}

func processItems(items []seItem, itemType string, since time.Time) []UpdateItem {
	var results []UpdateItem

	for _, item := range items {
		timestamp := item.LastEditDate
		if timestamp == 0 {
			timestamp = item.CreationDate
		}

		updatedAt := time.Unix(timestamp, 0)

		if !updatedAt.After(since) {
			continue
		}

		results = append(results, UpdateItem{
			Type:        itemType,
			User:        item.Owner.DisplayName,
			UpdatedAt:   updatedAt,
			Description: preparePreview(item.Body),
		})
	}
	return results
}

func (f *Fetcher) getQuestionTitle(ctx context.Context, qID string) (string, error) {
	reqURL := fmt.Sprintf("%s/questions/%s?site=stackoverflow", f.baseURL, qID)
	items, err := f.fetchItems(ctx, reqURL)
	if err != nil || len(items) == 0 {
		return "", fmt.Errorf("question not found or error: %v", err)
	}
	return html.UnescapeString(items[0].Title), nil
}

var htmlRegex = regexp.MustCompile(`<[^>]*>`)

func preparePreview(body string) string {
	clean := htmlRegex.ReplaceAllString(body, " ")
	clean = html.UnescapeString(clean)
	clean = strings.Join(strings.Fields(clean), " ")

	runes := []rune(clean)
	if len(runes) > 200 {
		return string(runes[:200]) + "..."
	}
	return clean
}

func extractQuestionID(questionURL string) (string, error) {
	u, err := url.Parse(questionURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, part := range parts {
		if part == "questions" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}
	return "", fmt.Errorf("could not find question ID in URL")
}
