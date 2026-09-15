package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	maxBodyBytes     = 8 << 20 // 8 MiB
)

// Fetcher es un cliente HTTP con cabeceras de navegador real.
type Fetcher struct {
	client    *http.Client
	userAgent string
}

// NewFetcher crea un Fetcher con el timeout indicado.
func NewFetcher(timeout time.Duration) *Fetcher {
	return &Fetcher{
		client:    &http.Client{Timeout: timeout},
		userAgent: defaultUserAgent,
	}
}

// newRequest construye una petición GET con cabeceras de navegador.
func (f *Fetcher) newRequest(ctx context.Context, rawURL, accept, acceptLang string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", accept)
	if acceptLang != "" {
		req.Header.Set("Accept-Language", acceptLang)
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	return req, nil
}

// GetHTML descarga una página HTML y devuelve el cuerpo y la URL final tras
// seguir redirecciones.
func (f *Fetcher) GetHTML(ctx context.Context, rawURL, acceptLang string) ([]byte, string, error) {
	req, err := f.newRequest(ctx, rawURL,
		"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		acceptLang)
	if err != nil {
		return nil, "", err
	}
	return f.do(req)
}

// GetJSON descarga un recurso JSON. El referer es opcional.
func (f *Fetcher) GetJSON(ctx context.Context, rawURL, referer, acceptLang string) ([]byte, string, error) {
	req, err := f.newRequest(ctx, rawURL, "application/json, text/plain, */*", acceptLang)
	if err != nil {
		return nil, "", err
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
		req.Header.Set("Origin", originOf(referer))
	}
	return f.do(req)
}

func (f *Fetcher) do(req *http.Request) ([]byte, string, error) {
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, "", fmt.Errorf("leyendo respuesta de %s: %w", req.URL, err)
	}
	finalURL := req.URL.String()
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	if resp.StatusCode >= 400 {
		return body, finalURL, &HTTPStatusError{
			StatusCode: resp.StatusCode,
			URL:        finalURL,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	return body, finalURL, nil
}

// HTTPStatusError representa una respuesta HTTP con código de error. Incluye
// el Retry-After indicado por el servidor, si lo hubiera.
type HTTPStatusError struct {
	StatusCode int
	URL        string
	RetryAfter time.Duration
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("respuesta HTTP %d para %s", e.StatusCode, e.URL)
}

// Retryable indica si el error conviene reintentarlo con backoff: límite de
// peticiones (429) o errores del servidor (5xx).
func (e *HTTPStatusError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// parseRetryAfter interpreta la cabecera Retry-After, que puede ser un número
// de segundos o una fecha HTTP.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// retryDelay decide si un error merece backoff y, cuando el servidor lo indicó,
// el retraso sugerido vía Retry-After. Los errores de red (timeouts, cortes)
// también se reintentan con backoff calculado.
func retryDelay(err error) (time.Duration, bool) {
	var he *HTTPStatusError
	if errors.As(err, &he) {
		return he.RetryAfter, he.Retryable()
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return 0, true
	}
	return 0, false
}

func originOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// acceptLanguageForAmazon devuelve el idioma adecuado según el dominio de
// Amazon (soporta las variantes europeas).
func acceptLanguageForAmazon(rawURL string) string {
	host := strings.ToLower(rawURL)
	switch {
	case strings.Contains(host, "amazon.de"):
		return "de-DE,de;q=0.9,en;q=0.8"
	case strings.Contains(host, "amazon.fr"):
		return "fr-FR,fr;q=0.9,en;q=0.8"
	case strings.Contains(host, "amazon.it"):
		return "it-IT,it;q=0.9,en;q=0.8"
	case strings.Contains(host, "amazon.co.uk"):
		return "en-GB,en;q=0.9"
	case strings.Contains(host, "amazon.nl"):
		return "nl-NL,nl;q=0.9,en;q=0.8"
	case strings.Contains(host, "amazon.se"):
		return "sv-SE,sv;q=0.9,en;q=0.8"
	case strings.Contains(host, "amazon.pl"):
		return "pl-PL,pl;q=0.9,en;q=0.8"
	case strings.Contains(host, "amazon.com.be"):
		return "nl-BE,nl;q=0.9,fr;q=0.8,en;q=0.7"
	default: // amazon.es y resto
		return "es-ES,es;q=0.9,en;q=0.8"
	}
}
