package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
	mu sync.RWMutex
}

type Favorite struct {
	ID        int64  `json:"id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Duration  int    `json:"duration"`
	Thumbnail string `json:"thumbnail"`
	Author    string `json:"author"`
}

type Collection struct {
	ID        int64            `json:"id"`
	UserID    string           `json:"user_id"`
	Name      string           `json:"name"`
	ShareCode string           `json:"share_code"`
	Items     []CollectionItem `json:"items"`
}

type CollectionItem struct {
	ID           int64  `json:"id"`
	CollectionID int64  `json:"collection_id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Duration     int    `json:"duration"`
	Thumbnail    string `json:"thumbnail"`
	Author       string `json:"author"`
	Position     int    `json:"position"`
}

func Init(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	sqliteDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	database := &DB{DB: sqliteDB}

	// High-performance SQLite PRAGMA tuning for concurrent multi-guild access
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA temp_store=MEMORY;",
		"PRAGMA cache_size=-64000;", // 64MB Cache
		"PRAGMA busy_timeout=5000;",
	}
	for _, p := range pragmas {
		_, _ = sqliteDB.Exec(p)
	}

	if err := database.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return database, nil
}

func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS favorites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			title TEXT NOT NULL,
			url TEXT NOT NULL,
			duration INTEGER NOT NULL,
			thumbnail TEXT DEFAULT '',
			author TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, url)
		);`,
		`CREATE TABLE IF NOT EXISTS collections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			share_code TEXT UNIQUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, name)
		);`,
		`CREATE TABLE IF NOT EXISTS collection_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			collection_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			url TEXT NOT NULL,
			duration INTEGER NOT NULL,
			thumbnail TEXT DEFAULT '',
			author TEXT DEFAULT '',
			position INTEGER NOT NULL,
			FOREIGN KEY(collection_id) REFERENCES collections(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS search_cache (
			query TEXT PRIMARY KEY,
			json_data TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS guild_settings (
			guild_id TEXT PRIMARY KEY,
			language TEXT DEFAULT 'en',
			prefix TEXT DEFAULT '!',
			default_volume INTEGER DEFAULT 100
		);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	// Auto-migrate legacy SQLite databases: ensure missing columns in guild_settings are added
	_, _ = db.Exec(`ALTER TABLE guild_settings ADD COLUMN language TEXT DEFAULT 'en';`)
	_, _ = db.Exec(`ALTER TABLE guild_settings ADD COLUMN prefix TEXT DEFAULT '!';`)
	_, _ = db.Exec(`ALTER TABLE guild_settings ADD COLUMN default_volume INTEGER DEFAULT 100;`)

	return nil
}

// Favorites Operations
func (db *DB) AddFavorite(userID, title, url string, duration int, thumbnail, author string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec(
		`INSERT INTO favorites (user_id, title, url, duration, thumbnail, author) 
		 VALUES (?, ?, ?, ?, ?, ?) 
		 ON CONFLICT(user_id, url) DO UPDATE SET title=excluded.title, duration=excluded.duration, thumbnail=excluded.thumbnail, author=excluded.author`,
		userID, title, url, duration, thumbnail, author,
	)
	return err
}

func (db *DB) GetFavorites(userID string) ([]Favorite, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.Query(`SELECT id, user_id, title, url, duration, thumbnail, author FROM favorites WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favorites []Favorite
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.UserID, &f.Title, &f.URL, &f.Duration, &f.Thumbnail, &f.Author); err != nil {
			return nil, err
		}
		favorites = append(favorites, f)
	}
	return favorites, nil
}

func (db *DB) RemoveFavorite(userID string, index int) error {
	favs, err := db.GetFavorites(userID)
	if err != nil || index < 0 || index >= len(favs) {
		return fmt.Errorf("invalid favorite index")
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	_, err = db.Exec(`DELETE FROM favorites WHERE id = ?`, favs[index].ID)
	return err
}

// Collections Operations
func (db *DB) CreateCollection(userID, name string) (*Collection, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	res, err := db.Exec(`INSERT INTO collections (user_id, name) VALUES (?, ?)`, userID, name)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Collection{ID: id, UserID: userID, Name: name}, nil
}

func (db *DB) GetUserCollections(userID string) ([]Collection, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.Query(`SELECT id, user_id, name, COALESCE(share_code, '') FROM collections WHERE user_id = ? ORDER BY name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collections []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.ShareCode); err != nil {
			return nil, err
		}
		collections = append(collections, c)
	}
	return collections, nil
}

func (db *DB) GetCollectionItems(collectionID int64) ([]CollectionItem, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.Query(`SELECT id, collection_id, title, url, duration, thumbnail, author, position FROM collection_items WHERE collection_id = ? ORDER BY position ASC`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CollectionItem
	for rows.Next() {
		var item CollectionItem
		if err := rows.Scan(&item.ID, &item.CollectionID, &item.Title, &item.URL, &item.Duration, &item.Thumbnail, &item.Author, &item.Position); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (db *DB) AddToCollection(collectionID int64, title, url string, duration int, thumbnail, author string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var maxPos int
	_ = db.QueryRow(`SELECT COALESCE(MAX(position), 0) FROM collection_items WHERE collection_id = ?`, collectionID).Scan(&maxPos)

	_, err := db.Exec(
		`INSERT INTO collection_items (collection_id, title, url, duration, thumbnail, author, position) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		collectionID, title, url, duration, thumbnail, author, maxPos+1,
	)
	return err
}

func (db *DB) DeleteCollection(userID, name string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec(`DELETE FROM collections WHERE user_id = ? AND name = ?`, userID, name)
	return err
}

// Search Cache
func (db *DB) SetSearchCache(query, jsonData string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec(
		`INSERT INTO search_cache (query, json_data) VALUES (?, ?) ON CONFLICT(query) DO UPDATE SET json_data=excluded.json_data, updated_at=CURRENT_TIMESTAMP`,
		query, jsonData,
	)
	return err
}

func (db *DB) GetSearchCache(query string) (string, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var jsonData string
	err := db.QueryRow(`SELECT json_data FROM search_cache WHERE query = ?`, query).Scan(&jsonData)
	if err != nil {
		return "", false
	}
	return jsonData, true
}

func (db *DB) SetGuildLanguage(guildID, lang string) error {
	if db == nil || db.DB == nil {
		return fmt.Errorf("database connection is nil")
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	// Ensure guild_settings table and 'language' column exist (resilient check for legacy SQLite databases)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS guild_settings (
		guild_id TEXT PRIMARY KEY,
		language TEXT DEFAULT 'en',
		prefix TEXT DEFAULT '!',
		default_volume INTEGER DEFAULT 100
	);`)
	_, _ = db.Exec(`ALTER TABLE guild_settings ADD COLUMN language TEXT DEFAULT 'en';`)

	_, err := db.Exec(
		`INSERT INTO guild_settings (guild_id, language) VALUES (?, ?) 
		 ON CONFLICT(guild_id) DO UPDATE SET language=excluded.language`,
		guildID, lang,
	)
	if err != nil {
		_, err = db.Exec(
			`INSERT OR REPLACE INTO guild_settings (guild_id, language) VALUES (?, ?)`,
			guildID, lang,
		)
	}
	return err
}

func (db *DB) GetGuildLanguage(guildID string) string {
	if db == nil || db.DB == nil {
		return "en"
	}
	db.mu.RLock()
	defer db.mu.RUnlock()

	var lang string
	err := db.QueryRow(`SELECT language FROM guild_settings WHERE guild_id = ?`, guildID).Scan(&lang)
	if err != nil || lang == "" {
		return "en"
	}
	return lang
}
