package scrapper

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/api/botapi"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/common/linkkind"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/githubfetcher"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/linkstorage"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/stackoverflowfetcher"
)

const (
	shardsNumber = 256
)

type LinkStorage interface {
	RegisterChat(ctx context.Context, chatID int64) error
	DeleteChat(ctx context.Context, chatID int64) error
	GetLinks(ctx context.Context, chatID int64) ([]linkstorage.Link, error)
	AddLink(ctx context.Context, chatID int64, request linkstorage.AddLinkInput) (linkstorage.Link, error)
	DeleteLink(ctx context.Context, chatID int64, request linkstorage.DeleteLinkInput) (linkstorage.Link, error)
	GetAllLinks(ctx context.Context) ([]linkstorage.Link, error)
	GetLastUpdated(ctx context.Context, linkID int) (time.Time, error)
	GetUsersWithLink(ctx context.Context, linkID int) ([]int64, error)
	SetLastUpdated(ctx context.Context, linkID int, lastUpdated time.Time) error
	Close()
}

type BotClient interface {
	SendUpdate(linkUpdate botapi.LinkUpdate) error
}

type githubFetcher interface {
	FetchUpdates(repoURL string, since time.Time) ([]githubfetcher.UpdateInfo, error)
}

type stackoverflowFetcher interface {
	FetchUpdates(questionURL string, since time.Time) (*stackoverflowfetcher.QuestionUpdates, error)
}

type LinksCache interface {
	GetLinks(ctx context.Context, chatID int64) ([]domain.Link, error)
	SetLinks(ctx context.Context, chatID int64, links []domain.Link) error
	Invalidate(ctx context.Context, chatID int64) error
}

type ChatLocker struct {
	shards [shardsNumber]sync.Mutex
}

func NewChatLocker() *ChatLocker {
	return &ChatLocker{}
}

func (l *ChatLocker) Lock(chatID int64) func() {
	idx := uint64(chatID) % shardsNumber
	l.shards[idx].Lock()
	return l.shards[idx].Unlock
}

type Service struct {
	logger        *slog.Logger
	storage       LinkStorage
	client        BotClient
	scheduler     gocron.Scheduler
	jobs          map[int]gocron.Job
	githubFetcher githubFetcher
	sofetcher     stackoverflowFetcher
	cache         LinksCache
	jobDuration   int
	locker        *ChatLocker
}

func NewService(logger *slog.Logger, client BotClient, storage LinkStorage, githubFetcher githubFetcher, sofetcher stackoverflowFetcher, cache LinksCache, jobDuration int) (*Service, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return &Service{}, fmt.Errorf("new scheduler: %w", err)
	}

	s.Start()
	return &Service{logger: logger, storage: storage, client: client, scheduler: s, jobs: make(map[int]gocron.Job), githubFetcher: githubFetcher, sofetcher: sofetcher, cache: cache, jobDuration: jobDuration, locker: NewChatLocker()}, nil
}

func (s *Service) LoadAllLinks() error {
	links, err := s.storage.GetAllLinks(context.Background())
	if err != nil {
		s.logger.Error("storage get all links", slog.String("error", err.Error()))
		return fmt.Errorf("storage get all links: %w", err)
	}

	for _, link := range links {
		if err = s.loadJob(domain.Link{
			ID:   link.ChatID,
			URL:  link.URL,
			Tags: link.Tags,
		}, link.LinkID); err != nil {
			s.logger.Error("load job at start up", slog.String("error", err.Error()))
		}

	}
	return nil
}

func (s *Service) RegisterChat(chatID int64) error {
	unlock := s.locker.Lock(chatID)
	defer unlock()

	if err := s.storage.RegisterChat(context.Background(), chatID); err != nil {
		return fmt.Errorf("storage register chat: %w", err)
	}
	return nil
}

func (s *Service) DeleteChat(chatID int64) error {
	unlock := s.locker.Lock(chatID)
	defer unlock()

	var err error
	links, err := s.storage.GetLinks(context.Background(), chatID)
	if err != nil {
		s.logger.Error("storage get links", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
		return fmt.Errorf("storage get links: %w", err)
	}

	for _, link := range links {
		err = s.unloadJob(link.LinkID)
		if err != nil {
			s.logger.Error("unload job", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
		}
	}

	err = s.storage.DeleteChat(context.Background(), chatID)
	if err != nil {
		s.logger.Error("delete chat", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
	}

	if err == nil && s.cache != nil {
		if err2 := s.cache.Invalidate(context.Background(), chatID); err2 != nil {
			s.logger.Error("can't invalidate cache", slog.String("error", err2.Error()), slog.Int64("chatID", chatID))
		}
	}

	return nil
}

func (s *Service) GetLinks(chatID int64) ([]domain.Link, error) {
	unlock := s.locker.Lock(chatID)
	defer unlock()

	ctx := context.Background()

	if s.cache != nil {
		cachedLinks, err := s.cache.GetLinks(ctx, chatID)
		if err == nil && cachedLinks != nil {
			s.logger.Info("cache hit for links", slog.Int64("chatID", chatID))
			return cachedLinks, nil
		}
	}

	links, err := s.storage.GetLinks(context.Background(), chatID)
	if err != nil {
		s.logger.Error("storage get links", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
		return []domain.Link{}, fmt.Errorf("storage get links: %w", err)
	}

	result := make([]domain.Link, len(links))
	for i, link := range links {
		result[i] = domain.Link{ID: int64(link.LinkID), URL: link.URL, Tags: link.Tags}
	}

	if s.cache != nil {
		if err = s.cache.SetLinks(ctx, chatID, result); err != nil {
			s.logger.Error("can't set cache", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
		}
	}

	return result, nil
}

func (s *Service) AddLink(chatID int64, request AddLinkInput) (domain.Link, error) {
	unlock := s.locker.Lock(chatID)
	defer unlock()

	link, err := s.storage.AddLink(context.Background(), chatID, linkstorage.AddLinkInput{
		URL:         request.URL,
		Tags:        request.Tags,
		LastUpdated: time.Now(),
	})
	if err != nil {
		s.logger.Error("storage add link", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.String("request", fmt.Sprintf("%#v", request)))
		return domain.Link{}, fmt.Errorf("storage add link: %w", err)
	}

	if s.cache != nil {
		if err = s.cache.Invalidate(context.Background(), chatID); err != nil {
			s.logger.Error("can't invalidate cache", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
		}
	}

	added := domain.Link{
		ID:   link.ChatID,
		URL:  link.URL,
		Tags: link.Tags,
	}

	if err = s.loadJob(added, link.LinkID); err != nil {
		s.logger.Error("load job", slog.String("error", err.Error()))
		return added, nil
	}

	return added, nil
}

func (s *Service) DeleteLink(chatID int64, request DeleteLinkInput) (domain.Link, error) {
	unlock := s.locker.Lock(chatID)
	defer unlock()

	link, err := s.storage.DeleteLink(context.Background(), chatID, linkstorage.DeleteLinkInput{
		URL: request.URL,
	})
	if err != nil {
		s.logger.Error("storage delete link", slog.String("error", err.Error()), slog.Int64("chatID", chatID), slog.String("link", fmt.Sprintf("%#v", link)))
		return domain.Link{}, fmt.Errorf("storage delete link: %w", err)
	}

	if s.cache != nil {
		if err = s.cache.Invalidate(context.Background(), chatID); err != nil {
			s.logger.Error("can't invalidate cache", slog.String("error", err.Error()), slog.Int64("chatID", chatID))
		}
	}

	deleted := domain.Link{
		ID:   int64(link.LinkID),
		URL:  link.URL,
		Tags: link.Tags,
	}

	if err = s.unloadJob(link.LinkID); err != nil {
		s.logger.Error("unload job", slog.String("error", err.Error()), slog.Int("linkID", link.LinkID))
		return deleted, nil
	}

	return deleted, nil
}

func (s *Service) Stop() {
	s.storage.Close()
}

func (s *Service) loadJob(link domain.Link, linkID int) error {
	s.logger.Info("load job", slog.Int64("chatID", link.ID), slog.String("url", link.URL), slog.String("tags", strings.Join(link.Tags, ",")), slog.Int("linkID", linkID))

	var runJob func()

	switch linkkind.Kind(link.URL) {
	case "github":
		runJob = func() {
			s.process(link, linkID, s.buildUpdateGithub)
		}

	case "stackoverflow":
		runJob = func() {
			s.process(link, linkID, s.buildUpdateStackoverflow)
		}

	default:
		return errors.New("invalid link")
	}

	j, err := s.scheduler.NewJob(
		gocron.DurationJob(
			time.Duration(s.jobDuration)*time.Second,
		),
		gocron.NewTask(
			runJob,
		),
	)
	if err != nil {
		s.logger.Error("new job", slog.Int("linkID", linkID), slog.String("error", err.Error()))
		return fmt.Errorf("new job: %w", err)
	}

	s.jobs[linkID] = j
	return nil
}

func (s *Service) unloadJob(linkID int) error {
	s.logger.Info("unload job", slog.Int("linkID", linkID))

	j, has := s.jobs[linkID]
	if !has {
		s.logger.Error("find job to unload", slog.String("error", "no such job"), slog.Int("linkID", linkID))
		return ErrNoSuchJob
	}

	if err := s.scheduler.RemoveJob(j.ID()); err != nil {
		s.logger.Error("scheduler remove job", slog.String("error", err.Error()), slog.Int("linkID", linkID))
		return fmt.Errorf("scheduler remove job: %w", err)
	}

	delete(s.jobs, linkID)
	return nil
}
