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

type InMemory struct {
	mu      sync.RWMutex
	storage map[int64]userData
	counter int
}

func NewInMemory() *InMemory {
	return &InMemory{storage: make(map[int64]userData)}
}

func (ls *InMemory) GetAllLinks(_ context.Context) ([]Link, error) {
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

func (ls *InMemory) RegisterChat(_ context.Context, chatID int64) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	data := ls.storage[chatID]
	if data.chatRegistered {
		return domain.ChatAlreadyExistError{}
	}
	data.chatRegistered = true
	ls.storage[chatID] = data
	return nil
}

func (ls *InMemory) DeleteChat(_ context.Context, chatID int64) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	data := ls.storage[chatID]
	if !data.chatRegistered {
		return domain.ChatNotExistError{}
	}
	delete(ls.storage, chatID)
	return nil
}

func (ls *InMemory) GetLinks(_ context.Context, chatID int64) ([]Link, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	data := ls.storage[chatID]
	if !data.chatRegistered {
		return []Link{}, domain.ChatNotExistError{}
	}
	return data.links, nil
}

func (ls *InMemory) AddLink(_ context.Context, chatID int64, request AddLinkInput) (Link, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	data := ls.storage[chatID]
	if !data.chatRegistered {
		return Link{}, domain.ChatNotExistError{}
	}
	for i := range data.links {
		if data.links[i].URL == request.URL {
			return data.links[i], domain.AlreadyTrackingError{}
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

func (ls *InMemory) DeleteLink(_ context.Context, chatID int64, request DeleteLinkInput) (Link, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	data := ls.storage[chatID]
	if !data.chatRegistered {
		return Link{}, domain.ChatOrLinkNotFoundError{}
	}
	for i := range data.links {
		if data.links[i].URL == request.URL {
			result := data.links[i]
			data.links = slices.Concat(data.links[:i], data.links[i+1:])
			ls.storage[chatID] = data
			return result, nil
		}
	}
	return Link{}, domain.ChatOrLinkNotFoundError{}
}

func (ls *InMemory) GetLastUpdated(_ context.Context, linkID int) (time.Time, error) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	for _, data := range ls.storage {
		for _, link := range data.links {
			if link.LinkID == linkID {
				return link.LastUpdated, nil
			}
		}
	}
	return time.Time{}, domain.ChatOrLinkNotFoundError{}
}

func (ls *InMemory) GetUsersWithLink(_ context.Context, linkID int) ([]int64, error) {
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

func (ls *InMemory) SetLastUpdated(_ context.Context, linkID int, lastUpdated time.Time) error {
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
		return domain.ChatOrLinkNotFoundError{}
	}
	return nil
}

func (ls *InMemory) Close() {}
