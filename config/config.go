package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken        string
	SpotifyClientID     string
	SpotifyClientSecret string
	AdminKey            string
	OwnerID             string
	Prefix              string
	MaxQueueSize        int
	MaxPlaylistSize     int
	DefaultVolume       float64
	DBPath              string
	CacheDir            string
	MaxCacheSizeMB      int64
	YtdlpClients        string
	CookiesPath         string
	InternalIPCToken    string
}

func Load() *Config {
	_ = godotenv.Load() // Ignore error if .env doesn't exist

	return &Config{
		DiscordToken:        getEnv("DISCORD_TOKEN", ""),
		SpotifyClientID:     getEnv("SPOTIFY_CLIENT_ID", ""),
		SpotifyClientSecret: getEnv("SPOTIFY_CLIENT_SECRET", ""),
		AdminKey:            getEnv("ADMIN_KEY", getEnv("DASHBOARD_PASSWORD", "")),
		OwnerID:             getEnv("OWNER_ID", ""),
		Prefix:              getEnv("PREFIX", "!"),
		MaxQueueSize:        getEnvAsInt("MAX_QUEUE_SIZE", 100),
		MaxPlaylistSize:     getEnvAsInt("MAX_PLAYLIST_SIZE", 50),
		DefaultVolume:       getEnvAsFloat("DEFAULT_VOLUME", 1.0),
		DBPath:              getEnv("DB_PATH", "./data/aetrna.db"),
		CacheDir:            getEnv("CACHE_DIR", "./data/cache"),
		MaxCacheSizeMB:      int64(getEnvAsInt("MAX_CACHE_SIZE_MB", 5120)), // Default 5GB
		YtdlpClients:        getEnv("YTDLP_CLIENTS", "ios,android"),
		CookiesPath:         getEnv("COOKIES_PATH", "./cookies.txt"),
		InternalIPCToken:    getEnv("INTERNAL_IPC_TOKEN", "aetrna-internal-ipc-token-dev-2026"),
	}
}

func getEnv(key, defaultVal string) string {
	if val, exists := os.LookupEnv(key); exists && val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	if valStr, exists := os.LookupEnv(key); exists {
		if val, err := strconv.Atoi(valStr); err == nil {
			return val
		}
	}
	return defaultVal
}

func getEnvAsFloat(key string, defaultVal float64) float64 {
	if valStr, exists := os.LookupEnv(key); exists {
		if val, err := strconv.ParseFloat(valStr, 64); err == nil {
			return val
		}
	}
	return defaultVal
}
