package main

import "testing"

func TestSaveAndLoadProducts(t *testing.T) {
	oldDir := configDir
	configDir = t.TempDir()
	defer func() { configDir = oldDir }()

	original := &UserConfig{
		ChatID: 42,
		Products: []Product{
			{Name: "Switch 2", URL: "https://www.game.es/x", Store: "game", IntervalMin: 5, LastStatus: StatusOutOfStock},
			{Name: "Zelda", URL: "https://www.amazon.es/dp/B0", Store: "amazon", IntervalMin: 10},
		},
	}
	saveProducts(original)

	loaded := &UserConfig{ChatID: 42}
	loadProducts(loaded)

	if len(loaded.Products) != 2 {
		t.Fatalf("se cargaron %d productos, want 2", len(loaded.Products))
	}
	if loaded.Products[0].Name != "Switch 2" || loaded.Products[0].Store != "game" {
		t.Errorf("producto 0 inesperado: %+v", loaded.Products[0])
	}
	if loaded.Products[0].LastStatus != StatusOutOfStock {
		t.Errorf("LastStatus no persistido: %v", loaded.Products[0].LastStatus)
	}
	if loaded.Products[1].IntervalMin != 10 {
		t.Errorf("IntervalMin no persistido: %d", loaded.Products[1].IntervalMin)
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
