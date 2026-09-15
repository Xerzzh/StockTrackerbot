package main

import (
	"testing"
	"time"
)

func TestBackoffDuration(t *testing.T) {
	if got := backoffDuration(0); got != 0 {
		t.Fatalf("0 fallos = %v", got)
	}
	if got := backoffDuration(1); got != baseBackoff {
		t.Fatalf("1 fallo = %v", got)
	}
	if got := backoffDuration(2); got != 2*baseBackoff {
		t.Fatalf("2 fallos = %v", got)
	}
	if got := backoffDuration(100); got != maxBackoff {
		t.Fatalf("muchos fallos = %v", got)
	}
}

func TestWithJitterBounds(t *testing.T) {
	d := 100 * time.Second
	spread := time.Duration(float64(d) * jitterRatio)
	for i := 0; i < 1000; i++ {
		got := withJitter(d)
		if got < d-spread || got > d+spread {
			t.Fatalf("withJitter fuera de rango: %v", got)
		}
	}
	if got := withJitter(0); got != 0 {
		t.Fatalf("withJitter(0) = %v", got)
	}
}

func TestAddJitterBounds(t *testing.T) {
	d := 100 * time.Second
	spread := time.Duration(float64(d) * jitterRatio)
	for i := 0; i < 1000; i++ {
		got := addJitter(d)
		if got < d || got > d+spread {
			t.Fatalf("addJitter fuera de rango: %v", got)
		}
	}
}
