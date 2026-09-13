package main

import "testing"

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
	got, err := parseInterval(" 5 ")
	if err != nil || got != 5 {
		t.Fatalf("parseInterval(5) = %d, %v", got, err)
	}
}
