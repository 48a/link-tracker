package scrapper

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-co-op/gocron/v2"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/linkstorage"
)

type linkStorage interface {
	RegisterChat(chatID int64) error
	DeleteChat(chatID int64) error
	GetLinks(chatID int64) ([]linkstorage.Link, error)
	AddLink(chatID int64, request linkstorage.AddLinkInput) (linkstorage.Link, error)
	DeleteLink(chatID int64, request linkstorage.DeleteLinkInput) (linkstorage.Link, error)
	GetAllLinks() ([]linkstorage.Link, error)
	// Close()
}

type botClient interface {
	SendUpdate(linkUpdate botapi.LinkUpdate) error
}

type service struct {
	logger    *slog.Logger
	storage   linkStorage
	client    botClient
	scheduler gocron.Scheduler
	jobs      map[int]gocron.Job
}

func NewService(logger *slog.Logger, client botClient, storage linkStorage) (*service, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return &service{}, err
	}
	s.Start()
	return &service{logger: logger, storage: storage, client: client, scheduler: s, jobs: make(map[int]gocron.Job)}, nil
}

func (s *service) loadJob(link domain.Link, linkID int) error {
	fmt.Printf("got job user: %v\n", link.ID)
	j, err := s.scheduler.NewJob(
		gocron.DurationJob(
			30*time.Second,
		),
		gocron.NewTask(
			s.client.SendUpdate,
			botapi.LinkUpdate{
				ID:          link.ID,
				URL:         link.URL,
				Description: "update test every 30 sec",
				TgChatIDs:   []int64{link.ID},
			},
		),
	)
	if err != nil {
		return err
	}

	s.jobs[linkID] = j
	return nil
}

func (s *service) unloadJob(linkID int) error {
	j, has := s.jobs[linkID]
	if !has {
		return ErrNoSuchJob
	}

	err := s.scheduler.RemoveJob(j.ID())
	if err != nil {
		return err
	}

	delete(s.jobs, linkID)
	return nil
}

func (s *service) LoadAllLinks() error {
	links, err := s.storage.GetAllLinks()
	if err != nil {
		return err
	}

	for _, link := range links {
		fmt.Printf("got link %#v\n", link)
		err = errors.Join(err, s.loadJob(domain.Link{
			ID:   link.ChatID,
			URL:  link.URL,
			Tags: link.Tags,
		}, link.LinkID))
	}
	return err
}

func (s *service) RegisterChat(chatID int64) error {
	return s.storage.RegisterChat(chatID)
}

func (s *service) DeleteChat(chatID int64) error {
	var err error
	links, err := s.storage.GetLinks(chatID)
	if err != nil {
		return err
	}
	for _, link := range links {
		err = errors.Join(err, s.unloadJob(link.LinkID))
	}
	err = errors.Join(err, s.storage.DeleteChat(chatID))
	return err
}

func (s *service) GetLinks(chatID int64) ([]domain.Link, error) {
	links, err := s.storage.GetLinks(chatID)
	if err != nil {
		return []domain.Link{}, err
	}

	result := make([]domain.Link, len(links))
	for i, link := range links {
		result[i] = domain.Link{ID: link.ChatID, URL: link.URL, Tags: link.Tags}
	}
	return result, nil
}

func (s *service) AddLink(chatID int64, request AddLinkInput) (domain.Link, error) {
	link, err := s.storage.AddLink(chatID, linkstorage.AddLinkInput{
		URL:         request.URL,
		Tags:        request.Tags,
		LastUpdated: time.Now(),
	})
	if err != nil {
		return domain.Link{}, err
	}

	err = s.loadJob(domain.Link{
		ID:   link.ChatID,
		URL:  link.URL,
		Tags: link.Tags,
	}, link.LinkID)
	if err != nil {
		return domain.Link{}, err
	}

	return domain.Link{
		ID:   link.ChatID,
		URL:  link.URL,
		Tags: link.Tags,
	}, nil
}

func (s *service) DeleteLink(chatID int64, request DeleteLinkInput) (domain.Link, error) {
	link, err := s.storage.DeleteLink(chatID, linkstorage.DeleteLinkInput{
		URL: request.URL,
	})
	if err != nil {
		return domain.Link{}, err
	}

	err = s.unloadJob(link.LinkID)
	if err != nil {
		return domain.Link{}, err
	}

	return domain.Link{
		ID:   link.ChatID,
		URL:  link.URL,
		Tags: link.Tags,
	}, nil
}
