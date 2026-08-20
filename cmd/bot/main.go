package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/bot"
	"aetrna-music/internal/version"
)

func main() {
	bot.InitLogCapture()
	log.Printf("[INFO] Starting aetrna-music %s (Golang Modular Engine)...", version.AppVersion)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	version.StartBackgroundChecker(ctx)

	cfg := config.Load()
	if cfg.DiscordToken == "" {
		log.Println("[WARN] DISCORD_TOKEN is missing in environment variables or .env file.")
		log.Println("[INFO] Please copy .env.example to .env and set DISCORD_TOKEN.")
	}

	database, err := db.Init(cfg.DBPath)
	if err != nil {
		log.Fatalf("[ERROR] Failed to initialize SQLite database: %v", err)
	}
	defer database.Close()
	log.Printf("[INFO] SQLite database connected: %s", cfg.DBPath)

	musicBot, err := bot.New(cfg, database)
	if err != nil {
		log.Fatalf("[ERROR] Failed to initialize bot: %v", err)
	}

	if cfg.DiscordToken != "" {
		if err := musicBot.Start(); err != nil {
			log.Fatalf("[ERROR] Failed to start bot session: %v", err)
		}
		defer musicBot.Stop()
	} else {
		log.Println("[WARN] Bot initialized in dry-run mode (waiting for DISCORD_TOKEN).")
	}

	log.Println("[INFO] Bot engine initialized successfully. Press Ctrl+C to exit.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("[INFO] Shutting down gracefully...")
}
