package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// monitorTickInterval es el tick del planificador interno. Cada producto
	// se comprueba cuando vence su propio intervalo. Se usa 1s para permitir
	// intervalos de un segundo sin límite inferior.
	monitorTickInterval = 1 * time.Second

	// defaultInterval se usa cuando un producto no tiene intervalo definido.
	defaultInterval = 5 * time.Minute

	// checkConcurrency limita cuántas comprobaciones simultáneas se lanzan.
	checkConcurrency = 4

	// checkTimeout es el tiempo máximo por comprobación individual.
	checkTimeout = 45 * time.Second

	// Parámetros de backoff ante respuestas 429/5xx y errores de red.
	baseBackoff = 15 * time.Second
	maxBackoff  = 15 * time.Minute

	// jitterRatio es la variación aleatoria aplicada a intervalos y backoff
	// (±20% en la programación, +0-20% en el backoff) para evitar patrones
	// fijos fácilmente detectables.
	jitterRatio = 0.2
)

// withJitter devuelve d con una variación aleatoria de ±jitterRatio.
func withJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	spread := int64(float64(d) * jitterRatio)
	if spread <= 0 {
		return d
	}
	offset := rand.Int63n(2*spread+1) - spread
	return d + time.Duration(offset)
}

// addJitter añade entre 0 y +jitterRatio a d. Se usa en el backoff para no
// reintentar antes de lo indicado por el servidor.
func addJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	spread := int64(float64(d) * jitterRatio)
	if spread <= 0 {
		return d
	}
	return d + time.Duration(rand.Int63n(spread+1))
}

// backoffDuration calcula el retraso exponencial según los fallos consecutivos,
// acotado por maxBackoff.
func backoffDuration(failures int) time.Duration {
	if failures < 1 {
		return 0
	}
	d := baseBackoff
	for i := 1; i < failures && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

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
	// skip marca las fuentes que están en backoff y no deben comprobarse.
	skip []bool
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
			if j < len(job.skip) && job.skip[j] {
				continue
			}
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

		skip := make([]bool, len(p.Sources))
		active := 0
		for j := range p.Sources {
			if !force && p.Sources[j].inBackoff(now) {
				skip[j] = true
				continue
			}
			active++
		}
		p.NextRun = now.Add(withJitter(p.Interval()))
		if active == 0 {
			continue
		}
		jobs = append(jobs, checkJob{index: i, product: *p, skip: skip})
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
			if j < len(job.skip) && job.skip[j] {
				continue
			}
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
				if d, ok := retryDelay(outcomes[i][j].err); ok {
					src.failures++
					if d <= 0 {
						d = backoffDuration(src.failures)
					}
					src.nextAttempt = now.Add(addJitter(d))
					log.Printf("[User %d] ⏳ %s (%s): pausa %s (fallo %d)",
						config.ChatID, p.Name, StoreLabel(src.Store), intervalLabel(d), src.failures)
				}
				continue
			}

			src.failures = 0
			src.nextAttempt = time.Time{}
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
