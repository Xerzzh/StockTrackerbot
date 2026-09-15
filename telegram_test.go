package main

import "testing"

func TestParseSourcesMultiple(t *testing.T) {
	sources, err := parseSources(
		"https://www.game.es/x https://www.amazon.es/dp/B0 https://www.xtralife.com/producto/y/123")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 3 {
		t.Fatalf("se esperaban 3 fuentes, hay %d", len(sources))
	}
	want := []string{"game", "amazon", "xtralife"}
	for i, s := range sources {
		if s.Store != want[i] {
			t.Errorf("fuente %d: store = %q, want %q", i, s.Store, want[i])
		}
	}
}

func TestParseSourcesDedupes(t *testing.T) {
	sources, err := parseSources("https://www.game.es/x\nhttps://www.game.es/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("se esperaba 1 fuente deduplicada, hay %d", len(sources))
	}
}

func TestParseSourcesRejectsUnsupported(t *testing.T) {
	if _, err := parseSources("https://www.pccomponentes.com/x"); err == nil {
		t.Fatal("se esperaba error para tienda no soportada")
	}
}

func TestAppendDetectedSources(t *testing.T) {
	existing := []Source{{URL: "https://www.game.es/x", Store: "game"}}
	sources, added, problems := appendDetectedSources(existing,
		"https://www.game.es/x https://www.amazon.es/dp/B0 https://www.pccomponentes.com/y")
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	if len(problems) != 1 {
		t.Fatalf("problems = %d, want 1", len(problems))
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(sources))
	}
	if sources[1].Store != "amazon" {
		t.Fatalf("store = %q, want amazon", sources[1].Store)
	}
}

func TestAddTempSources(t *testing.T) {
	config := &UserConfig{}
	added, problems := addTempSources(config,
		"https://www.game.es/x https://www.pccomponentes.com/y")
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	if len(problems) != 1 {
		t.Fatalf("problems = %d, want 1", len(problems))
	}
	if added, _ := addTempSources(config, "https://www.game.es/x"); added != 0 {
		t.Fatalf("una URL duplicada no debería añadirse, added = %d", added)
	}
}
