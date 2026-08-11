package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"aetrna-music/config"
	"aetrna-music/db"
	"aetrna-music/internal/bot"
)

func main() {
	log.Println("🚀 Starting aetrna-music (Golang Modular Engine v2.0)...")

	cfg := config.Load()
	if cfg.DiscordToken == "" {
		log.Println("⚠️ DISCORD_TOKEN is missing in environment variables or .env file.")
		log.Println("ℹ️ Please copy .env.example to .env and set DISCORD_TOKEN.")
	}

	database, err := db.Init(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ Failed to initialize SQLite database: %v", err)
	}
	defer database.Close()
	log.Printf("✅ SQLite database connected: %s", cfg.DBPath)

	musicBot, err := bot.New(cfg, database)
	if err != nil {
		log.Fatalf("❌ Failed to initialize bot: %v", err)
	}

	if cfg.DiscordToken != "" {
		if err := musicBot.Start(); err != nil {
			log.Fatalf("❌ Failed to start bot session: %v", err)
		}
		defer musicBot.Stop()
	} else {
		log.Println("⚠️ Bot initialized in dry-run mode (waiting for DISCORD_TOKEN).")
	}

	log.Println("✅ Bot engine initialized successfully. Press Ctrl+C to exit.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("🧹 Shutting down gracefully...")
}
