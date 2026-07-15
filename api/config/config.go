package config

import (
	"os"
)

type Config struct {
	Port       string
	RedisAddr  string
	SQLitePath string
    MusicApi   string
}

func Load() *Config {
	return &Config{
		Port:       getEnv("PORT", "8080"),
		RedisAddr:  getEnv("REDIS_ADDR", "localhost:6379"),
		SQLitePath: getEnv("SQLITE_PATH", "players.db"),
        MusicApi:   getEnv("MUSIC_API", "https://music-api.xxx"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
