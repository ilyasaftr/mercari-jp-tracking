package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ilyasaftr/mercari-price-tracking/internal/bot"
	"github.com/ilyasaftr/mercari-price-tracking/internal/mercari"
	"github.com/ilyasaftr/mercari-price-tracking/internal/notifier"
	"github.com/ilyasaftr/mercari-price-tracking/internal/storage"
	"github.com/ilyasaftr/mercari-price-tracking/internal/tracker"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	token := getEnv("TELEGRAM_BOT_TOKEN", "")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required")
	}

	dbPath := getEnv("DB_PATH", "mercari.db")
	checkInterval := getEnvDuration("CHECK_INTERVAL", 30*time.Minute)

	repo, err := storage.NewSQLite(dbPath)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	defer repo.Close()

	mercariClient, err := mercari.NewClient()
	if err != nil {
		log.Fatalf("init mercari client: %v", err)
	}

	botAPI, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}

	tg := notifier.NewTelegram(botAPI)
	svc := tracker.NewService(mercariClient, repo, tg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go svc.RunScheduler(ctx, checkInterval)

	handler := bot.NewHandler(botAPI, svc)
	log.Printf("bot started as @%s", botAPI.Self.UserName)
	handler.Run(ctx)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
