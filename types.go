package main

import (
	"sync"
	"time"
)

// Status representa el resultado de comprobar la disponibilidad de un producto.
type Status int

const (
	// StatusUnknown indica que no se ha podido determinar con certeza.
	StatusUnknown Status = iota
	// StatusOutOfStock indica que el producto no se puede comprar ahora mismo.
	StatusOutOfStock
	// StatusInStock indica que hay un botón de compra/añadir a la cesta activo.
	StatusInStock
)

// String devuelve una etiqueta legible para el estado.
func (s Status) String() string {
	switch s {
	case StatusInStock:
		return "Disponible"
	case StatusOutOfStock:
		return "Agotado"
	default:
		return "Desconocido"
	}
}

// Emoji devuelve un emoji representativo del estado.
func (s Status) Emoji() string {
	switch s {
	case StatusInStock:
		return "🟢"
	case StatusOutOfStock:
		return "🔴"
	default:
		return "⚪"
	}
}

// CheckResult es el resultado de comprobar un producto.
type CheckResult struct {
	Status Status
	// Detail es la evidencia encontrada (texto del botón, disponibilidad, etc.).
	Detail string
}

// Source es una URL concreta de una tienda para un producto. Un mismo
// producto puede vigilarse en varias tiendas a la vez y se notifica una sola
// vez cuando cualquiera de ellas lo tiene disponible.
type Source struct {
	URL         string    `json:"url"`
	Store       string    `json:"store"`
	LastStatus  Status    `json:"last_status"`
	LastChecked time.Time `json:"last_checked"`
	LastDetail  string    `json:"last_detail,omitempty"`
	LastError   string    `json:"last_error,omitempty"`

	// Estado de backoff en runtime (no se persiste). failures cuenta los
	// fallos reintentables consecutivos y nextAttempt es el momento antes del
	// cual no se debe volver a comprobar esta fuente.
	failures    int
	nextAttempt time.Time
}

// inBackoff indica si la fuente está en espera por backoff en el momento dado.
func (s Source) inBackoff(now time.Time) bool {
	return now.Before(s.nextAttempt)
}

// Product es un producto vigilado por un usuario. Puede tener una o varias
// fuentes (URLs en distintas tiendas).
type Product struct {
	Name        string    `json:"name"`
	Sources     []Source  `json:"sources"`
	IntervalSec int       `json:"interval_sec"`
	LastStatus  Status    `json:"last_status"`
	LastChecked time.Time `json:"last_checked"`
	LastDetail  string    `json:"last_detail,omitempty"`
	LastError   string    `json:"last_error,omitempty"`

	// Campos heredados de versiones con una sola URL. Solo se usan para
	// migrar datos antiguos al cargar.
	URL   string `json:"url,omitempty"`
	Store string `json:"store,omitempty"`

	// IntervalMin es el intervalo heredado (en minutos) de versiones
	// anteriores. Solo se usa para migrar datos antiguos al cargar.
	IntervalMin int `json:"interval_min,omitempty"`

	// NextRun se calcula en runtime y no se persiste.
	NextRun time.Time `json:"-"`
}

// Interval devuelve el intervalo efectivo de comprobación del producto.
func (p Product) Interval() time.Duration {
	if p.IntervalSec > 0 {
		return time.Duration(p.IntervalSec) * time.Second
	}
	return defaultInterval
}

// StoreKeys devuelve las claves de tienda del producto sin repetir.
func (p Product) StoreKeys() []string {
	seen := make(map[string]bool, len(p.Sources))
	var keys []string
	for _, s := range p.Sources {
		if s.Store == "" || seen[s.Store] {
			continue
		}
		seen[s.Store] = true
		keys = append(keys, s.Store)
	}
	return keys
}

// SourcesInStock devuelve las fuentes disponibles ahora mismo.
func (p Product) SourcesInStock() []Source {
	var in []Source
	for _, s := range p.Sources {
		if s.LastStatus == StatusInStock {
			in = append(in, s)
		}
	}
	return in
}

// UserConfig agrupa todo el estado runtime de un usuario del bot.
type UserConfig struct {
	ChatID      int64
	DisplayName string
	Products    []Product
	IsRunning   bool
	StopChan    chan struct{}
	stopOnce    sync.Once
	BotState    BotState

	// Campos temporales usados durante el asistente de alta.
	TempName        string
	TempSources     []Source
	TempIntervalSec int

	EditIndex int
	// SourceSelection marca las fuentes seleccionadas en el gestor de URLs
	// del producto que se está editando.
	SourceSelection map[int]bool

	// Mutex protege el estado del usuario.
	Mutex sync.Mutex
	// CheckMu serializa las comprobaciones de red para un mismo usuario.
	CheckMu sync.Mutex
}

// UserStorage es el formato de serialización en disco.
type UserStorage struct {
	Products []Product `json:"products"`
}

// BotState representa el estado de la máquina de estados por usuario.
type BotState int

const (
	StateIdle BotState = iota

	// Asistente de alta de producto.
	StateAddName
	StateAddURL
	StateAddInterval

	// Edición de campos de un producto existente.
	StateEditName
	StateEditURLAdd
	StateEditInterval

	// Flujo admin.
	StateAdminAddUser
	StateAdminDelUser
)

// stopMonitoring cierra el canal de parada una sola vez para evitar panics.
func (c *UserConfig) stopMonitoring() {
	c.stopOnce.Do(func() {
		if c.StopChan != nil {
			close(c.StopChan)
		}
	})
}

// resetStop prepara un nuevo canal de parada (y reinicia el sync.Once).
func (c *UserConfig) resetStop() {
	c.StopChan = make(chan struct{})
	c.stopOnce = sync.Once{}
}

// inRange verifica que EditIndex apunte a un producto válido.
func (c *UserConfig) inRange() bool {
	return c.EditIndex >= 0 && c.EditIndex < len(c.Products)
}
