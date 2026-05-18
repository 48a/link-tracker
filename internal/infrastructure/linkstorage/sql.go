package linkstorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type linkStorage struct {
	pool *pgxpool.Pool
}

func NewLinkStorage(dsn string) (*linkStorage, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file:///app/migrations", "postgres", driver)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err = m.Up(); err != migrate.ErrNoChange && err != nil {
		return nil, fmt.Errorf("migrate up: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return &linkStorage{
		pool: pool,
	}, nil
}

func (ls *linkStorage) RegisterChat(chatID int64) error {
	_, err := ls.pool.Exec(context.Background(), `insert into chats(chat_id) values ($1)`, chatID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrChatAlreadyExist{}
	}
	if err != nil {
		return fmt.Errorf("register chat: %w", err)
	}

	return nil
}

func (ls *linkStorage) DeleteChat(chatID int64) error {
	var deletedID int64
	err := ls.pool.QueryRow(context.Background(), `delete from chats where chat_id = $1 returning chat_id`, chatID).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrChatNotExist{}
	}
	if err != nil {
		return fmt.Errorf("delete chat: %w", err)
	}

	return nil
}

func (ls *linkStorage) GetAllLinks() ([]Link, error) {
	return ls.getLinksByQuery(`
		with chats_tags as (
			select
				cl.chat_id, cl.link_id, coalesce(array_agg(t.tag) filter (where t.tag is not null), '{}') as list_tags
			from
				chats_links cl
			join tags t on cl.id = t.sub_id
			group by
				cl.id
		)
		select
			ct.link_id, ct.chat_id, l.url, ct.list_tags, l.last_updated
		from
			chats_tags ct
		join links l on
			ct.link_id = l.id;
	`)
}

func (ls *linkStorage) GetLinks(chatID int64) ([]Link, error) {
	return ls.getLinksByQuery(`
		with chats_tags as (
		select
				cl.chat_id,
			cl.link_id,
			coalesce(array_agg(t.tag) filter (where t.tag is not null), '{}') as list_tags
		from
				chats_links cl
		join tags t on
			cl.id = t.sub_id
		group by
				cl.id
		)
		select
			ct.link_id,
			ct.chat_id,
			l.url,
			ct.list_tags,
			l.last_updated
		from
			chats_tags ct
		join links l on
			ct.link_id = l.id
		where
			chat_id = $1;`,
		chatID)
}

func (ls *linkStorage) getLinksByQuery(query string, args ...any) ([]Link, error) {
	rows, err := ls.pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	defer rows.Close()

	result := make([]Link, 0)

	for rows.Next() {
		var link Link
		err = rows.Scan(&link.LinkID, &link.ChatID, &link.URL, &link.Tags, &link.LastUpdated)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		result = append(result, link)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return result, nil
}

func (ls *linkStorage) AddLink(chatID int64, request AddLinkInput) (_ Link, err error) {
	ctx := context.Background()

	tx, err := ls.pool.Begin(ctx)
	if err != nil {
		return Link{}, fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var chatExists bool
	err = tx.QueryRow(ctx, `select exists(select 1 from chats where chat_id = $1)`, chatID).Scan(&chatExists)
	if err != nil {
		return Link{}, fmt.Errorf("check chat: %w", err)
	}
	if !chatExists {
		return Link{}, domain.ErrChatNotExist{}
	}

	var link Link
	err = tx.QueryRow(ctx, `insert into links(url, last_updated) values ($1, $2) on conflict (url) do update set url = excluded.url returning id, url, last_updated`, request.URL, request.LastUpdated).Scan(&link.LinkID, &link.URL, &link.LastUpdated)
	if err != nil {
		return Link{}, fmt.Errorf("upsert link: %w", err)
	}

	var chatsLinkID int
	err = tx.QueryRow(ctx, `insert into chats_links(chat_id, link_id) values ($1, $2) returning id`, chatID, link.LinkID).Scan(&chatsLinkID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Link{}, domain.ErrAlreadyTracking{}
	}
	if err != nil {
		return Link{}, fmt.Errorf("insert chats_links: %w", err)
	}

	link.ChatID = chatID

	for _, tag := range request.Tags {
		_, err = tx.Exec(ctx, `insert into tags(tag, sub_id) values ($1, $2)`, tag, chatsLinkID)
		if err != nil {
			return Link{}, fmt.Errorf("insert tag: %w", err)
		}
	}

	link.Tags = request.Tags

	err = tx.Commit(ctx)
	if err != nil {
		return Link{}, fmt.Errorf("commit: %w", err)
	}

	return link, nil
}

func (ls *linkStorage) DeleteLink(chatID int64, request DeleteLinkInput) (_ Link, err error) {
	ctx := context.Background()

	tx, err := ls.pool.Begin(ctx)
	if err != nil {
		return Link{}, fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var link Link
	var chatsLinkID int

	err = tx.QueryRow(
		ctx,
		`
		select
			l.id,
			l.url,
			l.last_updated,
			cl.id,
			coalesce(array_agg(t.tag)
				filter (where t.tag is not null), '{}')
		from chats_links cl
		join links l on l.id = cl.link_id
		left join tags t on t.sub_id = cl.id
		where cl.chat_id = $1
		  and l.url = $2
		group by l.id, cl.id
		`,
		chatID,
		request.URL,
	).Scan(
		&link.LinkID,
		&link.URL,
		&link.LastUpdated,
		&chatsLinkID,
		&link.Tags,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, domain.ErrChatOrLinkNotFound{}
	}
	if err != nil {
		return Link{}, fmt.Errorf("find link: %w", err)
	}

	link.ChatID = chatID

	_, err = tx.Exec(ctx, `delete from chats_links where id = $1`, chatsLinkID)
	if err != nil {
		return Link{}, fmt.Errorf("delete chats_links: %w", err)
	}

	var refs int
	err = tx.QueryRow(ctx, `select count(*) from chats_links where link_id = $1`, link.LinkID).Scan(&refs)
	if err != nil {
		return Link{}, fmt.Errorf("count refs: %w", err)
	}

	if refs == 0 {
		_, err = tx.Exec(ctx, `delete from links where id = $1`, link.LinkID)
		if err != nil {
			return Link{}, fmt.Errorf("delete link: %w", err)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return Link{}, fmt.Errorf("commit: %w", err)
	}

	return link, nil
}

func (ls linkStorage) Close() {
	ls.pool.Close()
}
