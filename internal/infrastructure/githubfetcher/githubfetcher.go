package githubfetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	maxLen      = 200
	httpTimeout = time.Duration(10) * time.Second
)

type UpdateInfo struct {
	Type        string
	Title       string
	User        string
	CreatedAt   time.Time
	Description string
	UpdatedAt   time.Time
}

type githubUser struct {
	Login string `json:"login"`
}

type githubIssue struct {
	Title            string      `json:"title"`
	User             githubUser  `json:"user"`
	Body             string      `json:"body"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	PullRequestLinks interface{} `json:"pull_request,omitempty"`
}

type Fetcher struct {
	httpClient *http.Client
	token      string
	timeout    time.Duration
	baseURL    string
}

func NewFetcher(token string, timeout int) *Fetcher {
	baseURL := os.Getenv("GITHUB_API_URL")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	return &Fetcher{
		httpClient: &http.Client{Timeout: httpTimeout},
		token:      token,
		timeout:    time.Duration(timeout) * time.Second,
		baseURL:    baseURL,
	}
}

func (f *Fetcher) FetchUpdates(repoURL string, since time.Time) ([]UpdateInfo, error) {
	owner, repo, err := parseURL(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	apiURL := fmt.Sprintf(
		"%s/repos/%s/%s/issues?state=all&sort=updated&since=%s&per_page=10",
		f.baseURL, owner, repo, since.UTC().Format(time.RFC3339),
	)

	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "My-Fetcher")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status: %s", resp.Status)
	}

	var rawIssues []githubIssue
	if err = json.NewDecoder(resp.Body).Decode(&rawIssues); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}

	var updates []UpdateInfo

	for _, issue := range rawIssues {
		if !issue.UpdatedAt.After(since) {
			continue
		}

		itemType := "Issue"
		if issue.PullRequestLinks != nil {
			itemType = "Pull Request"
		}

		description := issue.Body
		runes := []rune(description)
		if len(runes) > maxLen {
			description = string(runes[:maxLen]) + "..."
		}

		updates = append(updates, UpdateInfo{
			Type:        itemType,
			Title:       issue.Title,
			User:        issue.User.Login,
			CreatedAt:   issue.CreatedAt,
			Description: description,
			UpdatedAt:   issue.UpdatedAt,
		})
	}

	return updates, nil
}

//nolint:mnd // not a magic number
func parseURL(repoURL string) (string, string, error) {
	repoURL = strings.TrimSuffix(repoURL, "/")
	parts := strings.Split(repoURL, "github.com/")
	if len(parts) < 2 {
		return "", "", errors.New("invalid github host")
	}

	pathParts := strings.Split(parts[1], "/")
	if len(pathParts) < 2 {
		return "", "", errors.New("missing owner or repository name")
	}

	return pathParts[0], pathParts[1], nil
}
