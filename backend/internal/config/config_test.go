package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// quita las vars del entorno para simular que no existen y las restaura al terminar.
func limpiarEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"APP_ENV", "HTTP_PORT", "DB_URL", "JWT_SECRET", "ACCESS_TOKEN_TTL",
		"REFRESH_TOKEN_TTL", "SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASS",
		"SMTP_FROM", "MAIL_MOCK", "LOG_LEVEL",
	}
	guardadas := map[string]*string{}
	for _, k := range vars {
		if v, ok := os.LookupEnv(k); ok {
			v := v
			guardadas[k] = &v
		}
		_ = os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range vars {
			_ = os.Unsetenv(k)
		}
		for k, v := range guardadas {
			_ = os.Setenv(k, *v)
		}
	})
}

func TestLoadDefaultsDev(t *testing.T) {
	limpiarEnv(t)
	t.Setenv("DB_URL", "postgres://u:p@localhost:5432/app")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env: esperaba dev, obtuvo %q", cfg.Env)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort: esperaba 8080, obtuvo %d", cfg.HTTPPort)
	}
	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Errorf("AccessTokenTTL: esperaba 15m, obtuvo %v", cfg.AccessTokenTTL)
	}
	if !cfg.MailMock {
		t.Error("MailMock: en dev debe ser true por defecto")
	}
}

func TestLoadFallaSinJWTSecret(t *testing.T) {
	limpiarEnv(t)
	t.Setenv("DB_URL", "postgres://u:p@localhost:5432/app")

	if _, err := Load(); err == nil {
		t.Fatal("esperaba error por JWT_SECRET vacío")
	}
}

func TestLoadFallaConSecretCorto(t *testing.T) {
	limpiarEnv(t)
	t.Setenv("DB_URL", "postgres://u:p@localhost:5432/app")
	t.Setenv("JWT_SECRET", "corto")

	if _, err := Load(); err == nil {
		t.Fatal("esperaba error por JWT_SECRET < 32 bytes")
	}
}

func TestLoadFallaSinDBURL(t *testing.T) {
	limpiarEnv(t)
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	if _, err := Load(); err == nil {
		t.Fatal("esperaba error por DB_URL vacía")
	}
}

func TestLoadRechazaMailMockEnProd(t *testing.T) {
	limpiarEnv(t)
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DB_URL", "postgres://u:p@localhost:5432/app")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")

	if _, err := Load(); err == nil {
		t.Fatal("esperaba error: MAIL_MOCK=true no permitido en prod")
	}
}

func TestLoadLeeArchivoDotEnv(t *testing.T) {
	limpiarEnv(t)
	dir := t.TempDir()
	contenido := "DB_URL=postgres://u:p@db:5432/app\nJWT_SECRET=0123456789abcdef0123456789abcdef\nHTTP_PORT=9090\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPPort != 9090 {
		t.Errorf("HTTPPort: esperaba 9090 (desde .env), obtuvo %d", cfg.HTTPPort)
	}
}

func TestEnvRealGanaSobreDotEnv(t *testing.T) {
	limpiarEnv(t)
	dir := t.TempDir()
	contenido := "DB_URL=postgres://u:p@db:5432/app\nJWT_SECRET=0123456789abcdef0123456789abcdef\nHTTP_PORT=9090\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	t.Setenv("HTTP_PORT", "7777") // entorno real debe imponerse sobre .env

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPPort != 7777 {
		t.Errorf("HTTPPort: esperaba 7777 (env real gana sobre .env), obtuvo %d", cfg.HTTPPort)
	}
}

func TestDBURLMasked(t *testing.T) {
	c := &Config{DBURL: "postgres://admin:supersecret@db.ejemplo.com:5432/appdb"}
	if got := c.DBURLMasked(); got != "postgres://***@db.ejemplo.com:5432/appdb" {
		t.Errorf("DBURLMasked: obtuvo %q", got)
	}
}
