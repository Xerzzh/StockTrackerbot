package main

import (
	"context"
	"log"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot coordina Telegram, el comprobador HTTP y la configuración.
type Bot struct {
	api     *tgbotapi.BotAPI
	fetcher *Fetcher
	cfg     *Config
}

// NewBot construye el bot a partir de sus dependencias.
func NewBot(api *tgbotapi.BotAPI, fetcher *Fetcher, cfg *Config) *Bot {
	return &Bot{api: api, fetcher: fetcher, cfg: cfg}
}

// Run escucha actualizaciones de Telegram hasta que se cancele el contexto.
func (b *Bot) Run(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			b.processUpdate(update)
		}
	}
}

// processUpdate extrae los datos del update y lo enruta al handler apropiado.
func (b *Bot) processUpdate(update tgbotapi.Update) {
	var (
		chatID   int64
		text     string
		cmd      string
		isCmd    bool
		fromUser *tgbotapi.User
	)

	switch {
	case update.CallbackQuery != nil:
		if update.CallbackQuery.Message == nil {
			return
		}
		chatID = update.CallbackQuery.Message.Chat.ID
		cmd = update.CallbackQuery.Data
		isCmd = true
		fromUser = update.CallbackQuery.From
		_, _ = b.api.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
	case update.Message != nil:
		chatID = update.Message.Chat.ID
		text = update.Message.Text
		fromUser = update.Message.From
		if update.Message.IsCommand() {
			cmd = update.Message.Command()
			isCmd = true
		}
	default:
		return
	}

	if !isAuthorized(chatID) {
		log.Printf("⛔ Acceso denegado. ID del usuario: %d", chatID)
		if adminChatID != 0 {
			sendMessage(b.api, adminChatID, getUnauthorizedAdminMessage(chatID))
		}
		return
	}

	config := getUserConfig(chatID)
	if fromUser != nil {
		if fromUser.UserName != "" {
			config.DisplayName = "@" + fromUser.UserName
		} else {
			config.DisplayName = fromUser.FirstName
		}
	}

	config.Mutex.Lock()
	defer config.Mutex.Unlock()

	if isCmd {
		b.handleCommand(config, cmd)
		return
	}
	b.handleState(config, text)
}

// registerBotCommands publica la lista de comandos y configura el botón de
// menú del chat.
func (b *Bot) registerBotCommands() {
	commands := []tgbotapi.BotCommand{
		{Command: "menu", Description: "Mostrar el menú principal"},
		{Command: "add", Description: "Añadir un producto"},
		{Command: "list", Description: "Ver mis productos"},
		{Command: "check", Description: "Comprobar ahora"},
		{Command: "startbot", Description: "Iniciar el monitor"},
		{Command: "stopbot", Description: "Detener el monitor"},
		{Command: "edit", Description: "Editar un producto"},
		{Command: "delete", Description: "Eliminar un producto"},
	}
	if _, err := b.api.Request(tgbotapi.NewSetMyCommands(commands...)); err != nil {
		log.Printf("⚠️ Error registrando comandos: %v", err)
	}

	params := tgbotapi.Params{}
	if err := params.AddInterface("menu_button", map[string]string{"type": "commands"}); err != nil {
		log.Printf("⚠️ Error preparando menu_button: %v", err)
		return
	}
	if _, err := b.api.MakeRequest("setChatMenuButton", params); err != nil {
		log.Printf("⚠️ Error configurando botón de menú: %v", err)
	}
}

// autoStartMonitors arranca los monitores de los usuarios conocidos que ya
// tengan productos configurados.
func (b *Bot) autoStartMonitors() {
	targets := make(map[int64]bool)
	if adminChatID != 0 {
		targets[adminChatID] = true
	}
	authMutex.RLock()
	for id := range authorizedUsers {
		targets[id] = true
	}
	authMutex.RUnlock()

	for id := range targets {
		config := getUserConfig(id)
		if len(config.Products) == 0 {
			continue
		}
		config.IsRunning = true
		config.resetStop()
		go b.monitor(config)
	}
}

var (
	userConfigs  = make(map[int64]*UserConfig)
	configsMutex sync.RWMutex
)

// getUserConfig devuelve (o crea) la configuración para un chatID.
func getUserConfig(chatID int64) *UserConfig {
	configsMutex.Lock()
	defer configsMutex.Unlock()

	if config, exists := userConfigs[chatID]; exists {
		return config
	}
	config := &UserConfig{
		ChatID:    chatID,
		Products:  []Product{},
		EditIndex: -1,
	}
	loadProducts(config)
	userConfigs[chatID] = config
	return config
}
