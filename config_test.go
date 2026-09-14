package main

import (
	"testing"
	"time"
)

func TestLoadConfigMissingToken(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("se esperaba error al faltar TELEGRAM_BOT_TOKEN")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "token-de-prueba")
	t.Setenv("ADMIN_CHAT_ID", "12345")
	t.Setenv("HTTP_TIMEOUT", "")
	t.Setenv("STATE_DIR", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "token-de-prueba" {
		t.Errorf("Token = %q", cfg.Token)
	}
	if cfg.AdminChatID != 12345 {
		t.Errorf("AdminChatID = %d", cfg.AdminChatID)
	}
	if cfg.StateDir != "config" {
		t.Errorf("StateDir = %q", cfg.StateDir)
	}
}

func TestLoadConfigInvalidAdmin(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")
	t.Setenv("ADMIN_CHAT_ID", "no-numero")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("se esperaba error con ADMIN_CHAT_ID no numérico")
	}
}

func TestParseInterval(t *testing.T) {
	if _, err := parseInterval("0"); err == nil {
		t.Fatal("0 no es un intervalo válido")
	}
	if _, err := parseInterval("abc"); err == nil {
		t.Fatal("abc no es un intervalo válido")
	}
	if _, err := parseInterval("99999"); err == nil {
		t.Fatal("99999 supera el máximo")
	}
	if _, err := parseInterval("5s"); err == nil {
		t.Fatal("5s está por debajo del mínimo")
	}
	got, err := parseInterval(" 5 ")
	if err != nil || got != 5*time.Minute {
		t.Fatalf("parseInterval(5) = %v, %v", got, err)
	}
	if got, err := parseInterval("30s"); err != nil || got != 30*time.Second {
		t.Fatalf("parseInterval(30s) = %v, %v", got, err)
	}
	if got, err := parseInterval("2m"); err != nil || got != 2*time.Minute {
		t.Fatalf("parseInterval(2m) = %v, %v", got, err)
	}
	if got, err := parseInterval("90seg"); err != nil || got != 90*time.Second {
		t.Fatalf("parseInterval(90seg) = %v, %v", got, err)
	}
}
