package linkstorage

import (
	"slices"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type userData struct {
	chatRegistered bool
	links          []Link
}

type linkStorageInMemory struct {
	storage map[int64]userData
	counter int
}

func NewLinkStorageInMemory() linkStorageInMemory {
	return linkStorageInMemory{storage: make(map[int64]userData)}
}

func (ls *linkStorageInMemory) GetAllLinks() []Link {
	result := make([]Link, len(ls.storage))
	for _, link := range ls.storage {
		result = append(result, link.links...)
	}
	return result
}

func (ls *linkStorageInMemory) RegisterChat(chatID int64) error {
	data := ls.storage[chatID]
	if data.chatRegistered {
		return domain.ErrChatAlreadyExist{}
	}
	data.chatRegistered = true
	ls.storage[chatID] = data
	return nil
}

func (ls *linkStorageInMemory) DeleteChat(chatID int64) error {
	data := ls.storage[chatID]
	if !data.chatRegistered {
		return domain.ErrChatNotExist{}
	}
	delete(ls.storage, chatID)
	return nil
}

func (ls *linkStorageInMemory) GetLinks(chatID int64) ([]Link, error) {
	data := ls.storage[chatID]
	if !data.chatRegistered {
		return []Link{}, domain.ErrChatNotExist{}
	}
	return data.links, nil
}

func (ls *linkStorageInMemory) AddLink(chatID int64, request AddLinkInput) (Link, error) {
	data := ls.storage[chatID]
	if !data.chatRegistered {
		return Link{}, domain.ErrChatNotExist{}
	}
	for i := range data.links {
		if data.links[i].URL == request.URL {
			return data.links[i], domain.ErrAlreadyTracking{}
		}
	}
	link := Link{LinkID: ls.counter, ChatID: chatID, URL: request.URL, Tags: request.Tags}
	ls.counter++
	data.links = append(data.links, link)
	ls.storage[chatID] = data
	return link, nil
}

func (ls *linkStorageInMemory) DeleteLink(chatID int64, request DeleteLinkInput) (Link, error) {
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
