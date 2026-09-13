package main

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// parseHTML convierte un documento HTML en un árbol DOM.
func parseHTML(body []byte) (*html.Node, error) {
	return html.Parse(bytes.NewReader(body))
}

// skipTextTags son etiquetas cuyo contenido no forma parte del texto visible.
var skipTextTags = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
	"template": true,
	"head":     true,
	"title":    true,
	"svg":      true,
}

// visibleText devuelve el texto visible del documento, ignorando scripts,
// estilos y demás contenido no renderizado. Los espacios se normalizan.
func visibleText(n *html.Node) string {
	var b strings.Builder
	collectVisibleText(n, &b)
	return strings.Join(strings.Fields(b.String()), " ")
}

func collectVisibleText(n *html.Node, b *strings.Builder) {
	if n.Type == html.ElementNode && skipTextTags[n.Data] {
		return
	}
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		b.WriteString(" ")
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectVisibleText(c, b)
	}
}

// actionTags son etiquetas que suelen actuar como botones o enlaces.
var actionTags = map[string]bool{
	"button": true,
	"a":      true,
	"input":  true,
}

// actionTexts devuelve el texto de los elementos "accionables" (botones,
// enlaces, inputs). Es la fuente más fiable para detectar un botón de compra
// sin depender de clases o ids autogenerados.
func actionTexts(n *html.Node) []string {
	var texts []string
	collectActionTexts(n, &texts)
	return texts
}

func collectActionTexts(n *html.Node, texts *[]string) {
	if n.Type == html.ElementNode {
		if skipTextTags[n.Data] {
			return
		}
		role := attrValue(n, "role")
		if actionTags[n.Data] || role == "button" {
			var text string
			switch n.Data {
			case "input":
				text = firstNonEmpty(attrValue(n, "value"), attrValue(n, "aria-label"), attrValue(n, "title"))
			default:
				text = strings.Join(strings.Fields(visibleText(n)), " ")
			}
			if strings.TrimSpace(text) != "" {
				*texts = append(*texts, text)
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectActionTexts(c, texts)
	}
}

// attrValue devuelve el valor de un atributo o "" si no existe.
func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// hasAttr indica si un atributo está presente, aunque sea booleano (valor "").
func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
