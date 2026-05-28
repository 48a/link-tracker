package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type cacheLink struct {
	ID   int64    `json:"id"`
	URL  string   `json:"url"`
	Tags []string `json:"tags"`
}

type Cache struct {
	client  valkey.Client
	ttl     time.Duration
	timeout time.Duration
}

func NewCache(host, port, username, password string, ttl, timeout int) (*Cache, error) {
	addr := fmt.Sprintf("%s:%s", host, port)

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{addr},
		Username:    username,
		Password:    password,
	})
	if err != nil {
		return nil, fmt.Errorf("can't create valkey client: %w", err)
	}

	return &Cache{
		client:  client,
		ttl:     time.Duration(ttl) * time.Second,
		timeout: time.Duration(timeout) * time.Second,
	}, nil
}

func (c *Cache) GetLinks(ctx context.Context, chatID int64) ([]domain.Link, error) {
	opCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := c.client.B().Get().Key(c.key(chatID)).Cache()

	val, err := c.client.DoCache(opCtx, cmd, c.ttl).ToString()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("valkey get: %w", err)
	}

	var cachedLinks []cacheLink
	if err = json.Unmarshal([]byte(val), &cachedLinks); err != nil {
		return nil, fmt.Errorf("unmarshal cached links: %w", err)
	}

	links := make([]domain.Link, len(cachedLinks))
	for i, cl := range cachedLinks {
		links[i] = domain.Link{ID: cl.ID, URL: cl.URL, Tags: cl.Tags}
	}

	return links, nil
}

func (c *Cache) SetLinks(ctx context.Context, chatID int64, links []domain.Link) error {
	opCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cachedLinks := make([]cacheLink, len(links))
	for i, l := range links {
		cachedLinks[i] = cacheLink{ID: l.ID, URL: l.URL, Tags: l.Tags}
	}

	data, err := json.Marshal(cachedLinks)
	if err != nil {
		return fmt.Errorf("marshal links for cache: %w", err)
	}

	cmd := c.client.B().Set().Key(c.key(chatID)).Value(string(data)).Ex(c.ttl).Build()
	if err = c.client.Do(opCtx, cmd).Error(); err != nil {
		return fmt.Errorf("valkey client do: %w", err)
	}
	return nil
}

func (c *Cache) Invalidate(ctx context.Context, chatID int64) error {
	opCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := c.client.B().Del().Key(c.key(chatID)).Build()
	if err := c.client.Do(opCtx, cmd).Error(); err != nil {
		return fmt.Errorf("valkey client do: %w", err)
	}
	return nil
}

func (c *Cache) key(chatID int64) string {
	return fmt.Sprintf("chat:%d:links", chatID)
}
