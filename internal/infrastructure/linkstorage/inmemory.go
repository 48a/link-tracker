package linkstorage

import (
	"context"
	"slices"
	"sync"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type userData struct {
	chatRegistered bool
	links          []Link
}

type linkStorageInMemory struct {
	mu      sync.RWMutex
	storage map[int64]userData
	counter int
}

func NewLinkStorageInMemory() *linkStorageInMemory {
	return &linkStorageInMemory{storage: make(map[int64]userData)}
}

func (ls *linkStorageInMemory) GetAllLinks(ctx context.Context) ([]Link, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	var result []Link
	for _, data := range ls.storage {
		if data.chatRegistered {
			result = append(result, data.links...)
		}
	}
	return result, nil
}

func (ls *linkStorageInMemory) RegisterChat(ctx context.Context, chatID int64) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	data := ls.storage[chatID]
	if data.chatRegistered {
		return domain.ErrChatAlreadyExist{}
	}
	data.chatRegistered = true
	ls.storage[chatID] = data
	return nil
}

func (ls *linkStorageInMemory) DeleteChat(ctx context.Context, chatID int64) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	data := ls.storage[chatID]
	if !data.chatRegistered {
		return domain.ErrChatNotExist{}
	}
	delete(ls.storage, chatID)
	return nil
}

func (ls *linkStorageInMemory) GetLinks(ctx context.Context, chatID int64) ([]Link, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	data := ls.storage[chatID]
	if !data.chatRegistered {
		return []Link{}, domain.ErrChatNotExist{}
	}
	return data.links, nil
}

func (ls *linkStorageInMemory) AddLink(ctx context.Context, chatID int64, request AddLinkInput) (Link, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	data := ls.storage[chatID]
	if !data.chatRegistered {
		return Link{}, domain.ErrChatNotExist{}
	}
	for i := range data.links {
		if data.links[i].URL == request.URL {
			return data.links[i], domain.ErrAlreadyTracking{}
		}
	}
	link := Link{
		LinkID:      ls.counter,
		ChatID:      chatID,
		URL:         request.URL,
		Tags:        request.Tags,
		LastUpdated: request.LastUpdated,
	}
	ls.counter++
	data.links = append(data.links, link)
	ls.storage[chatID] = data
	return link, nil
}

func (ls *linkStorageInMemory) DeleteLink(ctx context.Context, chatID int64, request DeleteLinkInput) (Link, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	data := ls.storage[chatID]
	if !data.chatRegistered {
		return Link{}, domain.ErrChatOrLinkNotFound{}
	}
	for i := range data.links {
		if data.links[i].URL == request.URL {
			result := data.links[i]
			data.links = slices.Concat(data.links[:i], data.links[i+1:])
			ls.storage[chatID] = data
			return result, nil
		}
	}
	return Link{}, domain.ErrChatOrLinkNotFound{}
}

func (ls *linkStorageInMemory) GetLastUpdated(ctx context.Context, linkID int) (time.Time, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	for _, data := range ls.storage {
		for _, link := range data.links {
			if link.LinkID == linkID {
				return link.LastUpdated, nil
			}
		}
	}
	return time.Time{}, domain.ErrChatOrLinkNotFound{}
}

func (ls *linkStorageInMemory) GetUsersWithLink(ctx context.Context, linkID int) ([]int64, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	var result []int64
	for chatID, data := range ls.storage {
		for _, link := range data.links {
			if link.LinkID == linkID {
				result = append(result, chatID)
				break
			}
		}
	}
	return result, nil
}

func (ls *linkStorageInMemory) SetLastUpdated(ctx context.Context, linkID int, lastUpdated time.Time) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	found := false
	for chatID, data := range ls.storage {
		for i, link := range data.links {
			if link.LinkID == linkID {
				data.links[i].LastUpdated = lastUpdated
				ls.storage[chatID] = data
				found = true
			}
		}
	}

	if !found {
		return domain.ErrChatOrLinkNotFound{}
	}
	return nil
}

func (ls *linkStorageInMemory) Close() {}
