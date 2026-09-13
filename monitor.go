package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// monitorTickInterval es el tick del planificador interno. Cada producto
	// se comprueba cuando vence su propio intervalo.
	monitorTickInterval = 30 * time.Second

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

	outcomes := make([]checkOutcome, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, checkConcurrency)
	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job checkJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
			defer cancel()
			res, err := CheckProduct(ctx, b.fetcher, job.product.URL)
			outcomes[i] = checkOutcome{result: res, err: err}
		}(i, job)
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
		interval := p.IntervalMin
		if interval <= 0 {
			interval = 5
		}
		p.NextRun = now.Add(time.Duration(interval) * time.Minute)
		jobs = append(jobs, checkJob{index: i, product: *p})
	}
	return jobs
}

// applyOutcomes vuelca los resultados en el estado del usuario, persiste y
// envía las notificaciones de disponibilidad.
func (b *Bot) applyOutcomes(config *UserConfig, jobs []checkJob, outcomes []checkOutcome) []Product {
	config.Mutex.Lock()
	var notifications []Product
	for i, job := range jobs {
		if job.index < 0 || job.index >= len(config.Products) {
			continue
		}
		p := &config.Products[job.index]
		previous := p.LastStatus
		p.LastChecked = time.Now()

		if outcomes[i].err != nil {
			p.LastError = outcomes[i].err.Error()
			p.LastStatus = StatusUnknown
			p.LastDetail = ""
			log.Printf("[User %d] ❌ %s: %v", config.ChatID, p.Name, outcomes[i].err)
			continue
		}

		p.LastError = ""
		p.LastStatus = outcomes[i].result.Status
		p.LastDetail = outcomes[i].result.Detail
		log.Printf("[User %d] %s %s (%s)", config.ChatID, p.LastStatus.Emoji(), p.Name, p.LastDetail)

		if p.LastStatus == StatusInStock && previous != StatusInStock {
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

// sendStockNotification avisa al usuario de que un producto está disponible.
func (b *Bot) sendStockNotification(chatID int64, p Product) {
	text := fmt.Sprintf("🎉 ¡DISPONIBLE!\n\n📦 %s\n🏪 %s\n🔎 %s\n\n%s",
		p.Name, StoreLabel(p.Store), p.LastDetail, p.URL)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🛒 Abrir producto", p.URL),
		),
	)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("❌ Error enviando notificación a %d: %v", chatID, err)
	}
}
