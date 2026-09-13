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

// Product es un producto vigilado por un usuario.
type Product struct {
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Store       string    `json:"store"`
	IntervalMin int       `json:"interval_min"`
	LastStatus  Status    `json:"last_status"`
	LastChecked time.Time `json:"last_checked"`
	LastDetail  string    `json:"last_detail,omitempty"`
	LastError   string    `json:"last_error,omitempty"`

	// NextRun se calcula en runtime y no se persiste.
	NextRun time.Time `json:"-"`
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
	TempName     string
	TempURL      string
	TempStore    string
	TempInterval int

	EditIndex int

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
	StateEditURL
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
