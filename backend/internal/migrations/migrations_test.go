package migrations

import (
	"os"
	"testing"
)

// TestUpDownCycle requiere TEST_DB_URL apuntando a una base DEDICADA de pruebas
// (el ciclo deja la base como estaba). Se salta sola si no está definida.
func TestUpDownCycle(t *testing.T) {
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		t.Skip("TEST_DB_URL no definida; se omite (requiere docker compose up -d db)")
	}

	v, err := Up(dbURL)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if v != 7 {
		t.Errorf("versión tras Up: esperaba 7 (última migración), obtuvo %d", v)
	}

	v, err = Down(dbURL)
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if v != 6 {
		t.Errorf("versión tras Down: esperaba 6, obtuvo %d", v)
	}

	v, err = Up(dbURL)
	if err != nil {
		t.Fatalf("Up (segunda pasada): %v", err)
	}
	if v != 7 {
		t.Errorf("versión tras segundo Up: esperaba 7, obtuvo %d", v)
	}
}
