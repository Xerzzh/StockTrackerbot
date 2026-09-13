package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	log.SetFlags(log.LstdFlags)

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("configuración: %v", err)
	}

	configDir = cfg.StateDir
	adminChatID = cfg.AdminChatID
	loadAuthorizedUsers()

	fetcher := NewFetcher(cfg.HTTPTimeout)

	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		log.Fatalf("telegram: %v", err)
	}
	log.Printf("conectado como @%s", api.Self.UserName)

	bot := NewBot(api, fetcher, cfg)
	bot.registerBotCommands()
	bot.autoStartMonitors()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bot.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("bot: %v", err)
	}
	log.Println("bot detenido")
}
