package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Store describe una tienda soportada y cómo comprobar su stock.
type Store struct {
	Key   string
	Label string
	Match func(host string) bool
	Check func(ctx context.Context, f *Fetcher, rawURL string) (CheckResult, error)
}

// Stores es el registro de tiendas soportadas. El orden importa: las
// coincidencias más específicas van primero.
var Stores = []Store{
	{
		Key:   "amazon",
		Label: "Amazon",
		Match: func(host string) bool { return strings.Contains(host, "amazon.") },
		Check: checkAmazon,
	},
	{
		Key:   "game",
		Label: "GAME",
		Match: func(host string) bool { return host == "game.es" || strings.HasSuffix(host, ".game.es") },
		Check: checkGame,
	},
	{
		Key:   "xtralife",
		Label: "Xtralife",
		Match: func(host string) bool { return strings.Contains(host, "xtralife.com") },
		Check: checkXtralife,
	},
	{
		Key:   "mediamarkt",
		Label: "MediaMarkt",
		Match: func(host string) bool { return strings.Contains(host, "mediamarkt.") },
		Check: checkMediaMarkt,
	},
	{
		Key:   "fnac",
		Label: "Fnac",
		Match: func(host string) bool { return host == "fnac.es" || strings.HasSuffix(host, ".fnac.es") },
		Check: checkFnac,
	},
	{
		Key:   "carrefour",
		Label: "Carrefour",
		Match: func(host string) bool { return host == "carrefour.es" || strings.HasSuffix(host, ".carrefour.es") },
		Check: checkCarrefour,
	},
	{
		Key:   "elcorteingles",
		Label: "El Corte Inglés",
		Match: func(host string) bool {
			return host == "elcorteingles.es" || strings.HasSuffix(host, ".elcorteingles.es")
		},
		Check: checkElCorteIngles,
	},
}

// DetectStore devuelve la tienda asociada a una URL.
func DetectStore(rawURL string) (*Store, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("URL no válida: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("la URL debe empezar por http:// o https://")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil, fmt.Errorf("la URL no tiene dominio")
	}
	for i := range Stores {
		if Stores[i].Match(host) {
			return &Stores[i], nil
		}
	}
	return nil, fmt.Errorf("tienda no soportada: %s", host)
}

// StoreLabel devuelve el nombre legible de una tienda por su clave.
func StoreLabel(key string) string {
	for _, s := range Stores {
		if s.Key == key {
			return s.Label
		}
	}
	return key
}

// SupportedStoresList devuelve un texto con las tiendas soportadas.
func SupportedStoresList() string {
	var b strings.Builder
	for i, s := range Stores {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(s.Label)
	}
	return b.String()
}

// CheckProduct detecta la tienda y comprueba el producto.
func CheckProduct(ctx context.Context, f *Fetcher, rawURL string) (CheckResult, error) {
	store, err := DetectStore(rawURL)
	if err != nil {
		return CheckResult{}, err
	}
	return store.Check(ctx, f, rawURL)
}

// --- GAME ---

var (
	gameInTexts = []string{
		"Añadir a la cesta", "Añadir al carrito", "Comprar ahora",
	}
	gameOutTexts = []string{
		"PRÓXIMAMENTE", "Próximamente", "Agotado", "No disponible",
	}
)

func checkGame(ctx context.Context, f *Fetcher, rawURL string) (CheckResult, error) {
	body, _, err := f.GetHTML(ctx, rawURL, "es-ES,es;q=0.9,en;q=0.8")
	if err != nil {
		return CheckResult{}, err
	}
	return checkByButtonText(body, gameInTexts, gameOutTexts)
}

// --- Xtralife ---

const xtralifeAPIBase = "https://api.xtralife.com/public-api/v1/"

func checkXtralife(ctx context.Context, f *Fetcher, rawURL string) (CheckResult, error) {
	id := xtralifeSKU(rawURL)
	if id == "" {
		return CheckResult{}, fmt.Errorf("no se pudo extraer el ID del producto de la URL de Xtralife")
	}

	api := xtralifeAPIBase + "sku?storefront_id=1&id=" + url.QueryEscape(id)
	body, _, err := f.GetJSON(ctx, api, "https://www.xtralife.com/", "es-ES,es;q=0.9,en;q=0.8")
	if err != nil {
		return CheckResult{}, err
	}

	var resp struct {
		Success bool `json:"success"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
		Body struct {
			Disponibility struct {
				Disponibility string `json:"disponibility"`
				Signable      int    `json:"signable"`
				Reservable    bool   `json:"reservable"`
			} `json:"disponibility"`
		} `json:"body"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return CheckResult{}, fmt.Errorf("respuesta no válida de la API de Xtralife: %w", err)
	}
	if !resp.Success {
		msg := "producto no encontrado"
		if resp.Error != nil && resp.Error.Message != "" {
			msg = resp.Error.Message
		}
		return CheckResult{}, fmt.Errorf("API de Xtralife: %s", msg)
	}

	d := resp.Body.Disponibility.Disponibility
	if st, ok := xtralifeStatus(d); ok {
		return CheckResult{Status: st, Detail: d}, nil
	}
	return CheckResult{Status: StatusUnknown, Detail: d}, nil
}

// --- Amazon ---

var (
	amazonInTexts = []string{
		"Añadir a la cesta", "Añadir al carrito", "Comprar en Preventa ya", "Comprar ya",
		"Add to Cart", "Add to Basket", "Buy Now",
		"In den Einkaufswagen", "Jetzt kaufen",
		"Ajouter au panier", "Acheter maintenant",
		"Aggiungi al Carrello", "Acquista ora",
		"In winkelwagen", "Nu kopen",
		"Lägg i varukorgen", "Köp nu",
		"Dodaj do koszyka", "Kup teraz",
	}
	amazonOutTexts = []string{
		"No disponible.", "Actualmente no disponible", "Temporalmente sin stock",
		"Currently unavailable", "Derzeit nicht verfügbar",
		"Actuellement indisponible", "Attualmente non disponibile",
		"Agotado", "Niet beschikbaar", "Ej tillgänglig", "Obecnie niedostępny",
	}
	amazonNotFoundTexts = []string{
		"Page Not Found", "We couldn't find that page", "no encontramos esa página",
	}
)

func checkAmazon(ctx context.Context, f *Fetcher, rawURL string) (CheckResult, error) {
	body, finalURL, err := f.GetHTML(ctx, rawURL, acceptLanguageForAmazon(rawURL))
	if err != nil {
		return CheckResult{}, err
	}
	if isAmazonCaptcha(body, finalURL) {
		return CheckResult{Status: StatusUnknown, Detail: "captcha de Amazon"}, nil
	}

	full := ""
	if doc, perr := parseHTML(body); perr == nil {
		full = normalize(visibleText(doc))
	}
	if _, ok := containsAny(full, amazonNotFoundTexts); ok {
		return CheckResult{Status: StatusUnknown, Detail: "producto no encontrado"}, nil
	}

	// El botón de compra es la señal principal. Si no existe, Amazon no
	// muestra nada (producto sin stock), así que el valor por defecto es
	// "agotado" salvo que haya un texto explícito de no disponibilidad.
	if t, ok := amazonBuyButtonText(body); ok {
		return CheckResult{Status: StatusInStock, Detail: t}, nil
	}
	if t, ok := containsAny(full, amazonOutTexts); ok {
		return CheckResult{Status: StatusOutOfStock, Detail: t}, nil
	}
	return CheckResult{Status: StatusOutOfStock, Detail: "sin botón de compra"}, nil
}

// amazonBuyButtonText busca el botón real de compra. Solo considera inputs y
// botones, y descarta el widget de atajos de teclado de Amazon (cuyo texto
// también dice "Añadir a la cesta"), que es la principal fuente de falsos
// positivos. Como señal adicional acepta los spans de anuncio accesible
// estables de Amazon ("...-announce").
func amazonBuyButtonText(body []byte) (string, bool) {
	doc, err := parseHTML(body)
	if err != nil {
		return "", false
	}
	var found string

	var walk func(n *html.Node, noise bool)
	walk = func(n *html.Node, noise bool) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode {
			if isAmazonNoise(n) {
				noise = true
			}
			if !noise {
				switch n.Data {
				case "input":
					if !isDisabled(n) {
						val := firstNonEmpty(attrValue(n, "value"), attrValue(n, "aria-label"))
						if t, ok := containsAny(normalize(val), amazonInTexts); ok {
							found = t
							return
						}
					}
				case "button":
					if !isDisabled(n) {
						if t, ok := containsAny(normalize(visibleText(n)), amazonInTexts); ok {
							found = t
							return
						}
					}
				}
				id := strings.ToLower(attrValue(n, "id"))
				if strings.Contains(id, "add-to-cart-announce") || strings.Contains(id, "buy-now-announce") {
					if t, ok := containsAny(normalize(visibleText(n)), amazonInTexts); ok {
						found = t
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, noise)
		}
	}
	walk(doc, false)

	return found, found != ""
}

// isAmazonNoise detecta nodos del asistente de atajos de teclado, que repiten
// el texto de los botones de compra sin serlo.
func isAmazonNoise(n *html.Node) bool {
	hay := strings.ToLower(attrValue(n, "id") + " " + attrValue(n, "class"))
	return strings.Contains(hay, "nav-assist") || strings.Contains(hay, "keyboard-shortcut")
}

func isDisabled(n *html.Node) bool {
	if hasAttr(n, "disabled") {
		return true
	}
	return strings.EqualFold(attrValue(n, "aria-disabled"), "true")
}

func isAmazonCaptcha(body []byte, finalURL string) bool {
	lower := strings.ToLower(string(body)) + " " + strings.ToLower(finalURL)
	return strings.Contains(lower, "validatecaptcha") ||
		strings.Contains(lower, "type the characters you see") ||
		strings.Contains(lower, "introduce los caracteres") ||
		strings.Contains(lower, "escribe los caracteres")
}

// --- MediaMarkt ---

var (
	mediaMarktInTexts = []string{
		"Añadir al carrito", "Añadir a la cesta", "Comprar ahora",
	}
	mediaMarktOutTexts = []string{
		"Este artículo no está disponible actualmente", "No está disponible",
		"Agotado", "No disponible",
	}
)

func checkMediaMarkt(ctx context.Context, f *Fetcher, rawURL string) (CheckResult, error) {
	body, _, err := f.GetHTML(ctx, rawURL, "es-ES,es;q=0.9,en;q=0.8")
	if err != nil {
		return CheckResult{}, err
	}
	if res, ok := checkJSONLDAvailability(body, rawURL); ok {
		return res, nil
	}
	return checkByButtonText(body, mediaMarktInTexts, mediaMarktOutTexts)
}

// --- Fnac ---

var (
	fnacInTexts  = []string{"Añadir a la cesta"}
	fnacOutTexts = []string{"No disponible en Fnac.es", "No disponible"}
)

func checkFnac(ctx context.Context, f *Fetcher, rawURL string) (CheckResult, error) {
	body, finalURL, err := f.GetHTML(ctx, rawURL, "es-ES,es;q=0.9,en;q=0.8")
	if looksLikeCaptcha(body, finalURL) {
		return CheckResult{Status: StatusUnknown, Detail: "captcha de Fnac"}, nil
	}
	if err != nil {
		return CheckResult{}, err
	}
	if t, ok := fnacBuyButtonText(body); ok {
		return CheckResult{Status: StatusInStock, Detail: t}, nil
	}
	if doc, perr := parseHTML(body); perr == nil {
		if t, ok := containsAny(normalize(visibleText(doc)), fnacOutTexts); ok {
			return CheckResult{Status: StatusOutOfStock, Detail: t}, nil
		}
	}
	return CheckResult{Status: StatusUnknown}, nil
}

// fnacBuyButtonText busca el botón de compra por su identificador estable de
// automatización (data-automation-id), que Fnac solo incluye cuando el
// producto se puede añadir a la cesta.
func fnacBuyButtonText(body []byte) (string, bool) {
	doc, err := parseHTML(body)
	if err != nil {
		return "", false
	}
	label := findNode(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode &&
			attrValue(n, "data-automation-id") == "product-buy-btn-label"
	})
	if label == nil || isDisabled(label) || isInsideDisabled(label) {
		return "", false
	}
	if t, ok := containsAny(normalize(visibleText(label)), fnacInTexts); ok {
		return t, true
	}
	return "", false
}

// isInsideDisabled indica si un nodo está dentro de un botón deshabilitado.
func isInsideDisabled(n *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && (p.Data == "button" || p.Data == "input") && isDisabled(p) {
			return true
		}
	}
	return false
}

// --- Carrefour ---

func checkCarrefour(ctx context.Context, f *Fetcher, rawURL string) (CheckResult, error) {
	body, finalURL, err := f.GetHTML(ctx, rawURL, "es-ES,es;q=0.9,en;q=0.8")
	if looksLikeCaptcha(body, finalURL) {
		return CheckResult{Status: StatusUnknown, Detail: "captcha de Carrefour"}, nil
	}
	if err != nil {
		return CheckResult{}, err
	}
	// El botón "Añadir" aparece siempre (también sin stock): el error solo se
	// muestra al pulsarlo. La disponibilidad fiable está en el JSON-LD.
	if res, ok := checkJSONLDAvailability(body, rawURL); ok {
		return res, nil
	}
	return CheckResult{Status: StatusUnknown, Detail: "sin datos de disponibilidad"}, nil
}

// --- El Corte Inglés ---

var elCorteInglesInTexts = []string{"Añadir a la cesta"}

func checkElCorteIngles(ctx context.Context, f *Fetcher, rawURL string) (CheckResult, error) {
	body, _, err := f.GetHTML(ctx, rawURL, "es-ES,es;q=0.9,en;q=0.8")
	// El Corte Inglés responde 410 Gone para productos agotados, pero el HTML
	// servido sigue incluyendo el botón, así que se analiza el cuerpo aunque
	// la respuesta tenga un código de error.
	if res, ok := elCorteInglesAvailability(body); ok {
		return res, nil
	}
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{Status: StatusUnknown}, nil
}

// elCorteInglesAvailability lee el botón principal de compra (id/testid
// estables). Un botón deshabilitado o con el texto "AGOTADO" significa que no
// hay stock.
func elCorteInglesAvailability(body []byte) (CheckResult, bool) {
	doc, err := parseHTML(body)
	if err != nil {
		return CheckResult{}, false
	}
	btn := findNode(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode &&
			(attrValue(n, "data-testid") == "pdp-add-to-cart" ||
				attrValue(n, "id") == "add_to_cart_main_button")
	})
	if btn == nil {
		return CheckResult{}, false
	}
	text := normalize(visibleText(btn))
	if isDisabled(btn) || strings.Contains(text, "agotado") {
		detail := strings.TrimSpace(visibleText(btn))
		if detail == "" {
			detail = "AGOTADO"
		}
		return CheckResult{Status: StatusOutOfStock, Detail: detail}, true
	}
	if t, ok := containsAny(text, elCorteInglesInTexts); ok {
		return CheckResult{Status: StatusInStock, Detail: t}, true
	}
	return CheckResult{Status: StatusUnknown}, false
}
