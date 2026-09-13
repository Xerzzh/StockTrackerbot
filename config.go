package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config agrupa la configuración del bot, cargada desde variables de entorno
// o desde un fichero .env situado junto al binario.
type Config struct {
	Token       string
	AdminChatID int64
	HTTPTimeout time.Duration
	StateDir    string
}

// LoadConfig lee la configuración. El fichero .env (si existe) se carga antes
// de consultar las variables de entorno, sin sobrescribir las ya definidas.
func LoadConfig() (*Config, error) {
	loadDotEnv(".env")

	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("falta la variable TELEGRAM_BOT_TOKEN")
	}

	var admin int64
	if v := strings.TrimSpace(os.Getenv("ADMIN_CHAT_ID")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("ADMIN_CHAT_ID no es un número válido: %q", v)
		}
		admin = id
	}

	timeout, err := envDuration("HTTP_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}
	if timeout < 5*time.Second {
		return nil, fmt.Errorf("HTTP_TIMEOUT debe ser de al menos 5s")
	}

	return &Config{
		Token:       token,
		AdminChatID: admin,
		HTTPTimeout: timeout,
		StateDir:    envString("STATE_DIR", "config"),
	}, nil
}

func envString(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s no es una duración válida (ej: 30s, 2m): %q", key, v)
	}
	return d, nil
}

// loadDotEnv carga un fichero .env sencillo (KEY=VALUE) sin dependencias.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
