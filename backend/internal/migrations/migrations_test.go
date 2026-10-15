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
	if v != 9 {
		t.Errorf("versión tras Up: esperaba 9 (última migración), obtuvo %d", v)
	}

	v, err = Down(dbURL)
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if v != 8 {
		t.Errorf("versión tras Down: esperaba 8, obtuvo %d", v)
	}

	v, err = Up(dbURL)
	if err != nil {
		t.Fatalf("Up (segunda pasada): %v", err)
	}
	if v != 9 {
		t.Errorf("versión tras segundo Up: esperaba 9, obtuvo %d", v)
	}
}
