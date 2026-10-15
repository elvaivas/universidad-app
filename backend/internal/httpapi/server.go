// Capa HTTP del backend: router chi, handlers JSON uniformes {data|error},
// rate limit en memoria por IP para /auth/* y middleware de recuperación.
// Las rutas autenticadas usan rbac.RequireAuth; /api/admin/* exige mantenedor.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/rbac"
)

// ServicioAuth es la superficie que el HTTP necesita del módulo auth
// (se cumple con *auth.Servicio; interfaz para tests con dobles).
type ServicioAuth interface {
	Login(ctx context.Context, email, password, ip, ua string) (*auth.ParTokens, error)
	Refresh(ctx context.Context, refreshToken, ip, ua string) (*auth.ParTokens, error)
}

type Server struct {
	svc    ServicioAuth
	router chi.Router
	lim    *limiter
}

func NuevoServer(svc ServicioAuth, jwtSecreto string, limiteRPS float64, rafaga int) *Server {
	s := &Server{svc: svc, lim: nuevoLimiter(limiteRPS, rafaga)}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(slogChi)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
	})

	r.Route("/api/auth", func(r chi.Router) {
		r.Use(s.lim.middleware) // S2: rate limit estricto en auth
		r.Post("/login", s.login)
		r.Post("/refresh", s.refresh)
	})

	// rutas autenticadas: el JWT es el pasaporte (S3)
	r.Group(func(r chi.Router) {
		r.Use(rbac.RequireAuth(jwtSecreto))
		r.Get("/api/auth/me", s.me)
	})

	// placeholder del módulo admin: solo mantenedores
	r.With(rbac.RequireAuth(jwtSecreto), rbac.RequireMantenedor).
		Get("/api/admin/ping", func(w http.ResponseWriter, _ *http.Request) {
			escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"pong": true}})
		})
	s.router = r
	return s
}

func (s *Server) Handler() http.Handler { return s.router }

// --- handlers ---

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		escribirError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		escribirError(w, http.StatusBadRequest, "email y password son obligatorios")
		return
	}

	par, err := s.svc.Login(r.Context(), req.Email, req.Password, ipDe(r), r.UserAgent())
	switch {
	case errors.Is(err, auth.ErrCredenciales):
		escribirError(w, http.StatusUnauthorized, "credenciales inválidas")
		return
	case errors.Is(err, auth.ErrUsuarioInactivo):
		escribirError(w, http.StatusForbidden, "usuario inactivo")
		return
	case errors.Is(err, auth.ErrRequiere2FA):
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"requiere_2fa": true}})
		return
	case err != nil:
		slog.Error("login interno", "error", err)
		escribirError(w, http.StatusInternalServerError, "error interno")
		return
	}
	responderPar(w, par)
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		escribirError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	par, err := s.svc.Refresh(r.Context(), req.RefreshToken, ipDe(r), r.UserAgent())
	if errors.Is(err, auth.ErrRefreshInvalido) {
		escribirError(w, http.StatusUnauthorized, "refresh inválido o expirado")
		return
	}
	if err != nil {
		slog.Error("refresh interno", "error", err)
		escribirError(w, http.StatusInternalServerError, "error interno")
		return
	}
	responderPar(w, par)
}

// me devuelve las claims del portador: útil para el frontend y para tests.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, map[string]any{"data": rbac.ClaimsDe(r)})
}

func responderPar(w http.ResponseWriter, par *auth.ParTokens) {
	// refresh también como cookie httpOnly (camino del navegador; el body lo usa la PWA)
	http.SetCookie(w, &http.Cookie{
		Name: "refresh_token", Value: par.RefreshToken, Path: "/api/auth",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: false, // Secure=true con HTTPS (VPS)
		Expires: time.Now().Add(30 * 24 * time.Hour),
	})
	escribirJSON(w, http.StatusOK, map[string]any{"data": par})
}

// --- helpers ---

func escribirJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func escribirError(w http.ResponseWriter, status int, mensaje string) {
	escribirJSON(w, status, map[string]any{"error": mensaje})
}

func ipDe(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func slogChi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http", "metodo", r.Method, "ruta", r.URL.Path,
			"status", ww.Status(), "ms", time.Since(inicio).Milliseconds())
	})
}

// --- rate limiter (token bucket en memoria, por IP) ---

type cubeta struct {
	tokens float64
	ultimo time.Time
}

type limiter struct {
	mu      sync.Mutex
	cubetas map[string]*cubeta
	rate    float64 // tokens/segundo
	max     float64 // ráfaga
}

func nuevoLimiter(rate float64, rafaga int) *limiter {
	return &limiter{cubetas: map[string]*cubeta{}, rate: rate, max: float64(rafaga)}
}

func (l *limiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ipDe(r)
		l.mu.Lock()
		c, ok := l.cubetas[ip]
		if !ok {
			c = &cubeta{tokens: l.max, ultimo: time.Now()}
			l.cubetas[ip] = c
		}
		ahora := time.Now()
		c.tokens += ahora.Sub(c.ultimo).Seconds() * l.rate
		if c.tokens > l.max {
			c.tokens = l.max
		}
		c.ultimo = ahora
		if c.tokens < 1 {
			l.mu.Unlock()
			escribirError(w, http.StatusTooManyRequests, "demasiados intentos; espera unos segundos")
			return
		}
		c.tokens--
		l.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}
