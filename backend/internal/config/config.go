// Package config centraliza la carga y validación de variables de entorno.
// Valores por defecto pensados para desarrollo local; en producción todo
// se define explícitamente en el entorno (nunca en el código).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env             string        // dev | prod
	HTTPPort        int
	DBURL           string        // postgres://...
	JWTSecret       string        // HS256, mínimo 32 bytes
	AccessTokenTTL  time.Duration // 15 min según S2
	RefreshTokenTTL time.Duration
	SMTPHost        string
	SMTPPort        int
	SMTPUser        string
	SMTPPass        string
	SMTPFrom        string
	MailMock        bool // true en dev: los tokens de reset van al log
	LogLevel        string
	CORSOrigins     []string // orígenes permitidos para el navegador (PWA/teléfono)
}

func Load() (*Config, error) {
	// .env es opcional y solo comodidad de desarrollo; nunca obligatorio.
	// El entorno real SIEMPRE gana sobre .env (semántica 12-factor).
	_ = godotenv.Load()

	cfg := &Config{
		Env:             get("APP_ENV", "dev"),
		HTTPPort:        getInt("HTTP_PORT", 8080),
		DBURL:           os.Getenv("DB_URL"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessTokenTTL:  getDur("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL: getDur("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        getInt("SMTP_PORT", 587),
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPass:        os.Getenv("SMTP_PASS"),
		SMTPFrom:        get("SMTP_FROM", "no-reply@localhost"),
		MailMock:        getBool("MAIL_MOCK", true),
		LogLevel:        get("LOG_LEVEL", "info"),
		CORSOrigins:     getList("CORS_ORIGINS", "http://localhost:5173"),
	}

	if cfg.JWTSecret == "" {
		return nil, errors.New("JWT_SECRET es obligatorio (openssl rand -base64 48)")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET debe tener al menos 32 bytes (tiene %d)", len(cfg.JWTSecret))
	}
	if cfg.DBURL == "" {
		return nil, errors.New("DB_URL es obligatorio (postgres://usuario:pass@host:5432/db)")
	}
	if cfg.Env == "prod" && cfg.MailMock {
		return nil, errors.New("MAIL_MOCK=true no es válido en APP_ENV=prod")
	}
	return cfg, nil
}

// DBURLMasked oculta credenciales para logs.
func (c *Config) DBURLMasked() string {
	const prefix = "postgres://"
	rest := c.DBURL
	if len(rest) > len(prefix) && rest[:len(prefix)] == prefix {
		rest = rest[len(prefix):]
		if i := indexByte(rest, '@'); i >= 0 {
			return prefix + "***@" + rest[i+1:]
		}
	}
	return prefix + "***"
}

func (c *Config) LogLevelValue() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getList(key, def string) []string {
	v := get(key, def)
	partes := strings.Split(v, ",")
	out := make([]string, 0, len(partes))
	for _, p := range partes {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
