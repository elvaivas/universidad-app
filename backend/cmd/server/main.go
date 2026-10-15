package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/elvaivas/universidad-app/backend/internal/config"
	"github.com/elvaivas/universidad-app/backend/internal/database"
	"github.com/elvaivas/universidad-app/backend/internal/migrations"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevelValue()}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.New(ctx, cfg.DBURL)
	if err != nil {
		logger.Error("postgres no disponible", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("postgres conectado", "db", cfg.DBURLMasked())

	version, err := migrations.Up(cfg.DBURL)
	if err != nil {
		logger.Error("migraciones fallaron", "error", err)
		os.Exit(1)
	}
	logger.Info("migraciones al día", "version", version)

	// Bloque 3 levanta el HTTP server (chi) y el módulo auth aquí.
}
