package scrapper

import (
	"fmt"
	"log/slog"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/linkstorage"
)

type linkStorage interface {
	RegisterChat(chatID int64) error
	DeleteChat(chatID int64) error
	GetLinks(chatID int64) ([]domain.Link, error)
	AddLink(chatID int64, request linkstorage.AddLinkInput) (domain.Link, error)
	DeleteLink(chatID int64, request linkstorage.DeleteLinkInput) (domain.Link, error)
}

type service struct {
	logger  *slog.Logger
	storage linkStorage
}

func NewService(logger *slog.Logger, storage linkStorage) service {
	return service{logger: logger, storage: storage}
}

func (s service) RegisterChat(chatID int64) error {
	return s.storage.RegisterChat(chatID)
}

func (s service) DeleteChat(chatID int64) error {
	return s.storage.DeleteChat(chatID)
}

func (s service) GetLinks(chatID int64) ([]domain.Link, error) {
	return s.storage.GetLinks(chatID)
}

func (s service) AddLink(chatID int64, request AddLinkInput) (domain.Link, error) {
	fmt.Printf("Add link request %#v for user %v\n", request, chatID)
	return s.storage.AddLink(chatID, linkstorage.AddLinkInput{URL: request.URL, Tags: request.Tags})
}

func (s service) DeleteLink(chatID int64, request DeleteLinkInput) (domain.Link, error) {
	fmt.Printf("Delete link request %#v for user %v\n", request, chatID)
	return s.storage.DeleteLink(chatID, linkstorage.DeleteLinkInput{URL: request.URL})
}
