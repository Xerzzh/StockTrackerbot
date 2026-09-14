package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// monitorTickInterval es el tick del planificador interno. Cada producto
	// se comprueba cuando vence su propio intervalo.
	monitorTickInterval = 5 * time.Second

	// minInterval y maxInterval acotan el intervalo configurable por producto.
	minInterval = 10 * time.Second
	maxInterval = 24 * time.Hour

	// defaultInterval se usa cuando un producto no tiene intervalo definido.
	defaultInterval = 5 * time.Minute

	// checkConcurrency limita cuántas comprobaciones simultáneas se lanzan.
	checkConcurrency = 4

	// checkTimeout es el tiempo máximo por comprobación individual.
	checkTimeout = 45 * time.Second
)

// monitor es la goroutine de monitoreo de un usuario. Se detiene al cerrar
// config.StopChan.
func (b *Bot) monitor(config *UserConfig) {
	config.Mutex.Lock()
	stop := config.StopChan
	config.Mutex.Unlock()

	log.Printf("[User %d] 🔃 Monitor iniciado", config.ChatID)
	b.checkUserProducts(config, true)

	ticker := time.NewTicker(monitorTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.checkUserProducts(config, false)
		case <-stop:
			log.Printf("[User %d] ⏹️ Monitor detenido", config.ChatID)
			return
		}
	}
}

type checkJob struct {
	index   int
	product Product
}

type checkOutcome struct {
	result CheckResult
	err    error
}

// checkUserProducts comprueba los productos vencidos (o todos si force=true),
// actualiza su estado, persiste y notifica las transiciones a "disponible".
// Devuelve una copia del estado final de los productos.
func (b *Bot) checkUserProducts(config *UserConfig, force bool) []Product {
	config.CheckMu.Lock()
	defer config.CheckMu.Unlock()

	jobs := b.pickDueProducts(config, force)
	if len(jobs) == 0 {
		return b.snapshotProducts(config)
	}

	outcomes := make([][]checkOutcome, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, checkConcurrency)
	for i, job := range jobs {
		outcomes[i] = make([]checkOutcome, len(job.product.Sources))
		for j, src := range job.product.Sources {
			wg.Add(1)
			go func(i, j int, src Source) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
				defer cancel()
				res, err := CheckProduct(ctx, b.fetcher, src.URL)
				outcomes[i][j] = checkOutcome{result: res, err: err}
			}(i, j, src)
		}
	}
	wg.Wait()

	return b.applyOutcomes(config, jobs, outcomes)
}

// pickDueProducts selecciona los productos que deben comprobarse y reprograma
// su próxima ejecución. Mantiene el lock el mínimo tiempo posible.
func (b *Bot) pickDueProducts(config *UserConfig, force bool) []checkJob {
	config.Mutex.Lock()
	defer config.Mutex.Unlock()

	now := time.Now()
	var jobs []checkJob
	for i := range config.Products {
		p := &config.Products[i]
		if !force && now.Before(p.NextRun) {
			continue
		}
		p.NextRun = now.Add(p.Interval())
		jobs = append(jobs, checkJob{index: i, product: *p})
	}
	return jobs
}

// applyOutcomes vuelca los resultados en el estado del usuario, persiste y
// envía las notificaciones de disponibilidad. Cada producto se comprueba en
// todas sus tiendas y se notifica una sola vez si alguna está disponible.
func (b *Bot) applyOutcomes(config *UserConfig, jobs []checkJob, outcomes [][]checkOutcome) []Product {
	config.Mutex.Lock()
	var notifications []Product
	now := time.Now()
	for i, job := range jobs {
		if job.index < 0 || job.index >= len(config.Products) {
			continue
		}
		p := &config.Products[job.index]
		previous := p.LastStatus
		p.LastChecked = now

		aggregate := StatusUnknown
		detail := ""
		var lastErr string
		for j := range p.Sources {
			src := &p.Sources[j]
			src.LastChecked = now

			if j >= len(outcomes[i]) {
				continue
			}
			if outcomes[i][j].err != nil {
				src.LastError = outcomes[i][j].err.Error()
				src.LastStatus = StatusUnknown
				src.LastDetail = ""
				lastErr = src.LastError
				log.Printf("[User %d] ❌ %s (%s): %v",
					config.ChatID, p.Name, StoreLabel(src.Store), outcomes[i][j].err)
				continue
			}

			src.LastError = ""
			src.LastStatus = outcomes[i][j].result.Status
			src.LastDetail = outcomes[i][j].result.Detail
			log.Printf("[User %d] %s %s (%s) %s",
				config.ChatID, src.LastStatus.Emoji(), p.Name, StoreLabel(src.Store), src.LastDetail)

			switch {
			case src.LastStatus == StatusInStock && aggregate != StatusInStock:
				aggregate = StatusInStock
				detail = src.LastDetail
			case src.LastStatus == StatusOutOfStock && aggregate == StatusUnknown:
				aggregate = StatusOutOfStock
			}
		}

		p.LastError = lastErr
		p.LastStatus = aggregate
		p.LastDetail = detail

		if aggregate == StatusInStock && previous != StatusInStock {
			notifications = append(notifications, *p)
		}
	}
	snapshot := make([]Product, len(config.Products))
	copy(snapshot, config.Products)
	config.Mutex.Unlock()

	saveProductsSnapshot(config.ChatID, snapshot)

	for _, p := range notifications {
		b.sendStockNotification(config.ChatID, p)
	}
	return snapshot
}

func (b *Bot) snapshotProducts(config *UserConfig) []Product {
	config.Mutex.Lock()
	defer config.Mutex.Unlock()
	snapshot := make([]Product, len(config.Products))
	copy(snapshot, config.Products)
	return snapshot
}

// sendStockNotification avisa al usuario de que un producto está disponible en
// una o varias de sus tiendas. Se envía una sola notificación por producto.
func (b *Bot) sendStockNotification(chatID int64, p Product) {
	available := p.SourcesInStock()
	if len(available) == 0 {
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "🎉 ¡DISPONIBLE!\n\n📦 %s\n", p.Name)
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, s := range available {
		label := StoreLabel(s.Store)
		fmt.Fprintf(&sb, "\n🏪 %s\n🔎 %s\n%s\n", label, s.LastDetail, s.URL)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🛒 Abrir en "+label, s.URL),
		))
	}

	msg := tgbotapi.NewMessage(chatID, sb.String())
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("❌ Error enviando notificación a %d: %v", chatID, err)
	}
}
