package userstorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgStorage struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func NewPgStorage(dsn string, timeout int) (*PgStorage, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}
	defer func() { _ = db.Close() }()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file:///app/migrations", "postgres", driver)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err = m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			fmt.Println("No new migrations to apply or migrations folder is empty")
		} else {
			return nil, fmt.Errorf("migrate up: %w", err)
		}
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return &PgStorage{
		pool:    pool,
		timeout: time.Duration(timeout) * time.Second,
	}, nil
}

func (p *PgStorage) GetUserState(ctx context.Context, chatID int64) (int, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var state int
	err := p.pool.QueryRow(ctx,
		"select state from user_states where chat_id = $1", chatID,
	).Scan(&state)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("get user state: %w", err)
	}

	return state, true, nil
}

func (p *PgStorage) SetUserState(ctx context.Context, chatID int64, newState int) error {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	query := `
		insert into user_states (chat_id, state) 
		values ($1, $2) 
		on conflict (chat_id) do update set state = excluded.state;`

	_, err := p.pool.Exec(ctx, query, chatID, newState)
	if err != nil {
		return fmt.Errorf("set user state: %w", err)
	}
	return nil
}

func (p *PgStorage) SetRequestURL(ctx context.Context, chatID int64, link string) error {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	query := `
		insert into user_states (chat_id, url) 
		values ($1, $2) 
		on conflict (chat_id) do update set url = excluded.url;`

	_, err := p.pool.Exec(ctx, query, chatID, link)
	if err != nil {
		return fmt.Errorf("set request url: %w", err)
	}
	return nil
}

func (p *PgStorage) SetRequestTags(ctx context.Context, chatID int64, tags []string) error {
	if tags == nil {
		tags = []string{}
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	query := `
		insert into user_states (chat_id, tags) 
		values ($1, $2) 
		on conflict (chat_id) do update set tags = excluded.tags;`

	_, err := p.pool.Exec(ctx, query, chatID, tags)
	if err != nil {
		return fmt.Errorf("set request tags: %w", err)
	}
	return nil
}

func (p *PgStorage) GetRequest(ctx context.Context, chatID int64) (Request, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var req Request
	query := `select url, tags from user_states where chat_id = $1;`
	err := p.pool.QueryRow(ctx, query, chatID).Scan(&req.URL, &req.Tags)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Request{Tags: []string{}}, nil
		}
		return Request{}, fmt.Errorf("get request: %w", err)
	}

	if req.Tags == nil {
		req.Tags = []string{}
	}

	return req, nil
}

func (p *PgStorage) Close() {
	p.pool.Close()
}
