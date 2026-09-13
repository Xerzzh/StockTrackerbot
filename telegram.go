package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleCommand procesa comandos y callbacks (botones inline).
func (b *Bot) handleCommand(config *UserConfig, cmd string) {
	switch {
	case strings.HasPrefix(cmd, "edit_"):
		b.handleEditSelect(config, strings.TrimPrefix(cmd, "edit_"))
		return
	case strings.HasPrefix(cmd, "del_"):
		b.handleDelete(config, strings.TrimPrefix(cmd, "del_"))
		return
	}

	switch cmd {
	case "start", "menu":
		b.showMainMenu(config)
	case "add", "set":
		b.startAdd(config)
	case "addurl_done":
		b.finishAddURLs(config)
	case "list":
		b.showProductList(config)
	case "check":
		b.manualCheck(config)
	case "startbot":
		b.handleStartBot(config)
	case "stopbot":
		b.handleStopBot(config)
	case "edit":
		b.showEditList(config)
	case "delete":
		b.showDeleteList(config)
	case "ef_name":
		config.BotState = StateEditName
		sendMessage(b.api, config.ChatID, "✏️ Envía el nuevo nombre del producto:")
	case "ef_url":
		config.BotState = StateEditURL
		sendMessage(b.api, config.ChatID,
			"✏️ Envía la nueva URL del producto (o varias separadas por espacios).\n\nTiendas soportadas: "+SupportedStoresList())
	case "ef_interval":
		config.BotState = StateEditInterval
		sendMessage(b.api, config.ChatID, "✏️ Envía el nuevo intervalo en minutos (ej: 5):")
	case "admin_add":
		if config.ChatID == adminChatID {
			config.BotState = StateAdminAddUser
			sendMessage(b.api, config.ChatID, "👤 Envía el ID del usuario que quieres autorizar:")
		}
	case "admin_del":
		if config.ChatID == adminChatID {
			config.BotState = StateAdminDelUser
			sendMessage(b.api, config.ChatID, "🚫 Envía el ID del usuario a revocar:")
		}
	case "admin_list":
		if config.ChatID == adminChatID {
			sendMessage(b.api, config.ChatID, getAuthorizedUsersList())
		}
	default:
		sendMessage(b.api, config.ChatID, "Comando no reconocido. Usa /menu.")
	}
}

// handleState procesa mensajes de texto según el estado de la conversación.
func (b *Bot) handleState(config *UserConfig, text string) {
	text = strings.TrimSpace(text)

	switch config.BotState {
	case StateAddName:
		if text == "" {
			sendMessage(b.api, config.ChatID, "⚠️ El nombre no puede estar vacío.")
			return
		}
		config.TempName = text
		config.TempSources = nil
		config.BotState = StateAddURL
		sendMessage(b.api, config.ChatID,
			"2/3 · Envía una o varias URLs del producto.\n"+
				"Puedes añadirlas en varias tiendas y recibirás una sola notificación.\n"+
				"Envía las URLs una a una o varias separadas por espacios y pulsa ✅ Listo.\n\n"+
				"Tiendas soportadas: "+SupportedStoresList())

	case StateAddURL:
		added, problems := addTempSources(config, text)
		if added == 0 {
			msg := "⚠️ No se añadió ninguna URL."
			if len(problems) > 0 {
				msg += "\n\n" + strings.Join(problems, "\n")
			}
			msg += "\n\nTiendas soportadas: " + SupportedStoresList()
			sendMessage(b.api, config.ChatID, msg)
			return
		}
		b.showAddURLSummary(config, problems)

	case StateAddInterval:
		min, err := parseInterval(text)
		if err != nil {
			sendMessage(b.api, config.ChatID, "⚠️ "+err.Error())
			return
		}
		sources := make([]Source, len(config.TempSources))
		copy(sources, config.TempSources)
		config.Products = append(config.Products, Product{
			Name:        config.TempName,
			Sources:     sources,
			IntervalMin: min,
			LastStatus:  StatusUnknown,
			NextRun:     time.Now(),
		})
		saveProducts(config)
		resetTemp(config)
		config.BotState = StateIdle
		sendMessage(b.api, config.ChatID, "✅ Producto añadido.")
		b.showMainMenu(config)

	case StateEditName:
		if config.inRange() {
			config.Products[config.EditIndex].Name = text
			saveProducts(config)
			sendMessage(b.api, config.ChatID, "✅ Nombre actualizado.")
		}
		config.BotState = StateIdle
		b.showEditMenu(config)

	case StateEditURL:
		sources, err := parseSources(text)
		if err != nil {
			sendMessage(b.api, config.ChatID,
				"⚠️ "+err.Error()+"\n\nTiendas soportadas: "+SupportedStoresList())
			return
		}
		if config.inRange() {
			config.Products[config.EditIndex].Sources = sources
			config.Products[config.EditIndex].NextRun = time.Now()
			saveProducts(config)
			sendMessage(b.api, config.ChatID, "✅ URLs actualizadas.")
		}
		config.BotState = StateIdle
		b.showEditMenu(config)

	case StateEditInterval:
		min, err := parseInterval(text)
		if err != nil {
			sendMessage(b.api, config.ChatID, "⚠️ "+err.Error())
			return
		}
		if config.inRange() {
			config.Products[config.EditIndex].IntervalMin = min
			config.Products[config.EditIndex].NextRun = time.Now().Add(time.Duration(min) * time.Minute)
			saveProducts(config)
			sendMessage(b.api, config.ChatID, "✅ Intervalo actualizado.")
		}
		config.BotState = StateIdle
		b.showEditMenu(config)

	case StateAdminAddUser:
		id, err := parseChatID(text)
		if err != nil {
			sendMessage(b.api, config.ChatID, "⚠️ ID no válido.")
			return
		}
		authorizeUser(id)
		sendMessage(b.api, config.ChatID, fmt.Sprintf("✅ Usuario %d autorizado.", id))
		config.BotState = StateIdle
		b.showMainMenu(config)

	case StateAdminDelUser:
		id, err := parseChatID(text)
		if err != nil {
			sendMessage(b.api, config.ChatID, "⚠️ ID no válido.")
			return
		}
		revokeUser(id)
		sendMessage(b.api, config.ChatID, fmt.Sprintf("✅ Permisos revocados para %d.", id))
		config.BotState = StateIdle
		b.showMainMenu(config)

	default:
		sendMessage(b.api, config.ChatID, "No entiendo ese mensaje. Usa /menu para ver las opciones.")
	}
}

// --- Alta ---

func (b *Bot) startAdd(config *UserConfig) {
	resetTemp(config)
	config.BotState = StateAddName
	sendMessage(b.api, config.ChatID,
		"➕ Nueva búsqueda\n\n1/3 · Envía un nombre para identificarla (ej: Switch 2 Zelda):")
}

func resetTemp(config *UserConfig) {
	config.TempName = ""
	config.TempSources = nil
	config.TempInterval = 0
}

// addTempSources añade a las fuentes temporales las URLs del texto. Devuelve
// cuántas se añadieron y los problemas de las que no se pudieron detectar.
func addTempSources(config *UserConfig, text string) (int, []string) {
	seen := make(map[string]bool, len(config.TempSources))
	for _, s := range config.TempSources {
		seen[s.URL] = true
	}
	var added int
	var problems []string
	for _, raw := range strings.Fields(text) {
		store, err := DetectStore(raw)
		if err != nil {
			problems = append(problems, "⚠️ "+err.Error())
			continue
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		config.TempSources = append(config.TempSources, Source{URL: raw, Store: store.Key})
		added++
	}
	return added, problems
}

// parseSources convierte texto (una o varias URLs) en fuentes válidas.
func parseSources(text string) ([]Source, error) {
	var sources []Source
	seen := make(map[string]bool)
	var firstErr error
	for _, raw := range strings.Fields(text) {
		store, err := DetectStore(raw)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		sources = append(sources, Source{URL: raw, Store: store.Key})
	}
	if len(sources) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("no se encontró ninguna URL válida")
	}
	return sources, nil
}

// showAddURLSummary muestra las URLs acumuladas y el botón para continuar.
func (b *Bot) showAddURLSummary(config *UserConfig, problems []string) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🔗 URLs añadidas (%d):\n", len(config.TempSources))
	for _, s := range config.TempSources {
		fmt.Fprintf(&sb, "• %s · %s\n", StoreLabel(s.Store), s.URL)
	}
	if len(problems) > 0 {
		sb.WriteString("\n" + strings.Join(problems, "\n") + "\n")
	}
	sb.WriteString("\nEnvía más URLs o pulsa ✅ Listo para continuar.")
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Listo", "addurl_done"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancelar", "menu"),
		),
	)
	sendKeyboard(b.api, config.ChatID, sb.String(), keyboard)
}

// finishAddURLs avanza al paso del intervalo una vez recogidas las URLs.
func (b *Bot) finishAddURLs(config *UserConfig) {
	if len(config.TempSources) == 0 {
		sendMessage(b.api, config.ChatID, "⚠️ Añade al menos una URL antes de continuar.")
		return
	}
	config.BotState = StateAddInterval
	sendMessage(b.api, config.ChatID, fmt.Sprintf(
		"3/3 · ¿Cada cuántos minutos quieres comprobarlo? (ej: 5)\n\nTiendas: %s",
		sourcesStoreLabel(config.TempSources)))
}

// --- Listados y menús ---

func (b *Bot) showMainMenu(config *UserConfig) {
	config.BotState = StateIdle
	config.EditIndex = -1

	state := "🔴 Detenido"
	if config.IsRunning {
		state = "🟢 En ejecución"
	}
	text := fmt.Sprintf("👋 Hola %s\nTienes %d producto(s) configurado(s).\nMonitor: %s",
		config.DisplayName, len(config.Products), state)

	keyboard := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("➕ Añadir", "add"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Listar", "list"),
			tgbotapi.NewInlineKeyboardButtonData("🔎 Comprobar", "check"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("✏️ Editar", "edit"),
			tgbotapi.NewInlineKeyboardButtonData("🗑 Eliminar", "delete"),
		},
		{
			tgbotapi.NewInlineKeyboardButtonData("▶️ Iniciar", "startbot"),
			tgbotapi.NewInlineKeyboardButtonData("⏹ Detener", "stopbot"),
		},
	}
	if config.ChatID == adminChatID {
		keyboard = append(keyboard,
			[]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("👤 Autorizar ID", "admin_add"),
				tgbotapi.NewInlineKeyboardButtonData("🚫 Revocar ID", "admin_del"),
			},
			[]tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData("📋 Listar IDs", "admin_list"),
			},
		)
	}
	sendKeyboard(b.api, config.ChatID, text, tgbotapi.NewInlineKeyboardMarkup(keyboard...))
}

func (b *Bot) showProductList(config *UserConfig) {
	if len(config.Products) == 0 {
		sendMessage(b.api, config.ChatID, "No tienes productos configurados. Usa /add para añadir uno.")
		return
	}
	sendMessage(b.api, config.ChatID, formatStatusReport(config.Products))
	b.showMainMenu(config)
}

func (b *Bot) showEditList(config *UserConfig) {
	if len(config.Products) == 0 {
		sendMessage(b.api, config.ChatID, "No hay productos que editar.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for i, p := range config.Products {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ "+p.Name, fmt.Sprintf("edit_%d", i)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Volver", "menu"),
	))
	sendKeyboard(b.api, config.ChatID, "Selecciona el producto a editar:", tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func (b *Bot) showDeleteList(config *UserConfig) {
	if len(config.Products) == 0 {
		sendMessage(b.api, config.ChatID, "No hay productos que eliminar.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for i, p := range config.Products {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 "+p.Name, fmt.Sprintf("del_%d", i)),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Volver", "menu"),
	))
	sendKeyboard(b.api, config.ChatID, "Selecciona el producto a eliminar:", tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func (b *Bot) showEditMenu(config *UserConfig) {
	if !config.inRange() {
		b.showMainMenu(config)
		return
	}
	p := config.Products[config.EditIndex]
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏷 Nombre", "ef_name"),
			tgbotapi.NewInlineKeyboardButtonData("🔗 URLs", "ef_url"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏱ Intervalo", "ef_interval"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Volver", "menu"),
		),
	)
	sendKeyboard(b.api, config.ChatID, formatProductDetail(p), keyboard)
}

func (b *Bot) handleEditSelect(config *UserConfig, raw string) {
	idx, err := parseIndex(raw)
	if err != nil || idx < 0 || idx >= len(config.Products) {
		return
	}
	config.EditIndex = idx
	config.BotState = StateIdle
	b.showEditMenu(config)
}

func (b *Bot) handleDelete(config *UserConfig, raw string) {
	idx, err := parseIndex(raw)
	if err != nil || idx < 0 || idx >= len(config.Products) {
		return
	}
	name := config.Products[idx].Name
	config.Products = append(config.Products[:idx], config.Products[idx+1:]...)
	if config.EditIndex == idx {
		config.EditIndex = -1
	}
	saveProducts(config)
	sendMessage(b.api, config.ChatID, fmt.Sprintf("🗑 Producto «%s» eliminado.", name))
	b.showMainMenu(config)
}

// --- Control del monitor ---

func (b *Bot) handleStartBot(config *UserConfig) {
	if len(config.Products) == 0 {
		sendMessage(b.api, config.ChatID, "⚠️ No tienes productos configurados.")
		return
	}
	if config.IsRunning {
		sendMessage(b.api, config.ChatID, "ℹ️ El monitor ya está en ejecución.")
		return
	}
	config.IsRunning = true
	config.resetStop()
	go b.monitor(config)
	sendMessage(b.api, config.ChatID, "🚀 Monitor iniciado.")
}

func (b *Bot) handleStopBot(config *UserConfig) {
	if !config.IsRunning {
		sendMessage(b.api, config.ChatID, "ℹ️ El monitor ya está detenido.")
		return
	}
	config.IsRunning = false
	config.stopMonitoring()
	sendMessage(b.api, config.ChatID, "⏹ Monitor detenido.")
}

// manualCheck lanza una comprobación inmediata sin bloquear el handler.
func (b *Bot) manualCheck(config *UserConfig) {
	if len(config.Products) == 0 {
		sendMessage(b.api, config.ChatID, "No tienes productos configurados.")
		return
	}
	sendMessage(b.api, config.ChatID, "🔎 Comprobando productos...")
	go func() {
		products := b.checkUserProducts(config, true)
		sendMessage(b.api, config.ChatID, formatStatusReport(products))
	}()
}

func parseIndex(raw string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(raw))
}

func parseChatID(raw string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
}
