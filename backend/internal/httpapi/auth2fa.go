// Rutas de segundo factor (TOTP), cambio de contraseña, cierre de sesión
// y estado de seguridad del perfil.
// Montar2FA se llama desde main.go después de NuevoServer: conserva el
// server_test.go intacto (toma *auth.Servicio concreto, no la interfaz).
package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
	"github.com/elvaivas/universidad-app/backend/internal/rbac"
)

func (s *Server) Montar2FA(svc *auth.Servicio) {
	// segundo paso del login: público pero rate-limited
	s.router.With(s.lim.middleware).Post("/api/auth/2fa/login", login2FA(svc))
	s.router.With(s.lim.middleware).Post("/api/auth/logout", logout(svc))

	s.router.Group(func(r chi.Router) {
		r.Use(rbac.RequireAuth(s.jwtSecreto))
		r.Get("/api/auth/seguridad", seguridad(svc))
		r.Post("/api/auth/2fa/activar", activar2FA(svc))
		r.Post("/api/auth/2fa/confirmar", confirmar2FA(svc))
		r.Post("/api/auth/2fa/desactivar", desactivar2FA(svc))
		r.Post("/api/auth/password", cambiarPassword(svc))
	})
}

func uidDe(r *http.Request) string {
	if c := rbac.ClaimsDe(r); c != nil {
		return c.Subject
	}
	return ""
}

// seguridad devuelve el estado de seguridad del usuario (perfil / 2FA).
func seguridad(svc *auth.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		est, err := svc.Seguridad(r.Context(), uidDe(r))
		if err != nil {
			escribirError(w, http.StatusUnauthorized, "sesión no válida")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": est})
	}
}

type codigoReq struct {
	Codigo string `json:"codigo"`
}

func activar2FA(svc *auth.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sec, err := svc.Activar2FA(r.Context(), uidDe(r))
		if err != nil {
			slog.Error("activar 2FA", "error", err)
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": sec})
	}
}

func confirmar2FA(svc *auth.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req codigoReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		if err := svc.Confirmar2FA(r.Context(), uidDe(r), req.Codigo); err != nil {
			responderError2FA(w, err)
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"activado": true}})
	}
}

func desactivar2FA(svc *auth.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req codigoReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		if err := svc.Desactivar2FA(r.Context(), uidDe(r), req.Codigo); err != nil {
			responderError2FA(w, err)
			return
		}
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"desactivado": true}})
	}
}

type login2FAReq struct {
	PreToken string `json:"pre_2fa_token"`
	Codigo   string `json:"codigo"`
}

func login2FA(svc *auth.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req login2FAReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		if req.PreToken == "" || req.Codigo == "" {
			escribirError(w, http.StatusBadRequest, "pre_2fa_token y codigo son obligatorios")
			return
		}
		par, err := svc.Login2FA(r.Context(), req.PreToken, req.Codigo, ipDe(r), r.UserAgent())
		if errors.Is(err, auth.ErrCodigo2FAInvalido) {
			escribirError(w, http.StatusUnauthorized, "código 2FA inválido")
			return
		}
		if errors.Is(err, auth.ErrPreTokenInvalido) {
			escribirError(w, http.StatusUnauthorized, "pre-token inválido o expirado")
			return
		}
		if err != nil {
			slog.Error("login 2FA interno", "error", err)
			escribirError(w, http.StatusInternalServerError, "error interno")
			return
		}
		responderPar(w, par)
	}
}

// logout revoca el refresh (cookie httpOnly o body) y lo borra del navegador.
func logout(svc *auth.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		refresh := ""
		if c, err := r.Cookie("refresh_token"); err == nil {
			refresh = c.Value
		} else {
			var req refreshReq
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err == nil {
				refresh = req.RefreshToken
			}
		}
		if refresh != "" {
			_ = svc.RevocarPorHash(r.Context(), sha256Sum(refresh))
		}
		http.SetCookie(w, &http.Cookie{
			Name: "refresh_token", Value: "", Path: "/api/auth",
			HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1,
		})
		escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"logout": true}})
	}
}

func sha256Sum(v string) string {
	suma := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(suma[:])
}

type cambioPasswordReq struct {
	PasswordActual string `json:"password_actual"`
	PasswordNueva  string `json:"password_nueva"`
}

func cambiarPassword(svc *auth.Servicio) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req cambioPasswordReq
		if err := decodificarJSON(w, r, &req); err != nil {
			return
		}
		if req.PasswordActual == "" || req.PasswordNueva == "" {
			escribirError(w, http.StatusBadRequest, "password_actual y password_nueva son obligatorios")
			return
		}
		err := svc.CambiarPassword(r.Context(), uidDe(r), req.PasswordActual, req.PasswordNueva, ipDe(r))
		switch {
		case errors.Is(err, auth.ErrCredenciales):
			escribirError(w, http.StatusUnauthorized, "contraseña actual incorrecta")
		case errors.Is(err, auth.ErrPasswordDebil):
			escribirError(w, http.StatusBadRequest, err.Error())
		case err != nil:
			slog.Error("cambio password interno", "error", err)
			escribirError(w, http.StatusInternalServerError, "error interno")
		default:
			escribirJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"cambiado": true}})
		}
	}
}

func responderError2FA(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrCodigo2FAInvalido):
		escribirError(w, http.StatusUnauthorized, "código 2FA inválido")
	case errors.Is(err, auth.Err2FASinSecreto):
		escribirError(w, http.StatusBadRequest, "2FA no iniciado: activa y confirma primero")
	default:
		slog.Error("2FA interno", "error", err)
		escribirError(w, http.StatusInternalServerError, "error interno")
	}
}
