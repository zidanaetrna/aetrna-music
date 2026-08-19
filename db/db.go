package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	Source       string `json:"source"`
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
			source TEXT DEFAULT 'youtube',
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
		`CREATE TABLE IF NOT EXISTS queue_snapshots (
			guild_id TEXT PRIMARY KEY,
			voice_channel_id TEXT NOT NULL,
			text_channel_id TEXT NOT NULL,
			now_playing_title TEXT DEFAULT '',
			now_playing_url TEXT DEFAULT '',
			now_playing_author TEXT DEFAULT '',
			now_playing_thumbnail TEXT DEFAULT '',
			now_playing_duration INTEGER DEFAULT 0,
			position_ms INTEGER DEFAULT 0,
			position_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			generation INTEGER DEFAULT 1,
			songs_json TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS web_sessions (
			token TEXT PRIMARY KEY,
			expires_at TIMESTAMP NOT NULL
		);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	// Auto-migrate legacy SQLite databases: ensure missing columns in collection_items & guild_settings are added
	_, _ = db.Exec(`ALTER TABLE collection_items ADD COLUMN source TEXT DEFAULT 'youtube';`)
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

func (db *DB) GetCollectionByName(userID, name string) (*Collection, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var c Collection
	err := db.QueryRow(`SELECT id, user_id, name, COALESCE(share_code, '') FROM collections WHERE user_id = ? AND name = ?`, userID, name).Scan(&c.ID, &c.UserID, &c.Name, &c.ShareCode)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (db *DB) GetCollectionItems(collectionID int64) ([]CollectionItem, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.Query(`SELECT id, collection_id, title, url, COALESCE(source, 'youtube'), duration, thumbnail, author, position FROM collection_items WHERE collection_id = ? ORDER BY position ASC`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CollectionItem
	for rows.Next() {
		var item CollectionItem
		if err := rows.Scan(&item.ID, &item.CollectionID, &item.Title, &item.URL, &item.Source, &item.Duration, &item.Thumbnail, &item.Author, &item.Position); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (db *DB) AddToCollection(collectionID int64, title, url string, duration int, thumbnail, author string) error {
	return db.AddToCollectionWithSource(collectionID, title, url, "youtube", duration, thumbnail, author)
}

func (db *DB) AddToCollectionWithSource(collectionID int64, title, url, source string, duration int, thumbnail, author string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if source == "" {
		source = "youtube"
	}

	var maxPos int
	_ = db.QueryRow(`SELECT COALESCE(MAX(position), 0) FROM collection_items WHERE collection_id = ?`, collectionID).Scan(&maxPos)

	_, err := db.Exec(
		`INSERT INTO collection_items (collection_id, title, url, source, duration, thumbnail, author, position) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		collectionID, title, url, source, duration, thumbnail, author, maxPos+1,
	)
	return err
}

func (db *DB) RemoveFromCollectionItem(collectionID int64, itemID int64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec(`DELETE FROM collection_items WHERE collection_id = ? AND id = ?`, collectionID, itemID)
	return err
}

func (db *DB) ReorderCollectionItems(collectionID int64, itemIDs []int64) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`UPDATE collection_items SET position = ? WHERE collection_id = ? AND id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for pos, id := range itemIDs {
		if _, err := stmt.Exec(pos+1, collectionID, id); err != nil {
			return err
		}
	}

	return tx.Commit()
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

type QueueSnapshot struct {
	GuildID             string    `json:"guild_id"`
	VoiceChannelID      string    `json:"voice_channel_id"`
	TextChannelID       string    `json:"text_channel_id"`
	NowPlayingTitle     string    `json:"now_playing_title"`
	NowPlayingURL       string    `json:"now_playing_url"`
	NowPlayingAuthor    string    `json:"now_playing_author"`
	NowPlayingThumbnail string    `json:"now_playing_thumbnail"`
	NowPlayingDuration  int       `json:"now_playing_duration"`
	PositionMs          int64     `json:"position_ms"`
	PositionAt          time.Time `json:"position_at"`
	Generation          uint64    `json:"generation"`
	SongsJSON           string    `json:"songs_json"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (db *DB) SaveQueueSnapshot(s QueueSnapshot) error {
	if db == nil || db.DB == nil {
		return nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	posAtStr := s.PositionAt.Format(time.RFC3339)
	if s.PositionAt.IsZero() {
		posAtStr = time.Now().Format(time.RFC3339)
	}

	_, err := db.Exec(
		`INSERT INTO queue_snapshots (
			guild_id, voice_channel_id, text_channel_id, 
			now_playing_title, now_playing_url, now_playing_author, now_playing_thumbnail, now_playing_duration,
			position_ms, position_at, generation, songs_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(guild_id) DO UPDATE SET
			voice_channel_id=excluded.voice_channel_id,
			text_channel_id=excluded.text_channel_id,
			now_playing_title=excluded.now_playing_title,
			now_playing_url=excluded.now_playing_url,
			now_playing_author=excluded.now_playing_author,
			now_playing_thumbnail=excluded.now_playing_thumbnail,
			now_playing_duration=excluded.now_playing_duration,
			position_ms=excluded.position_ms,
			position_at=excluded.position_at,
			generation=excluded.generation,
			songs_json=excluded.songs_json,
			updated_at=CURRENT_TIMESTAMP`,
		s.GuildID, s.VoiceChannelID, s.TextChannelID,
		s.NowPlayingTitle, s.NowPlayingURL, s.NowPlayingAuthor, s.NowPlayingThumbnail, s.NowPlayingDuration,
		s.PositionMs, posAtStr, s.Generation, s.SongsJSON,
	)
	return err
}

func (db *DB) GetAllQueueSnapshots() ([]QueueSnapshot, error) {
	if db == nil || db.DB == nil {
		return nil, nil
	}
	db.mu.RLock()
	defer db.mu.RUnlock()

	rows, err := db.Query(`SELECT guild_id, voice_channel_id, text_channel_id, now_playing_title, now_playing_url, now_playing_author, now_playing_thumbnail, now_playing_duration, position_ms, position_at, generation, songs_json FROM queue_snapshots ORDER BY generation DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QueueSnapshot
	for rows.Next() {
		var s QueueSnapshot
		var posAt string
		if err := rows.Scan(
			&s.GuildID, &s.VoiceChannelID, &s.TextChannelID,
			&s.NowPlayingTitle, &s.NowPlayingURL, &s.NowPlayingAuthor, &s.NowPlayingThumbnail, &s.NowPlayingDuration,
			&s.PositionMs, &posAt, &s.Generation, &s.SongsJSON,
		); err != nil {
			return nil, err
		}
		s.PositionAt, _ = time.Parse(time.RFC3339, posAt)
		list = append(list, s)
	}
	return list, nil
}

func (db *DB) DeleteQueueSnapshot(guildID string) error {
	if db == nil || db.DB == nil {
		return nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec(`DELETE FROM queue_snapshots WHERE guild_id = ?`, guildID)
	return err
}

func (db *DB) SaveWebSession(token string, expiresAt time.Time) error {
	if db == nil || db.DB == nil {
		return nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec(`INSERT OR REPLACE INTO web_sessions (token, expires_at) VALUES (?, ?)`, token, expiresAt.Format(time.RFC3339))
	return err
}

func (db *DB) IsValidWebSession(token string) bool {
	if db == nil || db.DB == nil || token == "" {
		return false
	}
	db.mu.RLock()
	defer db.mu.RUnlock()

	var expStr string
	err := db.QueryRow(`SELECT expires_at FROM web_sessions WHERE token = ?`, token).Scan(&expStr)
	if err != nil {
		return false
	}

	exp, err := time.Parse(time.RFC3339, expStr)
	if err != nil {
		return false
	}

	if time.Now().After(exp) {
		go func() {
			db.mu.Lock()
			defer db.mu.Unlock()
			_, _ = db.Exec(`DELETE FROM web_sessions WHERE token = ?`, token)
		}()
		return false
	}

	return true
}

func (db *DB) DeleteWebSession(token string) error {
	if db == nil || db.DB == nil {
		return nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.Exec(`DELETE FROM web_sessions WHERE token = ?`, token)
	return err
}
