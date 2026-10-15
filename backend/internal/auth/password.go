// Hashing de contraseñas con Argon2id en formato PHC estándar
// ($argon2id$v=19$m=...,t=...,p=...$sal$hash). Verificación en tiempo constante.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parámetros recomendados OWASP para Argon2id.
const (
	argonMemoria  = 64 * 1024 // 64 MB
	argonIter     = 3
	argonParalelo = 4
	argonSalLen   = 16
	argonClaveLen = 32
)

// HashPassword genera un string PHC con sal aleatoria. Nunca se guarda la
// contraseña en claro ni derivados reversibles (S2).
func HashPassword(password string) (string, error) {
	sal := make([]byte, argonSalLen)
	if _, err := rand.Read(sal); err != nil {
		return "", fmt.Errorf("generar sal: %w", err)
	}
	clave := argon2.IDKey([]byte(password), sal, argonIter, argonMemoria, argonParalelo, argonClaveLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoria, argonIter, argonParalelo,
		base64.RawStdEncoding.EncodeToString(sal),
		base64.RawStdEncoding.EncodeToString(clave)), nil
}

// VerificarPassword compara en tiempo constante; no distingue "no existe el
// usuario" de "clave incorrecta" para no filtrar usuarios válidos (S2).
func VerificarPassword(hash, password string) (bool, error) {
	partes := strings.Split(hash, "$")
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false, errors.New("hash con formato inválido")
	}
	var version int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("versión ilegible: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("versión argon2 %d no soportada", version)
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, fmt.Errorf("parámetros ilegibles: %w", err)
	}
	sal, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil {
		return false, fmt.Errorf("sal ilegible: %w", err)
	}
	esperada, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil {
		return false, fmt.Errorf("hash ilegible: %w", err)
	}
	candidata := argon2.IDKey([]byte(password), sal, t, m, p, uint32(len(esperada)))
	return subtle.ConstantTimeCompare(candidata, esperada) == 1, nil
}
