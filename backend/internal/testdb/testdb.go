// Package testdb crea bases de datos PostgreSQL efímeras y migradas para
// tests de integración: cada paquete de tests trabaja aislado, en paralelo
// con los demás. Requiere TEST_DB_URL con permiso de CREATE DATABASE
// (en dev el usuario del contenedor es superusuario; se omite si no hay URL).
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elvaivas/universidad-app/backend/internal/migrations"
)

// Nueva crea una base migrada desde cero y registra su destrucción al terminar.
func Nueva(t testing.TB) string {
	t.Helper()
	adminURL := strings.TrimSpace(os.Getenv("TEST_DB_URL"))
	if adminURL == "" {
		t.Skip("TEST_DB_URL no definida; se omite test de integración")
	}

	sufijo := make([]byte, 6)
	_, _ = rand.Read(sufijo)
	nombre := "test_" + hex.EncodeToString(sufijo)

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("TEST_DB_URL inválida: %v", err)
	}
	host := u.Host
	if host == "" {
		t.Fatal("TEST_DB_URL sin host")
	}

	admin, err := pgxpool.New(context.Background(), fmt.Sprintf("postgres://%s@%s/postgres?sslmode=disable", u.User, host))
	if err == nil {
		_, err = admin.Exec(context.Background(), fmt.Sprintf("CREATE DATABASE %s", nombre))
		admin.Close()
	}
	if err != nil {
		t.Skipf("no se pudo crear base de pruebas (¿CREATE DATABASE permitido?): %v", err)
	}

	nueva := *u
	nueva.Path = "/" + nombre
	dsn := nueva.String()

	if _, err := migrations.Up(dsn); err != nil {
		t.Fatalf("migrar base de pruebas: %v", err)
	}

	t.Cleanup(func() {
		admin, err := pgxpool.New(context.Background(), fmt.Sprintf("postgres://%s@%s/postgres?sslmode=disable", u.User, host))
		if err != nil {
			return
		}
		defer admin.Close()
		_, _ = admin.Exec(context.Background(),
			fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", nombre))
	})
	return dsn
}
