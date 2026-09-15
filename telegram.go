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
	case strings.HasPrefix(cmd, "src_"), cmd == "ef_back":
		b.handleSourceCallback(config, cmd)
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
		config.BotState = StateIdle
		config.SourceSelection = nil
		b.showSourceManager(config)
	case "ef_interval":
		config.BotState = StateEditInterval
		sendMessage(b.api, config.ChatID, "✏️ Envía el nuevo intervalo en minutos o segundos, sin mínimo (ej: 1s, 5s, 30s, 2m):")
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
		interval, err := parseInterval(text)
		if err != nil {
			sendMessage(b.api, config.ChatID, "⚠️ "+err.Error())
			return
		}
		sources := make([]Source, len(config.TempSources))
		copy(sources, config.TempSources)
		config.Products = append(config.Products, Product{
			Name:        config.TempName,
			Sources:     sources,
			IntervalSec: int(interval / time.Second),
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

	case StateEditURLAdd:
		if !config.inRange() {
			config.BotState = StateIdle
			b.showMainMenu(config)
			return
		}
		added, problems := addProductSources(config, text)
		if added == 0 {
			msg := "⚠️ No se añadió ninguna URL."
			if len(problems) > 0 {
				msg += "\n\n" + strings.Join(problems, "\n")
			}
			msg += "\n\nTiendas soportadas: " + SupportedStoresList()
			sendMessage(b.api, config.ChatID, msg)
			return
		}
		p := &config.Products[config.EditIndex]
		p.NextRun = time.Now()
		saveProducts(config)
		config.BotState = StateIdle
		msg := fmt.Sprintf("✅ %d URL(s) añadida(s).", added)
		if len(problems) > 0 {
			msg += "\n" + strings.Join(problems, "\n")
		}
		sendMessage(b.api, config.ChatID, msg)
		b.showSourceManager(config)

	case StateEditInterval:
		interval, err := parseInterval(text)
		if err != nil {
			sendMessage(b.api, config.ChatID, "⚠️ "+err.Error())
			return
		}
		if config.inRange() {
			config.Products[config.EditIndex].IntervalSec = int(interval / time.Second)
			config.Products[config.EditIndex].NextRun = time.Now().Add(interval)
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
	config.TempIntervalSec = 0
}

// addTempSources añade a las fuentes temporales las URLs del texto. Devuelve
// cuántas se añadieron y los problemas de las que no se pudieron detectar.
func addTempSources(config *UserConfig, text string) (int, []string) {
	sources, added, problems := appendDetectedSources(config.TempSources, text)
	config.TempSources = sources
	return added, problems
}

// addProductSources añade fuentes al producto en edición, evitando duplicados.
func addProductSources(config *UserConfig, text string) (int, []string) {
	p := &config.Products[config.EditIndex]
	sources, added, problems := appendDetectedSources(p.Sources, text)
	p.Sources = sources
	return added, problems
}

// appendDetectedSources devuelve la lista con las URLs válidas y no
// duplicadas del texto añadidas al final, cuántas se añadieron y los
// problemas de las que no se pudieron detectar.
func appendDetectedSources(dst []Source, text string) ([]Source, int, []string) {
	seen := make(map[string]bool, len(dst))
	for _, s := range dst {
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
		dst = append(dst, Source{URL: raw, Store: store.Key})
		added++
	}
	return dst, added, problems
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
		"3/3 · ¿Cada cuánto quieres comprobarlo? (minutos o segundos, sin mínimo; ej: 1s, 5s, 30s, 2m)\n\nTiendas: %s",
		sourcesStoreLabel(config.TempSources)))
}

// --- Listados y menús ---

func (b *Bot) showMainMenu(config *UserConfig) {
	config.BotState = StateIdle
	config.EditIndex = -1
	config.SourceSelection = nil

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

// showSourceManager lista las URLs del producto en edición y ofrece botones
// para marcarlas, eliminarlas o añadir nuevas.
func (b *Bot) showSourceManager(config *UserConfig) {
	if !config.inRange() {
		b.showMainMenu(config)
		return
	}
	if config.SourceSelection == nil {
		config.SourceSelection = make(map[int]bool)
	}
	p := config.Products[config.EditIndex]

	var rows [][]tgbotapi.InlineKeyboardButton
	for i, s := range p.Sources {
		mark := "⬜"
		if config.SourceSelection[i] {
			mark = "✅"
		}
		label := fmt.Sprintf("%s %d. %s", mark, i+1, StoreLabel(s.Store))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("src_toggle_%d", i)),
		))
	}
	rows = append(rows,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Añadir URLs", "src_add"),
			tgbotapi.NewInlineKeyboardButtonData("🗑 Eliminar", "src_del"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ Volver", "ef_back"),
		),
	)
	sendKeyboard(b.api, config.ChatID,
		formatSourceManager(p, config.SourceSelection), tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// handleSourceCallback procesa las acciones del gestor de URLs.
func (b *Bot) handleSourceCallback(config *UserConfig, cmd string) {
	switch {
	case cmd == "src_add":
		if !config.inRange() {
			b.showMainMenu(config)
			return
		}
		config.BotState = StateEditURLAdd
		sendMessage(b.api, config.ChatID,
			"➕ Envía la URL o URLs que quieras añadir (separadas por espacios).\n\n"+
				"Tiendas soportadas: "+SupportedStoresList())
	case cmd == "src_del":
		b.deleteSelectedSources(config)
	case cmd == "ef_back":
		config.SourceSelection = nil
		config.BotState = StateIdle
		b.showEditMenu(config)
	case strings.HasPrefix(cmd, "src_toggle_"):
		if !config.inRange() {
			return
		}
		idx, err := parseIndex(strings.TrimPrefix(cmd, "src_toggle_"))
		if err != nil || idx < 0 || idx >= len(config.Products[config.EditIndex].Sources) {
			return
		}
		if config.SourceSelection == nil {
			config.SourceSelection = make(map[int]bool)
		}
		config.SourceSelection[idx] = !config.SourceSelection[idx]
		b.showSourceManager(config)
	}
}

// deleteSelectedSources elimina las URLs marcadas, evitando dejar el producto
// sin ninguna fuente.
func (b *Bot) deleteSelectedSources(config *UserConfig) {
	if !config.inRange() {
		b.showMainMenu(config)
		return
	}
	if len(config.SourceSelection) == 0 {
		sendMessage(b.api, config.ChatID, "⚠️ No has seleccionado ninguna URL.")
		return
	}
	p := &config.Products[config.EditIndex]
	kept := make([]Source, 0, len(p.Sources))
	var removed int
	for i, s := range p.Sources {
		if config.SourceSelection[i] {
			removed++
			continue
		}
		kept = append(kept, s)
	}
	if len(kept) == 0 {
		sendMessage(b.api, config.ChatID, "⚠️ Un producto debe tener al menos una URL. No se eliminó ninguna.")
		return
	}
	p.Sources = kept
	p.NextRun = time.Now()
	config.SourceSelection = nil
	saveProducts(config)
	sendMessage(b.api, config.ChatID, fmt.Sprintf("🗑 %d URL(s) eliminada(s).", removed))
	b.showSourceManager(config)
}

func (b *Bot) handleEditSelect(config *UserConfig, raw string) {
	idx, err := parseIndex(raw)
	if err != nil || idx < 0 || idx >= len(config.Products) {
		return
	}
	config.EditIndex = idx
	config.BotState = StateIdle
	config.SourceSelection = nil
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
