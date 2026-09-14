package main

import (
	"os"
	"testing"
)

func TestSaveAndLoadProducts(t *testing.T) {
	oldDir := configDir
	configDir = t.TempDir()
	defer func() { configDir = oldDir }()

	original := &UserConfig{
		ChatID: 42,
		Products: []Product{
			{Name: "Switch 2", Sources: []Source{{URL: "https://www.game.es/x", Store: "game"}}, IntervalSec: 300, LastStatus: StatusOutOfStock},
			{Name: "Zelda", Sources: []Source{{URL: "https://www.amazon.es/dp/B0", Store: "amazon"}}, IntervalSec: 600},
		},
	}
	saveProducts(original)

	loaded := &UserConfig{ChatID: 42}
	loadProducts(loaded)

	if len(loaded.Products) != 2 {
		t.Fatalf("se cargaron %d productos, want 2", len(loaded.Products))
	}
	if loaded.Products[0].Name != "Switch 2" || len(loaded.Products[0].Sources) != 1 ||
		loaded.Products[0].Sources[0].Store != "game" {
		t.Errorf("producto 0 inesperado: %+v", loaded.Products[0])
	}
	if loaded.Products[0].LastStatus != StatusOutOfStock {
		t.Errorf("LastStatus no persistido: %v", loaded.Products[0].LastStatus)
	}
	if loaded.Products[1].IntervalSec != 600 {
		t.Errorf("IntervalSec no persistido: %d", loaded.Products[1].IntervalSec)
	}
}

func TestLoadProductsMigratesLegacyURL(t *testing.T) {
	oldDir := configDir
	configDir = t.TempDir()
	defer func() { configDir = oldDir }()

	legacy := []byte(`{"products":[{"name":"Switch 2","url":"https://www.game.es/x","store":"game","interval_min":5}]}`)
	if err := os.WriteFile(productsPath(7), legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := &UserConfig{ChatID: 7}
	loadProducts(loaded)

	if len(loaded.Products) != 1 {
		t.Fatalf("se cargaron %d productos, want 1", len(loaded.Products))
	}
	p := loaded.Products[0]
	if len(p.Sources) != 1 || p.Sources[0].URL != "https://www.game.es/x" || p.Sources[0].Store != "game" {
		t.Fatalf("migración de URL antigua incorrecta: %+v", p)
	}
	if p.URL != "" || p.Store != "" {
		t.Fatalf("los campos heredados deberían limpiarse: %+v", p)
	}
	if p.IntervalSec != 300 || p.IntervalMin != 0 {
		t.Fatalf("migración de interval_min incorrecta: %+v", p)
	}
}

func TestLoadProductsMissingFile(t *testing.T) {
	oldDir := configDir
	configDir = t.TempDir()
	defer func() { configDir = oldDir }()

	loaded := &UserConfig{ChatID: 999}
	loadProducts(loaded)
	if len(loaded.Products) != 0 {
		t.Fatalf("se esperaban 0 productos, hay %d", len(loaded.Products))
	}
}
