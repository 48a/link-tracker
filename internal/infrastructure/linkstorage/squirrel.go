package linkstorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type squirrelStorage struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func NewSquirrelStorage(dsn string, timeout time.Duration, migrations string) (*squirrelStorage, error) {
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

	m, err := migrate.NewWithDatabaseInstance(migrations, "postgres", driver)
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

	return &squirrelStorage{
		pool:    pool,
		timeout: timeout,
	}, nil
}

func (ss *squirrelStorage) RegisterChat(ctx context.Context, chatID int64) error {
	ctx, cancel := context.WithTimeout(ctx, ss.timeout)
	defer cancel()

	query, args, err := psql.Insert("chats").
		Columns("chat_id").
		Values(chatID).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = ss.pool.Exec(ctx, query, args...)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrChatAlreadyExist{}
	}
	if err != nil {
		return fmt.Errorf("register chat: %w", err)
	}

	return nil
}

func (ss *squirrelStorage) DeleteChat(ctx context.Context, chatID int64) error {
	ctx, cancel := context.WithTimeout(ctx, ss.timeout)
	defer cancel()

	query, args, err := psql.Delete("chats").
		Where(sq.Eq{"chat_id": chatID}).
		Suffix("RETURNING chat_id").
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	var deletedID int64
	err = ss.pool.QueryRow(ctx, query, args...).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrChatNotExist{}
	}
	if err != nil {
		return fmt.Errorf("delete chat: %w", err)
	}

	return nil
}

func baseLinksQuery() sq.SelectBuilder {
	return psql.Select(
		"cl.link_id",
		"cl.chat_id",
		"l.url",
		"coalesce(array_agg(t.tag) filter (where t.tag is not null), '{}') as list_tags",
		"l.last_updated",
	).
		From("chats_links cl").
		Join("links l on cl.link_id = l.id").
		LeftJoin("tags t on cl.id = t.sub_id").
		GroupBy("cl.id", "cl.link_id", "cl.chat_id", "l.url", "l.last_updated")
}

func (ss *squirrelStorage) GetAllLinks(ctx context.Context) ([]Link, error) {
	ctx, cancel := context.WithTimeout(ctx, ss.timeout)
	defer cancel()

	query, args, err := baseLinksQuery().ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}
	return ss.getLinksByQuery(ctx, query, args...)
}

func (ss *squirrelStorage) GetLinks(ctx context.Context, chatID int64) ([]Link, error) {
	ctx, cancel := context.WithTimeout(ctx, ss.timeout)
	defer cancel()

	query, args, err := baseLinksQuery().
		Where(sq.Eq{"cl.chat_id": chatID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}
	return ss.getLinksByQuery(ctx, query, args...)
}

func (ss *squirrelStorage) getLinksByQuery(ctx context.Context, query string, args ...any) ([]Link, error) {
	rows, err := ss.pool.Query(ctx, query, args...)
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

func (ss *squirrelStorage) AddLink(ctx context.Context, chatID int64, request AddLinkInput) (_ Link, err error) {
	ctx, cancel := context.WithTimeout(ctx, ss.timeout)
	defer cancel()

	tx, err := ss.pool.Begin(ctx)
	if err != nil {
		return Link{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	query, args, err := psql.Select("1").
		Prefix("SELECT EXISTS(").
		From("chats").
		Where(sq.Eq{"chat_id": chatID}).
		Suffix(")").
		ToSql()
	if err != nil {
		return Link{}, fmt.Errorf("build chat check query: %w", err)
	}

	var chatExists bool
	err = tx.QueryRow(ctx, query, args...).Scan(&chatExists)
	if err != nil {
		return Link{}, fmt.Errorf("check chat: %w", err)
	}
	if !chatExists {
		return Link{}, domain.ErrChatNotExist{}
	}

	query, args, err = psql.Insert("links").
		Columns("url", "last_updated").
		Values(request.URL, request.LastUpdated).
		Suffix("ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url RETURNING id, url, last_updated").
		ToSql()
	if err != nil {
		return Link{}, fmt.Errorf("build upsert link query: %w", err)
	}

	var link Link
	err = tx.QueryRow(ctx, query, args...).Scan(&link.LinkID, &link.URL, &link.LastUpdated)
	if err != nil {
		return Link{}, fmt.Errorf("upsert link: %w", err)
	}

	query, args, err = psql.Insert("chats_links").
		Columns("chat_id", "link_id").
		Values(chatID, link.LinkID).
		Suffix("RETURNING id").
		ToSql()
	if err != nil {
		return Link{}, fmt.Errorf("build insert chats_links query: %w", err)
	}

	var chatsLinkID int
	err = tx.QueryRow(ctx, query, args...).Scan(&chatsLinkID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Link{}, domain.ErrAlreadyTracking{}
	}
	if err != nil {
		return Link{}, fmt.Errorf("insert chats_links: %w", err)
	}

	link.ChatID = chatID
	link.Tags = request.Tags

	if len(request.Tags) > 0 {
		tagInsert := psql.Insert("tags").Columns("tag", "sub_id")
		for _, tag := range request.Tags {
			tagInsert = tagInsert.Values(tag, chatsLinkID)
		}

		query, args, err = tagInsert.ToSql()
		if err != nil {
			return Link{}, fmt.Errorf("build insert tag query: %w", err)
		}

		_, err = tx.Exec(ctx, query, args...)
		if err != nil {
			return Link{}, fmt.Errorf("insert tag: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return Link{}, fmt.Errorf("commit: %w", err)
	}

	return link, nil
}

func (ss *squirrelStorage) DeleteLink(ctx context.Context, chatID int64, request DeleteLinkInput) (_ Link, err error) {
	ctx, cancel := context.WithTimeout(ctx, ss.timeout)
	defer cancel()

	tx, err := ss.pool.Begin(ctx)
	if err != nil {
		return Link{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	query, args, err := psql.Select(
		"l.id",
		"l.url",
		"l.last_updated",
		"cl.id",
		"coalesce(array_agg(t.tag) filter (where t.tag is not null), '{}')",
	).
		From("chats_links cl").
		Join("links l on l.id = cl.link_id").
		LeftJoin("tags t on t.sub_id = cl.id").
		Where(sq.Eq{"cl.chat_id": chatID, "l.url": request.URL}).
		GroupBy("l.id", "cl.id").
		ToSql()
	if err != nil {
		return Link{}, fmt.Errorf("build find link query: %w", err)
	}

	var link Link
	var chatsLinkID int
	err = tx.QueryRow(ctx, query, args...).Scan(
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

	query, args, err = psql.Delete("chats_links").Where(sq.Eq{"id": chatsLinkID}).ToSql()
	if err != nil {
		return Link{}, fmt.Errorf("build delete chats_links query: %w", err)
	}

	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		return Link{}, fmt.Errorf("delete chats_links: %w", err)
	}

	query, args, err = psql.Select("count(*)").
		From("chats_links").
		Where(sq.Eq{"link_id": link.LinkID}).
		ToSql()
	if err != nil {
		return Link{}, fmt.Errorf("build count refs query: %w", err)
	}

	var refs int
	err = tx.QueryRow(ctx, query, args...).Scan(&refs)
	if err != nil {
		return Link{}, fmt.Errorf("count refs: %w", err)
	}

	if refs == 0 {
		query, args, err = psql.Delete("links").Where(sq.Eq{"id": link.LinkID}).ToSql()
		if err != nil {
			return Link{}, fmt.Errorf("build delete link query: %w", err)
		}

		_, err = tx.Exec(ctx, query, args...)
		if err != nil {
			return Link{}, fmt.Errorf("delete link: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return Link{}, fmt.Errorf("commit: %w", err)
	}

	return link, nil
}

func (ss *squirrelStorage) GetLastUpdated(ctx context.Context, linkID int) (time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, ss.timeout)
	defer cancel()

	query, args, err := psql.Select("last_updated").
		From("links").
		Where(sq.Eq{"id": linkID}).
		ToSql()
	if err != nil {
		return time.Time{}, fmt.Errorf("build query: %w", err)
	}

	var result time.Time
	err = ss.pool.QueryRow(ctx, query, args...).Scan(&result)
	if err != nil {
		return time.Time{}, fmt.Errorf("scan: %w", err)
	}

	return result, nil
}

func (ss *squirrelStorage) GetUsersWithLink(ctx context.Context, linkID int) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, ss.timeout)
	defer cancel()

	query, args, err := psql.Select("array_agg(cl.chat_id)").
		From("chats_links cl").
		GroupBy("cl.link_id").
		Having(sq.Eq{"cl.link_id": linkID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	var result []int64
	err = ss.pool.QueryRow(ctx, query, args...).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return []int64{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	return result, nil
}

func (ss *squirrelStorage) SetLastUpdated(ctx context.Context, linkID int, lastUpdated time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, ss.timeout)
	defer cancel()

	query, args, err := psql.Update("links").
		Set("last_updated", lastUpdated).
		Where(sq.Eq{"id": linkID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	result, err := ss.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update error: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("link with id %d not found", linkID)
	}

	return nil
}

func (ss *squirrelStorage) Close() {
	ss.pool.Close()
}
