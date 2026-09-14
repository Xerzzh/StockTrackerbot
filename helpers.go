package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// sendMessage envía un mensaje de texto simple (sin Markdown para evitar
// problemas de escapado con nombres y URLs introducidos por el usuario).
func sendMessage(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	if _, err := bot.Send(msg); err != nil {
		log.Printf("❌ Error enviando mensaje: %v", err)
	}
}

// sendKeyboard envía un mensaje con un teclado inline.
func sendKeyboard(bot *tgbotapi.BotAPI, chatID int64, text string, markup tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = markup
	if _, err := bot.Send(msg); err != nil {
		log.Printf("❌ Error enviando teclado: %v", err)
	}
}

// intervalLabel formatea un intervalo en minutos o segundos.
func intervalLabel(d time.Duration) string {
	if d <= 0 {
		return "sin intervalo"
	}
	if d%time.Minute == 0 {
		min := int(d / time.Minute)
		if min == 1 {
			return "1 min"
		}
		return fmt.Sprintf("%d min", min)
	}
	if d%time.Second == 0 {
		return fmt.Sprintf("%d s", int(d/time.Second))
	}
	return d.String()
}

// sourcesStoreLabel devuelve las tiendas de una lista de fuentes sin repetir.
func sourcesStoreLabel(sources []Source) string {
	seen := make(map[string]bool, len(sources))
	var labels []string
	for _, s := range sources {
		if s.Store == "" || seen[s.Store] {
			continue
		}
		seen[s.Store] = true
		labels = append(labels, StoreLabel(s.Store))
	}
	return strings.Join(labels, ", ")
}

// productStatusLine formatea una línea con el estado de un producto.
func productStatusLine(p Product) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", p.LastStatus.Emoji(), p.Name)
	fmt.Fprintf(&b, "   🏪 %s · cada %s\n", sourcesStoreLabel(p.Sources), intervalLabel(p.Interval()))
	for _, s := range p.Sources {
		fmt.Fprintf(&b, "   %s %s", s.LastStatus.Emoji(), StoreLabel(s.Store))
		if s.LastError != "" {
			fmt.Fprintf(&b, " · ⚠️ %s", s.LastError)
		} else if s.LastDetail != "" {
			fmt.Fprintf(&b, " · %s", s.LastDetail)
		}
		if !s.LastChecked.IsZero() {
			fmt.Fprintf(&b, " · %s", s.LastChecked.Format("02/01 15:04"))
		}
		fmt.Fprintf(&b, "\n   %s\n", s.URL)
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatStatusReport genera el informe de estado de una lista de productos.
func formatStatusReport(products []Product) string {
	if len(products) == 0 {
		return "No hay productos configurados."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📊 Estado de tus productos (%d):\n", len(products))
	for _, p := range products {
		b.WriteString("\n")
		b.WriteString(productStatusLine(p))
		b.WriteString("\n")
	}
	return b.String()
}

// formatProductDetail genera el detalle de un producto para el menú de edición.
func formatProductDetail(p Product) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✏️ Editando: %s\n\n", p.Name)
	fmt.Fprintf(&b, "Tiendas: %s\n", sourcesStoreLabel(p.Sources))
	fmt.Fprintf(&b, "Intervalo: %s\n", intervalLabel(p.Interval()))
	fmt.Fprintf(&b, "Estado: %s %s\n", p.LastStatus.Emoji(), p.LastStatus)
	for _, s := range p.Sources {
		fmt.Fprintf(&b, "• %s: %s\n", StoreLabel(s.Store), s.URL)
	}
	return b.String()
}

// parseInterval valida y convierte el texto del intervalo. Acepta un número
// (interpretado como minutos) o una duración con sufijo de segundos o minutos:
// 30s, 30seg, 30segundos, 2m, 2min, 2minutos.
func parseInterval(text string) (time.Duration, error) {
	s := strings.ToLower(strings.TrimSpace(text))
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0, fmt.Errorf("introduce un intervalo válido (ej: 5, 2m, 30s)")
	}

	unit := time.Minute
	switch {
	case strings.HasSuffix(s, "minutos"):
		s, unit = strings.TrimSuffix(s, "minutos"), time.Minute
	case strings.HasSuffix(s, "minuto"):
		s, unit = strings.TrimSuffix(s, "minuto"), time.Minute
	case strings.HasSuffix(s, "mins"):
		s, unit = strings.TrimSuffix(s, "mins"), time.Minute
	case strings.HasSuffix(s, "min"):
		s, unit = strings.TrimSuffix(s, "min"), time.Minute
	case strings.HasSuffix(s, "m"):
		s, unit = strings.TrimSuffix(s, "m"), time.Minute
	case strings.HasSuffix(s, "segundos"):
		s, unit = strings.TrimSuffix(s, "segundos"), time.Second
	case strings.HasSuffix(s, "segundo"):
		s, unit = strings.TrimSuffix(s, "segundo"), time.Second
	case strings.HasSuffix(s, "segs"):
		s, unit = strings.TrimSuffix(s, "segs"), time.Second
	case strings.HasSuffix(s, "seg"):
		s, unit = strings.TrimSuffix(s, "seg"), time.Second
	case strings.HasSuffix(s, "s"):
		s, unit = strings.TrimSuffix(s, "s"), time.Second
	}

	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("introduce un intervalo válido (ej: 5, 2m, 30s)")
	}
	if n <= 0 {
		return 0, fmt.Errorf("el intervalo debe ser mayor que 0")
	}
	d := time.Duration(n) * unit
	if d < minInterval {
		return 0, fmt.Errorf("el intervalo mínimo es %s", intervalLabel(minInterval))
	}
	if d > maxInterval {
		return 0, fmt.Errorf("el intervalo máximo es %s", intervalLabel(maxInterval))
	}
	return d, nil
}
