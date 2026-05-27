package linkstorage

import (
	"context"
	"testing"
	"time"

	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Storage interface {
	RegisterChat(ctx context.Context, chatID int64) error
	AddLink(ctx context.Context, chatID int64, request AddLinkInput) (Link, error)
	DeleteLink(ctx context.Context, chatID int64, request DeleteLinkInput) (Link, error)
	GetLinks(ctx context.Context, chatID int64) ([]Link, error)
	Close()
}

func TestStorages(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "library/postgres:15-alpine",
		Env:          map[string]string{"POSTGRES_USER": "testuser", "POSTGRES_PASSWORD": "testpassword", "POSTGRES_DB": "testdb"},
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
	}

	postgresC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start container: %v", err)
	}
	defer postgresC.Terminate(ctx)

	host, err := postgresC.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}
	port, err := postgresC.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get port: %v", err)
	}

	dsn := "postgres://testuser:testpassword@" + host + ":" + port.Port() + "/testdb?sslmode=disable"
	migrationsPath := "file://../../../migrations/scrapper"

	sqlStore, err := NewLinkStorage(dsn, 5*time.Second, migrationsPath)
	if err != nil {
		t.Fatalf("failed to init sql storage: %v", err)
	}
	defer sqlStore.Close()

	squirrelStore, err := NewSquirrelStorage(dsn, 5*time.Second, migrationsPath)
	if err != nil {
		t.Fatalf("failed to init squirrel storage: %v", err)
	}
	defer squirrelStore.Close()

	t.Run("SQL Storage", func(t *testing.T) {
		runStorageTests(t, sqlStore, 100)
	})

	t.Run("Squirrel Storage", func(t *testing.T) {
		runStorageTests(t, squirrelStore, 200)
	})
}

func runStorageTests(t *testing.T, s Storage, chatID int64) {
	ctx := context.Background()

	if err := s.RegisterChat(ctx, chatID); err != nil {
		t.Fatalf("failed to register chat: %v", err)
	}

	testURL := "https://github.com/testcontainers"
	testTags := []string{"testing", "golang"}

	t.Run("add link", func(t *testing.T) {
		input := AddLinkInput{
			URL:         testURL,
			Tags:        testTags,
			LastUpdated: time.Now().Truncate(time.Second),
		}

		link, err := s.AddLink(ctx, chatID, input)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if link.URL != testURL {
			t.Errorf("expected URL %s, got %s", testURL, link.URL)
		}
		if !elementsMatch(testTags, link.Tags) {
			t.Errorf("expected tags %v, got %v", testTags, link.Tags)
		}

		links, err := s.GetLinks(ctx, chatID)
		if err != nil {
			t.Fatalf("expected no error getting links, got: %v", err)
		}
		if len(links) != 1 {
			t.Fatalf("expected 1 link, got %d", len(links))
		}
		if links[0].URL != testURL {
			t.Errorf("expected URL %s in db, got %s", testURL, links[0].URL)
		}
	})

	t.Run("add duplicate link", func(t *testing.T) {
		input := AddLinkInput{
			URL:         testURL,
			Tags:        testTags,
			LastUpdated: time.Now(),
		}

		_, err := s.AddLink(ctx, chatID, input)
		if err == nil {
			t.Error("expected error on duplicate link, got nil")
		}
	})

	t.Run("delete link", func(t *testing.T) {
		input := DeleteLinkInput{
			URL: testURL,
		}

		link, err := s.DeleteLink(ctx, chatID, input)
		if err != nil {
			t.Fatalf("expected no error deleting link, got: %v", err)
		}
		if link.URL != testURL {
			t.Errorf("expected deleted URL %s, got %s", testURL, link.URL)
		}

		links, err := s.GetLinks(ctx, chatID)
		if err != nil {
			t.Fatalf("expected no error getting links after deletion, got: %v", err)
		}
		if len(links) != 0 {
			t.Errorf("expected empty links list after deletion, got %d", len(links))
		}
	})
}

func elementsMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int)
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	return true
}
