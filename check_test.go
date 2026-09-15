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

func TestFnacBuyButtonText(t *testing.T) {
	inStock := []byte(`<html><body>
		<button type="button">
			<span class="ff-button-label" data-automation-id="product-buy-btn-label">
				Añadir a la cesta
			</span>
		</button>
	</body></html>`)
	if text, ok := fnacBuyButtonText(inStock); !ok || text != "Añadir a la cesta" {
		t.Fatalf("fnacBuyButtonText in-stock = %q/%v, want InStock", text, ok)
	}

	outOfStock := []byte(`<html><body>
		<div style="color: #a9a9a9;font-weight: bold;">No disponible en Fnac.es</div>
	</body></html>`)
	if _, ok := fnacBuyButtonText(outOfStock); ok {
		t.Fatal("sin botón de compra no debe detectarse disponibilidad")
	}

	disabled := []byte(`<html><body>
		<button type="button" disabled>
			<span class="ff-button-label" data-automation-id="product-buy-btn-label">
				Añadir a la cesta
			</span>
		</button>
	</body></html>`)
	if _, ok := fnacBuyButtonText(disabled); ok {
		t.Fatal("un botón deshabilitado no debe contar como disponible")
	}
}

func TestElCorteInglesAvailability(t *testing.T) {
	inStock := []byte(`<html><body>
		<div class="product_detail-aside--buy">
			<button class="pds-button pds-button--is-filled" aria-busy="false"
				id="add_to_cart_main_button" data-testid="pdp-add-to-cart">
				<span class="pds-button__label">Añadir a la cesta</span>
			</button>
		</div>
	</body></html>`)
	r, ok := elCorteInglesAvailability(inStock)
	if !ok || r.Status != StatusInStock {
		t.Fatalf("elCorteInglesAvailability in-stock = %v/%v, want InStock", r.Status, ok)
	}

	outOfStock := []byte(`<html><body>
		<div class="product_detail-aside--buy">
			<button class="pds-button pds-button--is-disabled" aria-disabled="true"
				aria-busy="false" id="add_to_cart_main_button" data-testid="pdp-add-to-cart">
				<span class="pds-button__label">AGOTADO</span>
			</button>
		</div>
	</body></html>`)
	r, ok = elCorteInglesAvailability(outOfStock)
	if !ok || r.Status != StatusOutOfStock {
		t.Fatalf("elCorteInglesAvailability out-of-stock = %v/%v, want OutOfStock", r.Status, ok)
	}

	if _, ok := elCorteInglesAvailability([]byte(`<html><body><p>sin botón</p></body></html>`)); ok {
		t.Fatal("sin botón de compra no debe detectarse disponibilidad")
	}
}

func TestCheckJSONLDAvailabilityCarrefour(t *testing.T) {
	body := []byte(`<html><head>
		<script type="application/ld+json">
		{"@context":"http://schema.org","@type":"Product","name":"Consola",
		"offers":{"price":"519.00","@type":"Offer",
		"url":"https://carrefour.es/consola/VC4A-34646617/p",
		"priceCurrency":"EUR","availability":"https://schema.org/InStock"}}
		</script>
	</head><body></body></html>`)
	r, ok := checkJSONLDAvailability(body, "https://www.carrefour.es/consola/VC4A-34646617/p")
	if !ok || r.Status != StatusInStock {
		t.Fatalf("checkJSONLDAvailability carrefour = %v/%v, want InStock", r.Status, ok)
	}
}

func TestLooksLikeCaptcha(t *testing.T) {
	dataDome := []byte(`<html><body><p id="cmsg">Please enable JS and disable any ad blocker</p>` +
		`<script>var dd={'rt':'i','host':'geo.captcha-delivery.com'}</script></body></html>`)
	if !looksLikeCaptcha(dataDome, "https://www.fnac.es/") {
		t.Fatal("se esperaba detectar el captcha de DataDome")
	}

	cloudflare := []byte(`<html><head><title>Just a moment...</title>` +
		`<meta http-equiv="content-security-policy" content="script-src https://challenges.cloudflare.com">` +
		`</head><body><script>window._cf_chl_opt={cZone:'www.carrefour.es'}</script></body></html>`)
	if !looksLikeCaptcha(cloudflare, "https://www.carrefour.es/") {
		t.Fatal("se esperaba detectar el challenge de Cloudflare")
	}

	normal := []byte(`<html><body><span data-automation-id="product-buy-btn-label">Añadir a la cesta</span></body></html>`)
	if looksLikeCaptcha(normal, "https://www.fnac.es/") {
		t.Fatal("una página de producto normal no debe detectarse como captcha")
	}
}
