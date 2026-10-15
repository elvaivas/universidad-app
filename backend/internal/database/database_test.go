package database

import (
	"context"
	"os"
	"testing"
)

func TestNewFallaConURLInvalida(t *testing.T) {
	_, err := New(context.Background(), "postgres://nadie:nada@127.0.0.1:1/nada?sslmode=disable&connect_timeout=2")
	if err == nil {
		t.Fatal("esperaba error con URL que no responde")
	}
}

// TestNewConDBReal requiere TEST_DB_URL; se salta sola si no está definida.
func TestNewConDBReal(t *testing.T) {
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		t.Skip("TEST_DB_URL no definida; se omite (requiere docker compose up -d db)")
	}
	pool, err := New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()
}
