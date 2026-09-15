package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// diacritics permite normalizar texto eliminando tildes para comparaciones.
var diacritics = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
	"ü", "u", "à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u",
	"ñ", "n", "ç", "c", "ä", "a", "ö", "o", "ë", "e", "ï", "i",
)

// normalize pasa a minúsculas, elimina tildes y colapsa espacios.
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = diacritics.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// containsAny comprueba si alguno de los textos (normalizados) está contenido
// en el haystack (que debe venir ya normalizado).
func containsAny(haystack string, needles []string) (string, bool) {
	for _, n := range needles {
		norm := normalize(n)
		if norm == "" {
			continue
		}
		if strings.Contains(haystack, norm) {
			return n, true
		}
	}
	return "", false
}

// checkByButtonText busca los textos de compra en los elementos accionables
// (botones/enlaces) y, como respaldo, en todo el texto visible. Los textos de
// "agotado" solo se buscan en el texto visible.
//
// El orden de prioridad es:
//  1. Texto de compra dentro de un botón/enlace -> Disponible.
//  2. Texto de agotado en el texto visible     -> Agotado.
//  3. Texto de compra en cualquier texto       -> Disponible.
//  4. Nada de lo anterior                      -> Desconocido.
func checkByButtonText(body []byte, inTexts, outTexts []string) (CheckResult, error) {
	doc, err := parseHTML(body)
	if err != nil {
		return CheckResult{}, fmt.Errorf("parseando HTML: %w", err)
	}

	full := normalize(visibleText(doc))
	actions := normalize(strings.Join(actionTexts(doc), "\n"))

	if t, ok := containsAny(actions, inTexts); ok {
		return CheckResult{Status: StatusInStock, Detail: t}, nil
	}
	if t, ok := containsAny(full, outTexts); ok {
		return CheckResult{Status: StatusOutOfStock, Detail: t}, nil
	}
	if t, ok := containsAny(full, inTexts); ok {
		return CheckResult{Status: StatusInStock, Detail: t}, nil
	}
	return CheckResult{Status: StatusUnknown}, nil
}

// --- Detección de captchas ---

// captchaMarkers son fragmentos que delatan páginas de verificación anti-bots
// (DataDome, Cloudflare, PerimeterX, hCaptcha, reCAPTCHA...).
var captchaMarkers = []string{
	"captcha-delivery.com",
	"datadome",
	"geo.captcha-delivery",
	"px-captcha",
	"perimeterx",
	"hcaptcha",
	"g-recaptcha",
	"cf-chl",
	"_cf_chl",
	"challenges.cloudflare.com",
	"just a moment",
	"turnstile",
	"please enable js and disable any ad blocker",
}

// looksLikeCaptcha indica si el cuerpo (o la URL final) corresponde a una
// página de captcha/verificación en lugar de la página del producto.
func looksLikeCaptcha(body []byte, finalURL string) bool {
	haystack := strings.ToLower(string(body) + " " + finalURL)
	for _, m := range captchaMarkers {
		if strings.Contains(haystack, m) {
			return true
		}
	}
	return false
}

// --- Detección vía datos estructurados (schema.org) ---

var ldScriptRe = regexp.MustCompile(`(?is)<script[^>]*type=["']application/ld\+json["'][^>]*>(.*?)</script>`)

type productAvailability struct {
	availability string
	url          string
}

// checkJSONLDAvailability busca un Product en los bloques JSON-LD y devuelve
// su disponibilidad. Si hay varios productos, prioriza el que coincide con la
// URL solicitada. Los ProductGroup (con hasVariant) se recogen antes que sus
// variantes, de modo que para la URL consultada manda la oferta del grupo y no
// la disponibilidad individual de una variante.
func checkJSONLDAvailability(body []byte, productURL string) (CheckResult, bool) {
	var products []productAvailability
	for _, m := range ldScriptRe.FindAllSubmatch(body, -1) {
		raw := html.UnescapeString(strings.TrimSpace(string(m[1])))
		var v interface{}
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			continue
		}
		collectProductAvailability(v, &products)
	}
	if len(products) == 0 {
		return CheckResult{}, false
	}

	want := normalizePath(productURL)
	for _, p := range products {
		if p.availability == "" {
			continue
		}
		if want != "" && normalizePath(p.url) == want {
			if st, ok := availabilityFromSchema(p.availability); ok {
				return CheckResult{Status: st, Detail: p.availability}, true
			}
		}
	}
	if len(products) == 1 && products[0].availability != "" {
		if st, ok := availabilityFromSchema(products[0].availability); ok {
			return CheckResult{Status: st, Detail: products[0].availability}, true
		}
	}
	return CheckResult{}, false
}

func collectProductAvailability(v interface{}, out *[]productAvailability) {
	switch t := v.(type) {
	case map[string]interface{}:
		if isProductType(t["@type"]) {
			pa := productAvailability{url: stringValue(t["url"])}
			for _, off := range offersList(t["offers"]) {
				if av := stringValue(off["availability"]); av != "" {
					pa.availability = av
					if u := stringValue(off["url"]); u != "" {
						pa.url = u
					}
					break
				}
			}
			*out = append(*out, pa)
		}
		for _, val := range t {
			collectProductAvailability(val, out)
		}
	case []interface{}:
		for _, e := range t {
			collectProductAvailability(e, out)
		}
	}
}

// isProductType indica si un @type de schema.org corresponde a un producto
// (Product) o a un grupo de variantes (ProductGroup). Se incluye ProductGroup
// porque MediaMarkt declara la oferta del producto visible a nivel de grupo,
// mientras que hasVariant puede contener disponibilidades desactualizadas.
func isProductType(v interface{}) bool {
	switch t := v.(type) {
	case string:
		return strings.EqualFold(t, "Product") || strings.EqualFold(t, "ProductGroup")
	case []interface{}:
		for _, e := range t {
			if s, ok := e.(string); ok && (strings.EqualFold(s, "Product") || strings.EqualFold(s, "ProductGroup")) {
				return true
			}
		}
	}
	return false
}

func offersList(v interface{}) []map[string]interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return []map[string]interface{}{t}
	case []interface{}:
		var out []map[string]interface{}
		for _, e := range t {
			if m, ok := e.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// availabilityFromSchema traduce un valor schema.org a un Status.
func availabilityFromSchema(av string) (Status, bool) {
	a := strings.ToLower(av)
	switch {
	case strings.Contains(a, "instock"),
		strings.Contains(a, "preorder"),
		strings.Contains(a, "presale"),
		strings.Contains(a, "backorder"),
		strings.Contains(a, "limitedavailability"),
		strings.Contains(a, "onlineonly"):
		return StatusInStock, true
	case strings.Contains(a, "outofstock"),
		strings.Contains(a, "soldout"),
		strings.Contains(a, "discontinued"),
		strings.Contains(a, "notavailable"):
		return StatusOutOfStock, true
	}
	return StatusUnknown, false
}

// normalizePath devuelve la ruta de una URL en minúsculas y sin barra final,
// para comparar URLs de producto ignorando parámetros y mayúsculas.
func normalizePath(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return strings.TrimSuffix(strings.ToLower(rawURL), "/")
	}
	return strings.TrimSuffix(strings.ToLower(u.Path), "/")
}

// --- Xtralife ---

// xtralifeStatus traduce el campo "disponibility" de la API pública de
// Xtralife a un Status. "sell" y "reservation" implican botón de compra o de
// "paga y señal"; el resto de estados no permiten comprar.
func xtralifeStatus(disponibility string) (Status, bool) {
	switch strings.ToLower(strings.TrimSpace(disponibility)) {
	case "sell", "reservation":
		return StatusInStock, true
	case "out_of_stock", "maybe_out_of_stock",
		"reservation_out_of_stock", "reservation_maybe_out_of_stock",
		"reservation_not_opened", "not_for_sale", "archived", "unpublished",
		"restock", "restock_maybe_out_of_stock":
		return StatusOutOfStock, true
	}
	return StatusUnknown, false
}

// xtralifeSKU extrae el identificador numérico final de la URL de producto.
func xtralifeSKU(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if last == "" {
		return ""
	}
	for _, r := range last {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return last
}

// --- Nintendo Store ---

// nintendoProductID extrae el identificador de producto del slug de la URL.
// Nintendo añade el id como último token tras un guion, y puede ser un SKU
// alfanumérico (P00211) o un id numérico de la eShop (70010000096802).
func nintendoProductID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	slug := parts[len(parts)-1]
	if slug == "" {
		return ""
	}
	if i := strings.LastIndex(slug, "-"); i >= 0 {
		slug = slug[i+1:]
	}
	return slug
}

// nintendoAvailability traduce la respuesta de la API de producto de Nintendo
// Store. "inventory" es la señal directa (orderable/preorderable); el tipo de
// "c_availabilityModel" sirve de respaldo y como detalle legible.
func nintendoAvailability(orderable, preorderable bool, availabilityType string) (Status, string) {
	switch {
	case orderable:
		return StatusInStock, "orderable"
	case preorderable:
		return StatusInStock, "preorderable"
	}
	switch strings.ToLower(strings.TrimSpace(availabilityType)) {
	case "instock", "available", "preorder", "backorder", "onlineonly":
		return StatusInStock, availabilityType
	case "outofstock", "soldout", "notavailable", "unavailable", "comingsoon", "discontinued":
		return StatusOutOfStock, availabilityType
	}
	return StatusUnknown, availabilityType
}
