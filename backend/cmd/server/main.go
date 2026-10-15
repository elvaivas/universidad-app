package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elvaivas/universidad-app/backend/internal/academic"
	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/catalog"
	"github.com/elvaivas/universidad-app/backend/internal/chat"
	"github.com/elvaivas/universidad-app/backend/internal/config"
	"github.com/elvaivas/universidad-app/backend/internal/database"
	"github.com/elvaivas/universidad-app/backend/internal/feed"
	"github.com/elvaivas/universidad-app/backend/internal/httpapi"
	"github.com/elvaivas/universidad-app/backend/internal/migrations"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "seed-admin" {
		if err := ejecutarSeedAdmin(); err != nil {
			fmt.Fprintln(os.Stderr, "seed-admin:", err)
			os.Exit(1)
		}
		return
	}

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

	svc := &auth.Servicio{
		Pool:       pool,
		JWTSecreto: cfg.JWTSecret,
		AccessTTL:  cfg.AccessTokenTTL,
		RefreshTTL: cfg.RefreshTokenTTL,
	}
	httpSrv := httpapi.NuevoServer(svc, cfg.JWTSecret, 0.5, 5)
	httpSrv.HabilitarCORS(cfg.CORSOrigins)
	httpSrv.MontarCatalogo(&catalog.Servicio{Pool: pool})
	httpSrv.Montar2FA(svc)
	httpSrv.MontarAcademico(&academic.Servicio{Pool: pool})
	httpSrv.MontarFeed(&feed.Servicio{Pool: pool})
	httpSrv.MontarChat(&chat.Servicio{Pool: pool}, chat.NuevoHub())

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           httpSrv.Handler(), // S2: 0.5 req/s sostenido, ráfaga 5
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("http escuchando", "puerto", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http falló", "error", err)
			os.Exit(1)
		}
	}()

	// apagado elegante: SIGINT/SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("apagando...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

// ejecutarSeedAdmin crea el primer mantenedor:
//   go run ./backend/cmd/server seed-admin --email admin@uni.edu --password '...'
func ejecutarSeedAdmin() error {
	fs := flag.NewFlagSet("seed-admin", flag.ExitOnError)
	email := fs.String("email", "", "email del mantenedor (obligatorio)")
	password := fs.String("password", "", "contraseña inicial (obligatoria; se exige cambio al primer login)")
	nombre := fs.String("nombre", "Mantenedor Inicial", "nombre visible")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *email == "" || *password == "" {
		return fmt.Errorf("--email y --password son obligatorios")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := database.New(context.Background(), cfg.DBURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if _, err := migrations.Up(cfg.DBURL); err != nil {
		return fmt.Errorf("migraciones: %w", err)
	}

	svc := &auth.Servicio{Pool: pool, JWTSecreto: cfg.JWTSecret,
		AccessTTL: cfg.AccessTokenTTL, RefreshTTL: cfg.RefreshTokenTTL}
	if _, err := svc.CrearMantenedorInicial(context.Background(), *email, *password, *nombre); err != nil {
		return err
	}
	fmt.Printf("mantenedor creado: %s (force_password_change=true)\n", *email)
	return nil
}
