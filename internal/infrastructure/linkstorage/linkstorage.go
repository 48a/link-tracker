package linkstorage

import (
	"fmt"
	"slices"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type userData struct {
	chatRegistered bool
	links          []domain.Link
}

type linkStorage struct {
	storage map[int64]userData
}

func NewLinkStorage() linkStorage {
	return linkStorage{storage: make(map[int64]userData)}
}

func (ls linkStorage) RegisterChat(chatID int64) error {
	data := ls.storage[chatID]
	if data.chatRegistered {
		return domain.ErrChatAlreadyExist{}
	}
	data.chatRegistered = true
	ls.storage[chatID] = data
	return nil
}

func (ls linkStorage) DeleteChat(chatID int64) error {
	data := ls.storage[chatID]
	if !data.chatRegistered {
		return domain.ErrChatNotExist{}
	}
	delete(ls.storage, chatID)
	return nil
}

func (ls linkStorage) GetLinks(chatID int64) ([]domain.Link, error) {
	data := ls.storage[chatID]
	if !data.chatRegistered {
		return []domain.Link{}, domain.ErrChatNotExist{}
	}
	fmt.Printf("now: %#v\n%#v\n%#v\n", ls.storage, data, data.links)
	return data.links, nil
}

func (ls linkStorage) AddLink(chatID int64, request AddLinkInput) (domain.Link, error) {
	data := ls.storage[chatID]
	if !data.chatRegistered {
		return domain.Link{}, domain.ErrChatNotExist{}
	}
	for i := range data.links {
		if data.links[i].URL == request.URL {
			return data.links[i], domain.ErrAlreadyTracking{}
		}
	}
	link := domain.Link{ID: chatID, URL: request.URL, Tags: request.Tags}
	data.links = append(data.links, link)
	ls.storage[chatID] = data
	return link, nil
}

func (ls linkStorage) DeleteLink(chatID int64, request DeleteLinkInput) (domain.Link, error) {
	data := ls.storage[chatID]
	if !data.chatRegistered {
		return domain.Link{}, domain.ErrChatOrLinkNotFound{}
	}
	for i := range data.links {
		if data.links[i].URL == request.URL {
			data.links = slices.Concat(data.links[:i], data.links[i+1:])
			ls.storage[chatID] = data
			return data.links[i], nil
		}
	}
	return domain.Link{}, domain.ErrChatOrLinkNotFound{}
}
