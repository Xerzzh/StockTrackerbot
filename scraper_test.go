package main

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter(""); got != 0 {
		t.Fatalf("vacío = %v", got)
	}
	if got := parseRetryAfter("30"); got != 30*time.Second {
		t.Fatalf("30 = %v", got)
	}
	if got := parseRetryAfter("0"); got != 0 {
		t.Fatalf("0 = %v", got)
	}
	if got := parseRetryAfter("no-numero"); got != 0 {
		t.Fatalf("inválido = %v", got)
	}
	future := time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 {
		t.Fatalf("fecha futura = %v", got)
	}
	past := time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(past); got != 0 {
		t.Fatalf("fecha pasada = %v", got)
	}
}

func TestRetryDelay(t *testing.T) {
	if _, ok := retryDelay(&HTTPStatusError{StatusCode: http.StatusTooManyRequests}); !ok {
		t.Fatal("429 debería ser reintentable")
	}
	if _, ok := retryDelay(&HTTPStatusError{StatusCode: http.StatusInternalServerError}); !ok {
		t.Fatal("500 debería ser reintentable")
	}
	if _, ok := retryDelay(&HTTPStatusError{StatusCode: http.StatusNotFound}); ok {
		t.Fatal("404 no debería ser reintentable")
	}
	d, ok := retryDelay(&HTTPStatusError{StatusCode: http.StatusTooManyRequests, RetryAfter: 42 * time.Second})
	if !ok || d != 42*time.Second {
		t.Fatalf("Retry-After = %v, %v", d, ok)
	}
	if _, ok := retryDelay(&url.Error{Op: "Get", URL: "https://x", Err: errors.New("boom")}); !ok {
		t.Fatal("error de red debería ser reintentable")
	}
	if _, ok := retryDelay(errors.New("otro")); ok {
		t.Fatal("error genérico no debería ser reintentable")
	}
}
