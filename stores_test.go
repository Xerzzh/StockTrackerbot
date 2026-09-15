package main

import "testing"

func TestDetectStore(t *testing.T) {
	cases := map[string]string{
		"https://www.game.es/nintendo-switch-2-267689":                           "game",
		"https://www.xtralife.com/producto/zelda/113185":                         "xtralife",
		"https://www.amazon.es/dp/B0F2TN43GH":                                    "amazon",
		"https://www.amazon.de/dp/B0F2TN43GH":                                    "amazon",
		"https://www.amazon.fr/dp/B0F2TN43GH":                                    "amazon",
		"https://www.amazon.it/dp/B0F2TN43GH":                                    "amazon",
		"https://www.amazon.co.uk/dp/B0F2TN43GH":                                 "amazon",
		"https://www.mediamarkt.es/es/product/_consola-1674231.html":             "mediamarkt",
		"https://www.carrefour.es/consola-nintendo-switch-2/VC4A-34646617/p":     "carrefour",
		"https://www.elcorteingles.es/videojuegos/A202302815-nintendo-switch-2/": "elcorteingles",
	}
	for rawURL, want := range cases {
		store, err := DetectStore(rawURL)
		if err != nil {
			t.Errorf("DetectStore(%q) error: %v", rawURL, err)
			continue
		}
		if store.Key != want {
			t.Errorf("DetectStore(%q) = %q, want %q", rawURL, store.Key, want)
		}
	}
}

func TestDetectStoreUnsupported(t *testing.T) {
	if _, err := DetectStore("https://www.pccomponentes.com/producto"); err == nil {
		t.Fatal("se esperaba error para tienda no soportada")
	}
	if _, err := DetectStore("https://store.nintendo.com/es-es/nintendo-switch-2-P00211"); err == nil {
		t.Fatal("Nintendo Store ya no está soportada; se esperaba error")
	}
	if _, err := DetectStore("https://www.fnac.es/Consola-Nintendo-Switch-2/a13481099"); err == nil {
		t.Fatal("Fnac ya no está soportada (DataDome); se esperaba error")
	}
	if _, err := DetectStore("no-es-una-url"); err == nil {
		t.Fatal("se esperaba error para URL inválida")
	}
}

func TestStoreLabel(t *testing.T) {
	if got := StoreLabel("game"); got != "GAME" {
		t.Fatalf("StoreLabel(game) = %q", got)
	}
	if got := StoreLabel("desconocida"); got != "desconocida" {
		t.Fatalf("StoreLabel(desconocida) = %q", got)
	}
}
