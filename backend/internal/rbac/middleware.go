// Package rbac: middleware de autenticación/autorización. RequireAuth valida
// el JWT Bearer y deja *auth.Claims en el contexto; RequireMantenedor y
// RequireRol guardan rutas por privilegio (S3).
package rbac

import (
	"context"
	"net/http"
	"strings"

	"github.com/elvaivas/universidad-app/backend/internal/auth"
)

type contextKey string

const claimsKey contextKey = "claims"

// RequireAuth exige Authorization: Bearer <jwt> válido; inyecta claims en ctx.
func RequireAuth(secreto string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, `{"error":"token requerido"}`, http.StatusUnauthorized)
				return
			}
			claims, err := auth.ValidarToken(strings.TrimPrefix(header, "Bearer "), secreto)
			if err != nil {
				http.Error(w, `{"error":"token inválido o expirado"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsDe extrae las claims del contexto (nil si el middleware no corrió).
func ClaimsDe(r *http.Request) *auth.Claims {
	c, _ := r.Context().Value(claimsKey).(*auth.Claims)
	return c
}

// RequireMantenedor exige claim es_mantenedor (403 en caso contrario).
// Debe usarse DESPUÉS de RequireAuth.
func RequireMantenedor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := ClaimsDe(r)
		if c == nil || !c.EsMantenedor {
			http.Error(w, `{"error":"se requiere ser mantenedor"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRol exige que rol_base esté en la lista (403 en caso contrario).
// Debe usarse DESPUÉS de RequireAuth.
func RequireRol(roles ...string) func(http.Handler) http.Handler {
	permitidos := map[string]bool{}
	for _, rol := range roles {
		permitidos[rol] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := ClaimsDe(r)
			if c == nil || !permitidos[c.RolBase] {
				http.Error(w, `{"error":"rol sin permiso para esta acción"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
