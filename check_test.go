package main

import "testing"

func TestNormalize(t *testing.T) {
	got := normalize("  Añadir a la CESTA  ")
	want := "anadir a la cesta"
	if got != want {
		t.Fatalf("normalize() = %q, want %q", got, want)
	}
}

func TestCheckByButtonTextGameOutOfStock(t *testing.T) {
	body := []byte(`<html><body>
		<div class="buy-l buy-new">
			<div class="buy--type"><span class="cm-txt cm-label">PRÓXIMAMENTE</span></div>
		</div>
	</body></html>`)
	r, err := checkByButtonText(body, gameInTexts, gameOutTexts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusOutOfStock {
		t.Fatalf("status = %v, want OutOfStock", r.Status)
	}
}

func TestCheckByButtonTextGameInStock(t *testing.T) {
	body := []byte(`<html><body>
		<button type="submit"><span class="">Añadir a la cesta</span></button>
	</body></html>`)
	r, err := checkByButtonText(body, gameInTexts, gameOutTexts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusInStock {
		t.Fatalf("status = %v, want InStock", r.Status)
	}
}

func TestCheckByButtonTextIgnoresScriptContent(t *testing.T) {
	// El texto de compra vive dentro de un <script> (diccionario i18n) y no
	// debe contar como botón visible.
	body := []byte(`<html><body>
		<script>var i18n = {"basket":{"addToBasket":"Añadir al carrito"}};</script>
		<p>Este artículo no está disponible actualmente.</p>
	</body></html>`)
	r, err := checkByButtonText(body, mediaMarktInTexts, mediaMarktOutTexts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusOutOfStock {
		t.Fatalf("status = %v, want OutOfStock", r.Status)
	}
}

func TestCheckByButtonTextPrefersActionOverOutText(t *testing.T) {
	body := []byte(`<html><body>
		<button>Añadir a la cesta</button>
		<p>No disponible en tu zona</p>
	</body></html>`)
	r, err := checkByButtonText(body, gameInTexts, gameOutTexts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusInStock {
		t.Fatalf("status = %v, want InStock", r.Status)
	}
}

func TestCheckJSONLDAvailability(t *testing.T) {
	body := []byte(`<html><head>
		<script type="application/ld+json">
		{"@type":"BuyAction","object":{"@type":"Product","name":"Consola",
		"offers":{"@type":"Offer","availability":"https://schema.org/OutOfStock",
		"url":"https://www.mediamarkt.es/es/product/_x-1674231.html"}}}
		</script>
	</head><body></body></html>`)
	r, ok := checkJSONLDAvailability(body, "https://www.mediamarkt.es/es/product/_x-1674231.html")
	if !ok {
		t.Fatal("no se encontró disponibilidad en el JSON-LD")
	}
	if r.Status != StatusOutOfStock {
		t.Fatalf("status = %v, want OutOfStock", r.Status)
	}
}

func TestCheckJSONLDAvailabilityInStock(t *testing.T) {
	body := []byte(`<script type="application/ld+json">
		{"@type":"Product","offers":{"@type":"Offer","availability":"https://schema.org/InStock"}}
	</script>`)
	r, ok := checkJSONLDAvailability(body, "")
	if !ok || r.Status != StatusInStock {
		t.Fatalf("got ok=%v status=%v, want InStock", ok, r.Status)
	}
}

func TestAvailabilityFromSchema(t *testing.T) {
	cases := map[string]Status{
		"https://schema.org/InStock":      StatusInStock,
		"https://schema.org/PreOrder":     StatusInStock,
		"https://schema.org/OutOfStock":   StatusOutOfStock,
		"https://schema.org/SoldOut":      StatusOutOfStock,
		"https://schema.org/Discontinued": StatusOutOfStock,
	}
	for in, want := range cases {
		got, ok := availabilityFromSchema(in)
		if !ok || got != want {
			t.Errorf("availabilityFromSchema(%q) = %v/%v, want %v", in, got, ok, want)
		}
	}
}

func TestXtralifeStatus(t *testing.T) {
	cases := map[string]Status{
		"sell":                     StatusInStock,
		"reservation":              StatusInStock,
		"reservation_not_opened":   StatusOutOfStock,
		"out_of_stock":             StatusOutOfStock,
		"maybe_out_of_stock":       StatusOutOfStock,
		"reservation_out_of_stock": StatusOutOfStock,
		"restock":                  StatusOutOfStock,
		"archived":                 StatusOutOfStock,
	}
	for in, want := range cases {
		got, ok := xtralifeStatus(in)
		if !ok || got != want {
			t.Errorf("xtralifeStatus(%q) = %v/%v, want %v", in, got, ok, want)
		}
	}
	if _, ok := xtralifeStatus("vaya_estado"); ok {
		t.Error("un estado desconocido no debería mapearse")
	}
}

func TestXtralifeSKU(t *testing.T) {
	got := xtralifeSKU("https://www.xtralife.com/producto/nintendo-switch-2/113185")
	if got != "113185" {
		t.Fatalf("xtralifeSKU = %q, want 113185", got)
	}
	if got := xtralifeSKU("https://www.xtralife.com/producto/sin-id/"); got != "" {
		t.Fatalf("xtralifeSKU sin id = %q, want vacío", got)
	}
}

func TestAmazonBuyButtonText(t *testing.T) {
	shortcut := []byte(`<html><body>
		<button id="nav-assist-add-to-cart" class="nav-assistant-link-button">
			<span class="shortcut-name">Añadir a la cesta</span>
		</button>
	</body></html>`)
	if _, ok := amazonBuyButtonText(shortcut); ok {
		t.Fatal("el widget de atajos no debe contar como botón de compra")
	}

	real := []byte(`<html><body>
		<input type="submit" name="submit.add-to-cart" value="Añadir a la cesta">
	</body></html>`)
	if _, ok := amazonBuyButtonText(real); !ok {
		t.Fatal("se esperaba detectar el input de compra")
	}

	disabled := []byte(`<html><body>
		<input type="submit" value="Añadir a la cesta" disabled>
	</body></html>`)
	if _, ok := amazonBuyButtonText(disabled); ok {
		t.Fatal("un botón deshabilitado no debe contar")
	}

	announce := []byte(`<html><body>
		<span id="submit.buy-now-announce" class="a-button-text">Comprar en Preventa ya</span>
	</body></html>`)
	if _, ok := amazonBuyButtonText(announce); !ok {
		t.Fatal("se esperaba detectar el span de anuncio de compra")
	}
}
