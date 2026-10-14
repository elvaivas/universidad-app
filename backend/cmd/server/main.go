package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/elvaivas/universidad-app/backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevelValue()}))
	slog.SetDefault(logger)

	logger.Info("config cargada",
		"env", cfg.Env,
		"puerto", cfg.HTTPPort,
		"db", cfg.DBURLMasked(),
	)
	// Bloque 2 conecta la DB y corre migraciones aquí.
}
