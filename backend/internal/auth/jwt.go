// JWT de acceso corto (15 min) con los claims del Flujo 1 del plano.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims transporta identidad y permisos básicos para el middleware RBAC.
type Claims struct {
	RolBase      string   `json:"rol_base"`    // estudiante | profesor
	Voceria      string   `json:"voceria"`     // "" | clase | carrera | universidad (mayor nivel)
	EsMantenedor bool     `json:"es_mantenedor"`
	SeccionIDs   []string `json:"seccion_ids"` // forward-compat: módulo académico
	jwt.RegisteredClaims
}

var ErrTokenInvalido = errors.New("token inválido o expirado")

// EmitirToken firma HS256 con expiración ttl.
func EmitirToken(c Claims, secreto string, ttl time.Duration) (string, error) {
	ahora := time.Now()
	c.RegisteredClaims = jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(ahora),
		ExpiresAt: jwt.NewNumericDate(ahora.Add(ttl)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secreto))
}

// ValidarToken verifica firma, expiración y algoritmo (rechaza none/RS->HS).
func ValidarToken(tokenStr, secreto string) (*Claims, error) {
	var claims Claims
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrTokenInvalido
		}
		return []byte(secreto), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrTokenInvalido
	}
	return &claims, nil
}
