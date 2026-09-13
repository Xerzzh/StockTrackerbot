package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

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

// intervalLabel formatea un intervalo en minutos.
func intervalLabel(min int) string {
	if min <= 0 {
		return "sin intervalo"
	}
	if min == 1 {
		return "1 min"
	}
	return fmt.Sprintf("%d min", min)
}

// productStatusLine formatea una línea con el estado de un producto.
func productStatusLine(p Product) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", p.LastStatus.Emoji(), p.Name)
	fmt.Fprintf(&b, "   🏪 %s · cada %s\n", StoreLabel(p.Store), intervalLabel(p.IntervalMin))
	if p.LastError != "" {
		fmt.Fprintf(&b, "   ⚠️ Error: %s\n", p.LastError)
	} else {
		if !p.LastChecked.IsZero() {
			fmt.Fprintf(&b, "   🕐 %s", p.LastChecked.Format("02/01 15:04"))
		} else {
			b.WriteString("   🕐 sin comprobar")
		}
		if p.LastDetail != "" {
			fmt.Fprintf(&b, " · %s", p.LastDetail)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "   %s", p.URL)
	return b.String()
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
	fmt.Fprintf(&b, "Tienda: %s\n", StoreLabel(p.Store))
	fmt.Fprintf(&b, "Intervalo: %s\n", intervalLabel(p.IntervalMin))
	fmt.Fprintf(&b, "Estado: %s %s\n", p.LastStatus.Emoji(), p.LastStatus)
	fmt.Fprintf(&b, "URL: %s\n", p.URL)
	return b.String()
}

// parseInterval valida y convierte el texto del intervalo.
func parseInterval(text string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0, fmt.Errorf("introduce un número de minutos válido")
	}
	if n <= 0 {
		return 0, fmt.Errorf("el intervalo debe ser mayor que 0")
	}
	if n > 1440 {
		return 0, fmt.Errorf("el intervalo máximo es 1440 minutos (24 h)")
	}
	return n, nil
}
