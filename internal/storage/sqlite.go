package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ilyasaftr/mercari-price-tracking/domain"
	_ "modernc.org/sqlite"
)

type SQLite struct {
	db *sql.DB
}

func NewSQLite(dbPath string) (*SQLite, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &SQLite{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS items (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			price INTEGER NOT NULL,
			status TEXT NOT NULL,
			thumbnail_url TEXT,
			photo_url TEXT,
			item_url TEXT,
			seller_id TEXT,
			seller_name TEXT DEFAULT '',
			category_id TEXT,
			condition_id TEXT,
			condition_name TEXT DEFAULT '',
			shipping_payer TEXT,
			num_likes INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME,
			first_seen DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS price_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			item_id TEXT NOT NULL REFERENCES items(id),
			price INTEGER NOT NULL,
			recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tracked_searches (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			keyword TEXT NOT NULL,
			chat_id INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(keyword, chat_id)
		)`,
		`CREATE TABLE IF NOT EXISTS search_items (
			search_id INTEGER NOT NULL REFERENCES tracked_searches(id) ON DELETE CASCADE,
			item_id TEXT NOT NULL REFERENCES items(id),
			PRIMARY KEY (search_id, item_id)
		)`,
		`CREATE TABLE IF NOT EXISTS alert_keywords (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			search_id INTEGER NOT NULL REFERENCES tracked_searches(id) ON DELETE CASCADE,
			chat_id INTEGER NOT NULL,
			keyword TEXT NOT NULL,
			match_field TEXT NOT NULL DEFAULT 'both',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(search_id, keyword)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_price_history_item ON price_history(item_id)`,
		`CREATE INDEX IF NOT EXISTS idx_price_history_recorded ON price_history(recorded_at)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("exec %q: %w", q[:40], err)
		}
	}

	s.db.Exec(`ALTER TABLE alert_keywords ADD COLUMN match_field TEXT NOT NULL DEFAULT 'both'`)
	s.db.Exec(`ALTER TABLE items ADD COLUMN name_en TEXT DEFAULT ''`)
	s.db.Exec(`ALTER TABLE items ADD COLUMN description_en TEXT DEFAULT ''`)

	return nil
}

func (s *SQLite) SaveItem(ctx context.Context, item domain.Item) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO items (id, name, name_en, description, description_en, price, status, thumbnail_url, photo_url, item_url, seller_id, seller_name, category_id, condition_id, condition_name, shipping_payer, num_likes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			name_en = excluded.name_en,
			description = excluded.description,
			description_en = excluded.description_en,
			price = excluded.price,
			status = excluded.status,
			thumbnail_url = excluded.thumbnail_url,
			photo_url = excluded.photo_url,
			seller_name = excluded.seller_name,
			condition_name = excluded.condition_name,
			num_likes = excluded.num_likes,
			updated_at = excluded.updated_at`,
		item.ID, item.Name, item.NameEN, item.Description, item.DescriptionEN,
		item.Price, item.Status,
		item.ThumbnailURL, item.PhotoURL, item.ItemURL,
		item.SellerID, item.SellerName, item.CategoryID,
		item.ConditionID, item.Condition, item.ShippingPayer,
		item.NumLikes, item.CreatedAt, item.UpdatedAt,
	)
	return err
}

func (s *SQLite) GetItem(ctx context.Context, id string) (*domain.Item, error) {
	row := s.db.QueryRowContext(ctx, itemSelectCols+` FROM items WHERE id = ?`, id)
	item, err := scanItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *SQLite) GetItemsBySearchID(ctx context.Context, searchID int64) ([]domain.Item, error) {
	rows, err := s.db.QueryContext(ctx,
		itemSelectColsPrefixed("i")+`
		 FROM items i
		 JOIN search_items si ON si.item_id = i.id
		 WHERE si.search_id = ?`, searchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

func (s *SQLite) SavePriceRecord(ctx context.Context, record domain.PriceRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO price_history (item_id, price, recorded_at) VALUES (?, ?, ?)`,
		record.ItemID, record.Price, record.RecordedAt)
	return err
}

func (s *SQLite) GetPriceHistory(ctx context.Context, itemID string) ([]domain.PriceRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT item_id, price, recorded_at FROM price_history WHERE item_id = ? ORDER BY recorded_at`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []domain.PriceRecord
	for rows.Next() {
		var r domain.PriceRecord
		if err := rows.Scan(&r.ItemID, &r.Price, &r.RecordedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *SQLite) SaveTrackedSearch(ctx context.Context, ts domain.TrackedSearch) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO tracked_searches (keyword, chat_id) VALUES (?, ?)`,
		ts.Keyword, ts.ChatID)
	if err != nil {
		return 0, err
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		id, err := result.LastInsertId()
		return id, err
	}

	var id int64
	err = s.db.QueryRowContext(ctx,
		`SELECT id FROM tracked_searches WHERE keyword = ? AND chat_id = ?`,
		ts.Keyword, ts.ChatID).Scan(&id)
	return id, err
}

func (s *SQLite) GetTrackedSearches(ctx context.Context) ([]domain.TrackedSearch, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, keyword, chat_id, created_at FROM tracked_searches`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var searches []domain.TrackedSearch
	for rows.Next() {
		var ts domain.TrackedSearch
		if err := rows.Scan(&ts.ID, &ts.Keyword, &ts.ChatID, &ts.CreatedAt); err != nil {
			return nil, err
		}
		searches = append(searches, ts)
	}
	return searches, rows.Err()
}

func (s *SQLite) DeleteTrackedSearch(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tracked_searches WHERE id = ?`, id)
	return err
}

func (s *SQLite) LinkItemToSearch(ctx context.Context, itemID string, searchID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO search_items (search_id, item_id) VALUES (?, ?)`,
		searchID, itemID)
	return err
}

func (s *SQLite) SaveAlertKeyword(ctx context.Context, ak domain.AlertKeyword) error {
	if ak.MatchField == "" {
		ak.MatchField = domain.MatchBoth
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO alert_keywords (search_id, chat_id, keyword, match_field) VALUES (?, ?, ?, ?)`,
		ak.SearchID, ak.ChatID, ak.Keyword, ak.MatchField)
	return err
}

func (s *SQLite) GetAlertsBySearchID(ctx context.Context, searchID int64) ([]domain.AlertKeyword, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, search_id, chat_id, keyword, match_field, created_at FROM alert_keywords WHERE search_id = ?`, searchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlertKeywords(rows)
}

func (s *SQLite) GetAlertsByChatID(ctx context.Context, chatID int64) ([]domain.AlertKeyword, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, search_id, chat_id, keyword, match_field, created_at FROM alert_keywords WHERE chat_id = ?`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAlertKeywords(rows)
}

func (s *SQLite) DeleteAlertKeyword(ctx context.Context, id int64, chatID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM alert_keywords WHERE id = ? AND chat_id = ?`, id, chatID)
	return err
}

func (s *SQLite) GetTrackedSearch(ctx context.Context, id int64) (*domain.TrackedSearch, error) {
	var ts domain.TrackedSearch
	err := s.db.QueryRowContext(ctx,
		`SELECT id, keyword, chat_id, created_at FROM tracked_searches WHERE id = ?`, id).
		Scan(&ts.ID, &ts.Keyword, &ts.ChatID, &ts.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ts, nil
}

func scanAlertKeywords(rows *sql.Rows) ([]domain.AlertKeyword, error) {
	var result []domain.AlertKeyword
	for rows.Next() {
		var ak domain.AlertKeyword
		if err := rows.Scan(&ak.ID, &ak.SearchID, &ak.ChatID, &ak.Keyword, &ak.MatchField, &ak.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, ak)
	}
	return result, rows.Err()
}

const itemSelectCols = `SELECT id, name, name_en, description, description_en, price, status, thumbnail_url, photo_url, item_url, seller_id, seller_name, category_id, condition_id, condition_name, shipping_payer, num_likes, created_at, updated_at`

func itemSelectColsPrefixed(alias string) string {
	cols := []string{"id", "name", "name_en", "description", "description_en", "price", "status", "thumbnail_url", "photo_url", "item_url", "seller_id", "seller_name", "category_id", "condition_id", "condition_name", "shipping_payer", "num_likes", "created_at", "updated_at"}
	prefixed := make([]string, len(cols))
	for i, c := range cols {
		prefixed[i] = alias + "." + c
	}
	return "SELECT " + strings.Join(prefixed, ", ")
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(s scanner) (domain.Item, error) {
	var item domain.Item
	err := s.Scan(&item.ID, &item.Name, &item.NameEN, &item.Description, &item.DescriptionEN,
		&item.Price, &item.Status,
		&item.ThumbnailURL, &item.PhotoURL, &item.ItemURL,
		&item.SellerID, &item.SellerName, &item.CategoryID,
		&item.ConditionID, &item.Condition, &item.ShippingPayer,
		&item.NumLikes, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanItems(rows *sql.Rows) ([]domain.Item, error) {
	var items []domain.Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
