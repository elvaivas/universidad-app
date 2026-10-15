// Package migrations versiona el esquema de la base de datos.
// Los archivos .sql van embebidos en el binario: el servidor aplica las
// migraciones pendientes al arrancar (S8: versionadas y reversibles).
package migrations

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var archivos embed.FS

func nuevoMigrate(dbURL string) (*migrate.Migrate, error) {
	src, err := iofs.New(archivos, ".")
	if err != nil {
		return nil, fmt.Errorf("fuente iofs: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dbURL)
	if err != nil {
		return nil, fmt.Errorf("crear migrate: %w", err)
	}
	return m, nil
}

// Up aplica las migraciones pendientes y devuelve la versión resultante.
func Up(dbURL string) (uint, error) {
	m, err := nuevoMigrate(dbURL)
	if err != nil {
		return 0, err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("aplicar migraciones: %w", err)
	}
	v, _, err := m.Version()
	if err != nil {
		return 0, fmt.Errorf("leer versión: %w", err)
	}
	return v, nil
}

// Down revierte la última migración aplicada (reversible = S8).
// OJO: migrate.Down() del vendored revierte TODAS; Steps(-1) revierte una.
func Down(dbURL string) (uint, error) {
	m, err := nuevoMigrate(dbURL)
	if err != nil {
		return 0, err
	}
	defer m.Close()

	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("revertir migración: %w", err)
	}
	v, _, err := m.Version()
	if err != nil {
		return 0, nil // sin migraciones aplicadas: versión 0
	}
	return v, nil
}
